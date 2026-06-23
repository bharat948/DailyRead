package cli

import (
	"context"
	"fmt"
	"log/slog"
	"strings"

	"dailyread/internal/config"
	"dailyread/internal/search"
	"dailyread/internal/search/ddg"
	"dailyread/internal/search/serpapi"
	"dailyread/internal/search/tavily"
	"github.com/spf13/cobra"
)

var searchCmd = &cobra.Command{
	Use:   "search [query]",
	Short: "Perform a web search using configured providers",
	Args:  cobra.MinimumNArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		queryText := strings.Join(args, " ")
		
		path, _ := cmd.Flags().GetString("file")
		if path == "" {
			path = "configs/config.yaml"
		}

		cfg, err := config.NewLoader(path).Load()
		if err != nil {
			return fmt.Errorf("failed to load configuration: %w", err)
		}

		available := []search.Searcher{
			tavily.New(),
			serpapi.New(),
			ddg.New(),
		}

		router := search.NewRouter(cfg.Search, available)

		q := search.Query{
			Text:       queryText,
			MaxResults: 5,
		}

		slog.Info("Executing search", "query", queryText, "fanout", cfg.Search.Fanout)
		
		results, err := router.Search(context.Background(), q)
		if err != nil {
			return fmt.Errorf("search failed: %w", err)
		}

		fmt.Printf("\nFound %d results:\n\n", len(results))
		for i, r := range results {
			fmt.Printf("%d. [%s] %s\n", i+1, r.Source, r.Title)
			fmt.Printf("   URL: %s\n", r.URL)
			// Truncate snippet
			snippet := r.Snippet
			if len(snippet) > 150 {
				snippet = snippet[:147] + "..."
			}
			fmt.Printf("   Snippet: %s\n\n", snippet)
		}

		return nil
	},
}

func init() {
	rootCmd.AddCommand(searchCmd)
	searchCmd.Flags().StringP("file", "f", "configs/config.yaml", "config file path")
}
