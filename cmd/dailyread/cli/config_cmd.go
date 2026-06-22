package cli

import (
	"fmt"

	"dailyread/internal/config"
	"github.com/spf13/cobra"
	"gopkg.in/yaml.v3"
)

var configCmd = &cobra.Command{
	Use:   "config",
	Short: "Manage configuration",
}

var configShowCmd = &cobra.Command{
	Use:   "show",
	Short: "Validate and display the current configuration",
	RunE: func(cmd *cobra.Command, args []string) error {
		path, _ := cmd.Flags().GetString("file")
		if path == "" {
			path = "configs/config.yaml" // Default path for now
		}

		loader := config.NewLoader(path)
		cfg, err := loader.Load()
		if err != nil {
			return fmt.Errorf("failed to load configuration: %w", err)
		}

		out, err := yaml.Marshal(cfg)
		if err != nil {
			return fmt.Errorf("failed to marshal config for display: %w", err)
		}

		fmt.Println("Configuration loaded and validated successfully:")
		fmt.Println("---")
		fmt.Print(string(out))
		return nil
	},
}

func init() {
	configCmd.AddCommand(configShowCmd)
	rootCmd.AddCommand(configCmd)

	configCmd.PersistentFlags().StringP("file", "f", "configs/config.yaml", "config file path")
}
