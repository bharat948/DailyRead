package domain

import (
	"context"
	"strings"
	"sync"
)

type contextKey string

const statsKey contextKey = "pipeline_stats"

type PipelineStats struct {
	TokensIn    int
	TokensOut   int
	Cost        float64
	WebRequests int
	mu          sync.Mutex
}

func NewPipelineStats() *PipelineStats {
	return &PipelineStats{}
}

func ContextWithStats(ctx context.Context, stats *PipelineStats) context.Context {
	return context.WithValue(ctx, statsKey, stats)
}

func StatsFromContext(ctx context.Context) *PipelineStats {
	if stats, ok := ctx.Value(statsKey).(*PipelineStats); ok {
		return stats
	}
	return nil // If not injected, we return nil (callers must check)
}

func (s *PipelineStats) AddLLMUsage(model string, tokensIn int, tokensOut int) {
	s.mu.Lock()
	defer s.mu.Unlock()

	s.TokensIn += tokensIn
	s.TokensOut += tokensOut

	// Approximate cost calculation (Prices per 1M tokens)
	var costIn, costOut float64
	m := strings.ToLower(model)
	if strings.Contains(m, "gpt-4o-mini") {
		costIn = 0.150
		costOut = 0.600
	} else if strings.Contains(m, "gpt-4o") {
		costIn = 2.50
		costOut = 10.00
	} else if strings.Contains(m, "o1") || strings.Contains(m, "o3-mini") {
		costIn = 1.10
		costOut = 4.40
	} else if strings.Contains(m, "claude-3-5-sonnet") {
		costIn = 3.00
		costOut = 15.00
	} else {
		// Default fallback
		costIn = 0.0
		costOut = 0.0
	}

	s.Cost += (float64(tokensIn) / 1_000_000.0) * costIn
	s.Cost += (float64(tokensOut) / 1_000_000.0) * costOut
}

func (s *PipelineStats) AddWebRequest() {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.WebRequests++
}
