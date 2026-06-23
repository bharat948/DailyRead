package observability

import (
	"io"
	"log/slog"
	"os"
)

// InitLogger initializes a global JSON structured logger.
// It writes to both stdout and a daily rolling log file.
func InitLogger() {
	roller := NewDailyRollingWriter("daily-logs", "dailyread")
	multi := io.MultiWriter(os.Stdout, roller)

	handler := slog.NewJSONHandler(multi, &slog.HandlerOptions{
		Level: slog.LevelInfo,
	})
	logger := slog.New(handler)
	slog.SetDefault(logger)
}

// WithRun creates a logger with the given run ID.
func WithRun(runID int64) *slog.Logger {
	return slog.With("run_id", runID)
}

// WithStage creates a logger with the given stage.
func WithStage(stage string) *slog.Logger {
	return slog.With("stage", stage)
}
