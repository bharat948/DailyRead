package schedule

import (
	"context"
	"log/slog"
	"time"

	"github.com/robfig/cron/v3"
)

type Job func(ctx context.Context) error

type Scheduler struct {
	c *cron.Cron
}

func New(timezone string) (*Scheduler, error) {
	var loc *time.Location
	var err error

	if timezone != "" {
		loc, err = time.LoadLocation(timezone)
		if err != nil {
			return nil, err
		}
	} else {
		loc = time.Local
	}

	c := cron.New(cron.WithLocation(loc))
	return &Scheduler{c: c}, nil
}

func (s *Scheduler) Register(expr string, job Job) error {
	_, err := s.c.AddFunc(expr, func() {
		slog.Info("Executing scheduled job", "expr", expr)
		ctx := context.Background()
		if err := job(ctx); err != nil {
			slog.Error("Scheduled job failed", "error", err)
		} else {
			slog.Info("Scheduled job completed successfully")
		}
	})
	return err
}

func (s *Scheduler) Start() {
	slog.Info("Starting cron scheduler")
	s.c.Start()
}

func (s *Scheduler) Stop() {
	slog.Info("Stopping cron scheduler")
	ctx := s.c.Stop()
	<-ctx.Done()
}
