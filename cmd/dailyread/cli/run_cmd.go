package cli

import (
	"context"
	"fmt"
	"log/slog"
	"os"

	"dailyread/internal/db"
	"dailyread/internal/pipeline"

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
			userID, err = resolveDefaultUser(repo)
			if err != nil {
				return err
			}
		}

		slog.Info("Starting FULL DailyRead Pipeline manually")
		return runPipelineForUser(repo, userID)
	},
}

// resolveDefaultUser picks a user when --user is omitted: the DEFAULT_USER_EMAIL
// from .env if set, otherwise the only user if there is exactly one.
func resolveDefaultUser(repo *db.Repository) (string, error) {
	_ = godotenv.Load()
	if email := os.Getenv("DEFAULT_USER_EMAIL"); email != "" {
		if user, err := repo.GetUserByEmail(email); err == nil && user != nil {
			slog.Info("auto-selecting default user from .env", "user_id", user.ID, "email", email)
			return user.ID, nil
		}
	}

	users, err := repo.GetAllUserIDs()
	if err != nil {
		return "", fmt.Errorf("failed to load users: %w", err)
	}
	switch len(users) {
	case 0:
		return "", fmt.Errorf("no users found in database")
	case 1:
		slog.Info("auto-selecting only user", "user_id", users[0])
		return users[0], nil
	default:
		return "", fmt.Errorf("multiple users found; specify --user ID. Available: %v", users)
	}
}

// runPipelineForUser runs the full pipeline synchronously via the pipeline service.
func runPipelineForUser(repo *db.Repository, userID string) error {
	_, err := pipeline.New(repo).Run(context.Background(), userID, "manual")
	return err
}

func init() {
	rootCmd.AddCommand(runCmd)
	runCmd.Flags().StringP("user", "u", "", "User ID to run for")
	runCmd.Flags().StringP("data", "d", "data", "database directory")
}
