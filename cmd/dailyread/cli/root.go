package cli

import (
	"fmt"
	"os"

	"github.com/spf13/cobra"
)

var rootCmd = &cobra.Command{
	Use:   "dailyread",
	Short: "DailyRead performs deep web research and curates a weekly reading list",
	Long:  `A Go service that performs deep web research and curates a personalized reading list.`,
}

// Execute adds all child commands to the root command and sets flags appropriately.
func Execute() error {
	if err := rootCmd.Execute(); err != nil {
		fmt.Fprintf(os.Stderr, "Error: %s\n", err)
		return err
	}
	return nil
}
