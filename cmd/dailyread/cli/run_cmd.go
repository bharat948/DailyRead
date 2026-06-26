package cli

import (
	"context"
	"fmt"
	"log/slog"
	"os"

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
	"github.com/joho/godotenv"
	"github.com/spf13/cobra"
)

var runCmd = &cobra.Command{
	Use:   "run-now",
	Short: "Run the DailyRead pipeline manually for a specific user",
	RunE: func(cmd *cobra.Command, args []string) error {
		userID, _ := cmd.Flags().GetString("user")
		dataDir, _ := cmd.Flags().GetString("data")
		if dataDir == "" {
			dataDir = "data"
		}

		database, err := db.InitDB(dataDir)
		if err != nil {
			return fmt.Errorf("failed to init database: %w", err)
		}
		repo := db.NewRepository(database)

		if userID == "" {
			_ = godotenv.Load()
			if defaultEmail := os.Getenv("DEFAULT_USER_EMAIL"); defaultEmail != "" {
				user, err := repo.GetUserByEmail(defaultEmail)
				if err == nil && user != nil {
					userID = user.ID
					slog.Info("No user specified, auto-selecting default user from .env", "user_id", userID, "email", defaultEmail)
				}
			}

			if userID == "" {
				users, err := repo.GetAllUserIDs()
				if err != nil {
					return fmt.Errorf("failed to load users: %w", err)
				}
				if len(users) == 0 {
					return fmt.Errorf("no users found in database")
				}
				if len(users) == 1 {
					userID = users[0]
					slog.Info("No user specified, auto-selecting only user", "user_id", userID)
				} else {
					return fmt.Errorf("multiple users found. Must specify --user ID. Available: %v", users)
				}
			}
		}

		slog.Info("Starting FULL DailyRead Pipeline manually")
		return runPipelineForUser(repo, userID)
	},
}

func runPipelineForUser(repo *db.Repository, userID string) error {
	// Fetch Config
	cfg, err := repo.GetUserConfig(userID)
	if err != nil {
		return fmt.Errorf("failed to load user config: %w", err)
	}
	
	// Fetch Interests
	interests, err := repo.GetUserInterests(userID)
	if err != nil {
		return fmt.Errorf("failed to load interests: %w", err)
	}

	stats := domain.NewPipelineStats()
	ctx := domain.ContextWithStats(context.Background(), stats)

	// Note: in a real system we'd pass user-specific API keys down here.
	// We'll use the global ones if the user hasn't provided them for now to maintain backward compatibility during transition.

	// Setup Search
	available := []search.Searcher{
		tavily.New(),
		serpapi.New(),
		ddg.New(),
	}
	searchRouter := search.NewRouter(search.SearchConfig{
		Primary: "tavily",
	}, available)

	// Setup Fetcher
	fetcher := fetch.New()

	// Setup LLM using the user's config
	llmCfg := llm.ModelsConfig{
		Provider: cfg.ModelsProvider,
	}
	llmRouter := llm.NewRouter(llmCfg)
	client, model, err := llmRouter.ClientFor(llm.RoleResearch)
	if err != nil {
		return err
	}

	// Setup Researcher Agent
	researcher := research.New(client, searchRouter, fetcher, model, 3)

	var allCandidates []domain.Candidate

	for _, intst := range interests {
		interest := domain.Interest{
			Tag:       intst.Tag,
			Primary:   intst.IsPrimary,
			Intensity: intst.Intensity,
			Types:     intst.Types,
		}

		fmt.Printf("\n=== Researching interest: %s ===\n", interest.Tag)

		candidates, err := researcher.Run(ctx, interest)
		if err != nil {
			slog.Error("Researcher failed for interest", "interest", interest.Tag, "error", err)
			continue
		}

		allCandidates = append(allCandidates, candidates...)
	}

	fmt.Printf("\n=== Research Complete! Pooled %d Candidates Total ===\n", len(allCandidates))

	if len(allCandidates) == 0 {
		slog.Info("No candidates found, skipping delivery")
		return nil
	}

	// Triage Phase
	fmt.Println("\n=== Starting Triage Phase ===")
	tClient, tModel, err := llmRouter.ClientFor(llm.RoleTriage)
	if err != nil {
		return err
	}

	triager := triage.New(tClient, tModel, 5)

	finalCandidates, err := triager.Run(ctx, allCandidates)
	if err != nil {
		return fmt.Errorf("triage failed: %w", err)
	}

	fmt.Printf("\n=== Triage Complete! Selected %d Candidates ===\n", len(finalCandidates))

	// Delivery Phase
	fmt.Println("\n=== Starting Delivery Phase ===")
	digest, err := delivery.FormatDigest(finalCandidates, stats)
	if err != nil {
		return fmt.Errorf("failed to format digest: %w", err)
	}

	emailCfg := delivery.EmailConfig{
		Channel: "smtp",
	}

	host := cfg.SMTPHost
	if host == "" {
		host = os.Getenv("SMTP_HOST")
	}
	port := cfg.SMTPPort
	if port == "" {
		port = os.Getenv("SMTP_PORT")
	}
	smtpUser := cfg.SMTPUser
	if smtpUser == "" {
		smtpUser = os.Getenv("SMTP_USER")
	}
	smtpPass := cfg.SMTPPassEncrypted
	if smtpPass == "" {
		smtpPass = os.Getenv("SMTP_PASS")
	}

	emailCfg.SMTP.Host = host
	emailCfg.SMTP.Port = port
	emailCfg.SMTP.User = smtpUser
	emailCfg.SMTP.Password = smtpPass

	userEmail := smtpUser // Fallback
	// Ideally we'd fetch the user to get their actual login email, but smtpUser is fine for now
	userCfg := delivery.UserConfig{
		Email: userEmail,
		Name:  "User",
	}

	sender := delivery.NewSender(emailCfg, userCfg)
	if err := sender.Send(digest); err != nil {
		return fmt.Errorf("delivery failed: %w", err)
	}

	return nil
}

func init() {
	rootCmd.AddCommand(runCmd)
	runCmd.Flags().StringP("user", "u", "", "User ID to run for")
	runCmd.Flags().StringP("data", "d", "data", "database directory")
}
