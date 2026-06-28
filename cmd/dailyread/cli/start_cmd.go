package cli

import (
	"context"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"syscall"

	"dailyread/internal/db"
	"dailyread/internal/pipeline"
	"dailyread/internal/schedule"
	"dailyread/internal/web"
	"github.com/spf13/cobra"
)

var startCmd = &cobra.Command{
	Use:   "start",
	Short: "Start the DailyRead background daemon and web UI",
	RunE: func(cmd *cobra.Command, args []string) error {
		dataDir, _ := cmd.Flags().GetString("data")
		if dataDir == "" {
			dataDir = "data"
		}

		// Init DB
		database, err := db.InitDB(dataDir)
		if err != nil {
			return fmt.Errorf("failed to init database: %w", err)
		}
		repo := db.NewRepository(database)
		pipe := pipeline.New(repo)

		// Define the pipeline execution wrapper (scheduled trigger)
		jobFn := func(ctx context.Context, userID string) error {
			slog.Info("Executing pipeline for user", "user_id", userID)
			_, err := pipe.Run(ctx, userID, "scheduled")
			return err
		}

		// Initialize Scheduler
		sched := schedule.New(jobFn)

		// Load all users and schedule their cron jobs
		users, err := repo.GetAllUserIDs()
		if err != nil {
			return fmt.Errorf("failed to load users: %w", err)
		}

		for _, uid := range users {
			cfg, err := repo.GetUserConfig(uid)
			if err != nil || cfg == nil {
				continue
			}
			if cfg.ScheduleEnabled {
				if err := sched.AddUserJob(uid, cfg.ScheduleCron, cfg.ScheduleTimezone); err != nil {
					slog.Error("Failed to schedule job", "user_id", uid, "error", err)
				}
			}
		}

		sched.Start()
		defer sched.Stop()

		slog.Info("DailyRead daemon is running", "users_loaded", len(users))

		// Provide the callback to update the schedule dynamically from the web UI
		updateSchedule := func(userID string, enabled bool, expr string, timezone string) {
			if enabled {
				if err := sched.AddUserJob(userID, expr, timezone); err != nil {
					slog.Error("Failed to dynamically update schedule", "user_id", userID, "error", err)
				}
			} else {
				sched.RemoveUserJob(userID)
			}
		}

		server, err := web.NewServer(repo, pipe, updateSchedule)
		if err != nil {
			return fmt.Errorf("failed to init web server: %w", err)
		}

		port, _ := cmd.Flags().GetInt("port")
		srv := &http.Server{
			Addr:    fmt.Sprintf(":%d", port),
			Handler: server,
		}

		go func() {
			slog.Info("Web Dashboard listening", "url", fmt.Sprintf("http://localhost:%d", port))
			if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
				slog.Error("Web server failed", "error", err)
			}
		}()

		// Wait for interruption
		sigChan := make(chan os.Signal, 1)
		signal.Notify(sigChan, os.Interrupt, syscall.SIGTERM)
		<-sigChan

		slog.Info("Shutting down DailyRead daemon...")
		srv.Shutdown(context.Background())
		return nil
	},
}

func init() {
	rootCmd.AddCommand(startCmd)
	startCmd.Flags().StringP("data", "d", "data", "database directory")
	startCmd.Flags().IntP("port", "p", 8080, "HTTP dashboard port")
}
