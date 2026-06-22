package cli

import (
	"context"
	"fmt"
	"log/slog"

	"dailyread/internal/store"
	"dailyread/migrations"
	"github.com/pressly/goose/v3"
	"github.com/spf13/cobra"
)


var migrateCmd = &cobra.Command{
	Use:   "migrate",
	Short: "Run database migrations",
	RunE: func(cmd *cobra.Command, args []string) error {
		dbPath, _ := cmd.Flags().GetString("db")
		if dbPath == "" {
			dbPath = "dailyread.db" // Fallback default
		}

		ctx := context.Background()
		slog.Info("Connecting to database", "path", dbPath)
		db, err := store.Open(ctx, dbPath)
		if err != nil {
			return fmt.Errorf("failed to open database: %w", err)
		}
		defer db.Close()

		goose.SetBaseFS(migrations.FS)
		
		if err := goose.SetDialect("sqlite3"); err != nil {
			return fmt.Errorf("failed to set dialect: %w", err)
		}

		slog.Info("Running migrations...")
		// Use dir parameter pointing inside the embed.FS (it preserves folder structure)
		if err := goose.Up(db.DB, "."); err != nil {
			return fmt.Errorf("migration failed: %w", err)
		}

		slog.Info("Database migrations completed successfully.")
		return nil
	},
}

func init() {
	rootCmd.AddCommand(migrateCmd)
	migrateCmd.Flags().String("db", "dailyread.db", "path to SQLite database")
}
