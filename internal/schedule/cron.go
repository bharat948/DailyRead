package schedule

import (
	"context"
	"log/slog"
	"sync"

	"github.com/robfig/cron/v3"
)

type Job func(ctx context.Context, userID string) error

type Scheduler struct {
	c    *cron.Cron
	jobs map[string]cron.EntryID
	mu   sync.Mutex
	fn   Job
}

func New(jobFn Job) *Scheduler {
	// We'll manage timezone per-user dynamically in the cron expression or by
	// standardizing everything to UTC. For now, robfig/cron parses CRON_TZ=...
	// so if a user has a timezone, we can prepend `CRON_TZ=America/New_York ` to their expr
	c := cron.New(cron.WithParser(cron.NewParser(
		cron.SecondOptional | cron.Minute | cron.Hour | cron.Dom | cron.Month | cron.Dow | cron.Descriptor,
	)))
	
	return &Scheduler{
		c:    c,
		jobs: make(map[string]cron.EntryID),
		fn:   jobFn,
	}
}

func (s *Scheduler) AddUserJob(userID string, expr string, timezone string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	// Remove existing if any
	if id, ok := s.jobs[userID]; ok {
		s.c.Remove(id)
		delete(s.jobs, userID)
	}

	// Format expr to include timezone if provided
	fullExpr := expr
	if timezone != "" && timezone != "UTC" {
		fullExpr = "CRON_TZ=" + timezone + " " + expr
	}

	id, err := s.c.AddFunc(fullExpr, func() {
		slog.Info("Executing scheduled job for user", "user_id", userID, "expr", fullExpr)
		ctx := context.Background()
		if err := s.fn(ctx, userID); err != nil {
			slog.Error("Scheduled job failed", "user_id", userID, "error", err)
		} else {
			slog.Info("Scheduled job completed successfully", "user_id", userID)
		}
	})

	if err != nil {
		return err
	}

	s.jobs[userID] = id
	return nil
}

func (s *Scheduler) RemoveUserJob(userID string) {
	s.mu.Lock()
	defer s.mu.Unlock()

	if id, ok := s.jobs[userID]; ok {
		s.c.Remove(id)
		delete(s.jobs, userID)
	}
}

func (s *Scheduler) Start() {
	slog.Info("Starting global cron scheduler")
	s.c.Start()
}

func (s *Scheduler) Stop() {
	slog.Info("Stopping global cron scheduler")
	ctx := s.c.Stop()
	<-ctx.Done()
}
