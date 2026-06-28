// Package pipeline runs the end-to-end DailyRead research pipeline for a user and
// records the results into user-specific memory (runs, digest items, seen set, profile).
// It is the single entry point shared by the CLI, the scheduler, and the HTTP API.
package pipeline

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"strconv"

	"dailyread/internal/agents/compact"
	"dailyread/internal/agents/curator"
	"dailyread/internal/agents/research"
	"dailyread/internal/agents/triage"
	"dailyread/internal/db"
	"dailyread/internal/delivery"
	"dailyread/internal/domain"
	"dailyread/internal/fetch"
	"dailyread/internal/llm"
	"dailyread/internal/search"
	"dailyread/internal/search/ddg"
	"dailyread/internal/search/serpapi"
	"dailyread/internal/search/tavily"

	"github.com/google/uuid"
)

const (
	researchMaxRounds = 3
	triageMaxItems    = 5
)

// Service orchestrates a pipeline run against the repository.
type Service struct {
	repo *db.Repository
}

func New(repo *db.Repository) *Service { return &Service{repo: repo} }

// Run executes the full pipeline synchronously and returns the finished run.
func (s *Service) Run(ctx context.Context, userID, trigger string) (*domain.Run, error) {
	run, err := s.begin(userID, trigger)
	if err != nil {
		return nil, err
	}
	s.execute(ctx, run)
	return run, nil
}

// TriggerAsync creates the run, returns it immediately (status "running"), and
// executes the pipeline in the background. Used by HTTP triggers so the caller
// can poll GET /api/runs/{id} for progress.
func (s *Service) TriggerAsync(userID, trigger string) (*domain.Run, error) {
	run, err := s.begin(userID, trigger)
	if err != nil {
		return nil, err
	}
	go s.execute(context.Background(), run)
	return run, nil
}

func (s *Service) begin(userID, trigger string) (*domain.Run, error) {
	run := &domain.Run{
		ID:      uuid.New().String(),
		UserID:  userID,
		Trigger: trigger,
		Status:  "running",
		Stage:   "load",
	}
	if err := s.repo.CreateRun(run); err != nil {
		return nil, fmt.Errorf("create run: %w", err)
	}
	slog.Info("pipeline run started", "run_id", run.ID, "user_id", userID, "trigger", trigger)
	return run, nil
}

func (s *Service) execute(ctx context.Context, run *domain.Run) {
	stats := domain.NewPipelineStats()
	ctx = domain.ContextWithStats(ctx, stats)

	fail := func(stage string, err error) {
		slog.Error("pipeline failed", "run_id", run.ID, "stage", stage, "error", err)
		run.Status = "failed"
		run.Stage = stage
		run.Error = err.Error()
		s.finalize(run, stats)
	}

	// ------------------------------------------------------------------ load
	cfg, err := s.repo.GetUserConfig(run.UserID)
	if err != nil {
		fail("load", fmt.Errorf("load config: %w", err))
		return
	}
	interests, err := s.repo.GetUserInterests(run.UserID)
	if err != nil {
		fail("load", fmt.Errorf("load interests: %w", err))
		return
	}
	if len(interests) == 0 {
		fail("load", fmt.Errorf("user has no interests configured"))
		return
	}
	seen, err := s.repo.GetSeenHashes(run.UserID)
	if err != nil {
		fail("load", fmt.Errorf("load seen set: %w", err))
		return
	}
	// Load user profile for the Curator and Compactor.
	profile, err := s.repo.GetUserProfile(run.UserID)
	if err != nil {
		fail("load", fmt.Errorf("load user profile: %w", err))
		return
	}

	// ------------------------------------------------------------------ deps
	searchRouter := search.NewRouter(
		search.SearchConfig{Primary: "tavily", Priority: []string{"tavily", "serpapi", "ddg"}},
		[]search.Searcher{tavily.New(), serpapi.New(), ddg.New()},
	)
	fetcher := fetch.New()
	llmRouter := llm.NewRouter(llm.ModelsConfig{Provider: cfg.ModelsProvider})

	rClient, rModel, err := llmRouter.ClientFor(llm.RoleResearch)
	if err != nil {
		fail("research", fmt.Errorf("research client: %w", err))
		return
	}
	researcher := research.New(rClient, searchRouter, fetcher, s.repo, rModel, researchMaxRounds)

	// --------------------------------------------------------------- research
	run.Stage = "research"
	var pool []domain.Candidate
	for _, intst := range interests {
		interest := domain.Interest{
			Tag: intst.Tag, Primary: intst.IsPrimary, Intensity: intst.Intensity, Types: intst.Types,
		}
		cands, err := researcher.Run(ctx, interest)
		if err != nil {
			slog.Error("researcher failed for interest", "interest", interest.Tag, "error", err)
			continue
		}
		pool = append(pool, cands...)
	}

	// --------------------------------------------------------- novelty filter
	fresh, dropped := filterUnseen(pool, seen)
	slog.Info("novelty filter applied", "run_id", run.ID,
		"pool", len(pool), "fresh", len(fresh), "dropped_as_seen", dropped)

	if len(fresh) == 0 {
		slog.Info("no fresh candidates after novelty filter", "run_id", run.ID)
		run.Status = "succeeded"
		run.Stage = "triage"
		run.ItemCount = 0
		s.finalize(run, stats)
		return
	}

	// ----------------------------------------------------------------- triage
	run.Stage = "triage"
	tClient, tModel, err := llmRouter.ClientFor(llm.RoleTriage)
	if err != nil {
		fail("triage", fmt.Errorf("triage client: %w", err))
		return
	}
	triaged, err := triage.New(tClient, tModel, triageMaxItems).Run(ctx, fresh)
	if err != nil {
		fail("triage", err)
		return
	}

	// ----------------------------------------------------------------- curate
	// Curator enriches each item with personalized Why / How / Slot using the
	// user's long-term profile. Best-effort: if it fails we fall back to the
	// raw triaged candidates so the rest of the pipeline continues.
	run.Stage = "curate"
	cClient, cModel, err := llmRouter.ClientFor(llm.RoleCurate)
	if err != nil {
		slog.Error("curator client unavailable (non-fatal)", "error", err)
		// fall through with triaged as-is
	} else {
		enriched, err := curator.New(cClient, cModel).Run(ctx, profile.CompactedSummary, interests, triaged)
		if err != nil {
			slog.Error("curator failed (non-fatal)", "run_id", run.ID, "error", err)
		} else {
			triaged = enriched
		}
	}

	// ------------------------------------------------------- persist to memory
	run.Stage = "persist"
	var persisted []domain.DigestItem
	for _, c := range triaged {
		if c.URL == "" {
			continue
		}
		h := domain.HashURL(c.URL)
		item := &domain.DigestItem{
			ID:          uuid.New().String(),
			RunID:       run.ID,
			UserID:      run.UserID,
			URLHash:     h,
			InterestTag: c.InterestTag,
			Title:       c.Title,
			URL:         c.URL,
			Summary:     c.Summary,
			Why:         c.Why,
			How:         c.How,
			Relevance:   float64(c.Relevance) / 10,
			Novelty:     1.0,
		}
		if err := s.repo.CreateDigestItem(item); err != nil {
			slog.Error("persist digest item", "run_id", run.ID, "error", err)
		}
		if err := s.repo.MarkSeen(run.UserID, h, run.ID); err != nil {
			slog.Error("mark seen", "run_id", run.ID, "error", err)
		}
		persisted = append(persisted, *item)
	}
	run.ItemCount = len(persisted)

	// ---------------------------------------------------------------- deliver
	run.Stage = "deliver"
	if err := s.deliver(cfg, triaged, stats); err != nil {
		slog.Error("delivery failed (non-fatal)", "run_id", run.ID, "error", err)
		run.Status = "partial"
		s.compact(ctx, llmRouter, run.UserID, profile.CompactedSummary, persisted)
		s.finalize(run, stats)
		return
	}

	// ---------------------------------------------------------------- compact
	// Fold this run into the user's long-term profile so the anchor learns.
	// Best-effort: a compaction failure never fails the run.
	s.compact(ctx, llmRouter, run.UserID, profile.CompactedSummary, persisted)

	run.Status = "succeeded"
	s.finalize(run, stats)
}

// compact runs the Compactor agent and writes the updated profile to the DB.
func (s *Service) compact(ctx context.Context, llmRouter *llm.Router, userID, currentProfile string, items []domain.DigestItem) {
	if len(items) == 0 {
		return
	}
	client, model, err := llmRouter.ClientFor(llm.RoleCompact)
	if err != nil {
		slog.Error("compactor client unavailable", "error", err)
		return
	}
	updated, err := compact.New(client, model).Run(ctx, currentProfile, items)
	if err != nil {
		slog.Error("compactor failed (non-fatal)", "user_id", userID, "error", err)
		return
	}
	if err := s.repo.UpsertUserProfile(userID, updated); err != nil {
		slog.Error("upsert user profile", "user_id", userID, "error", err)
	}
	slog.Info("user profile updated", "user_id", userID)
}

func (s *Service) finalize(run *domain.Run, stats *domain.PipelineStats) {
	run.TokensInput = stats.TokensIn
	run.TokensOutput = stats.TokensOut
	if run.Status == "" {
		run.Status = "succeeded"
	}
	if err := s.repo.FinishRun(run); err != nil {
		slog.Error("finish run", "run_id", run.ID, "error", err)
	}
	slog.Info("pipeline run finished", "run_id", run.ID, "status", run.Status,
		"items", run.ItemCount, "tokens_in", run.TokensInput, "tokens_out", run.TokensOutput)
}

func (s *Service) deliver(cfg *domain.UserConfig, final []domain.Candidate, stats *domain.PipelineStats) error {
	host := firstNonEmpty(cfg.SMTPHost, os.Getenv("SMTP_HOST"))
	if host == "" {
		slog.Info("no SMTP configured; skipping email delivery")
		return nil
	}

	digest, err := delivery.FormatDigest(final, stats)
	if err != nil {
		return fmt.Errorf("format digest: %w", err)
	}

	port := cfg.SMTPPort
	if port == 0 {
		if p, perr := strconv.Atoi(os.Getenv("SMTP_PORT")); perr == nil {
			port = p
		}
	}
	user := firstNonEmpty(cfg.SMTPUser, os.Getenv("SMTP_USER"))
	pass := firstNonEmpty(cfg.SMTPPassEncrypted, os.Getenv("SMTP_PASS"))

	emailCfg := delivery.EmailConfig{Channel: "smtp"}
	emailCfg.SMTP.Host = host
	emailCfg.SMTP.Port = port
	emailCfg.SMTP.User = user
	emailCfg.SMTP.Password = pass

	sender := delivery.NewSender(emailCfg, delivery.UserConfig{Email: user, Name: "User"})
	return sender.Send(digest)
}

// filterUnseen removes candidates the user has already been shown (cross-run
// dedup via the seen set) and de-duplicates within the current pool.
func filterUnseen(pool []domain.Candidate, seen map[string]bool) ([]domain.Candidate, int) {
	var fresh []domain.Candidate
	dropped := 0
	local := make(map[string]bool)
	for _, c := range pool {
		if c.URL == "" {
			continue
		}
		h := domain.HashURL(c.URL)
		if seen[h] || local[h] {
			dropped++
			continue
		}
		local[h] = true
		fresh = append(fresh, c)
	}
	return fresh, dropped
}

func firstNonEmpty(a, b string) string {
	if a != "" {
		return a
	}
	return b
}
