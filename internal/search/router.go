package search

import (
	"context"
	"fmt"
	"log/slog"
	"sync"
	"time"

	"dailyread/internal/domain"
	"github.com/cenkalti/backoff/v4"
	"github.com/sony/gobreaker"
)

type SearchConfig struct {
	Primary  string
	Fallback string
	Fanout   bool
	Priority []string
}

type Router struct {
	providers []Searcher
	breakers  map[string]*gobreaker.CircuitBreaker
	fanout    bool
	mu        sync.RWMutex
}

func NewRouter(cfg SearchConfig, available []Searcher) *Router {
	r := &Router{
		fanout:   cfg.Fanout,
		breakers: make(map[string]*gobreaker.CircuitBreaker),
	}

	// Order available by config priority
	priorityMap := make(map[string]int)
	for i, name := range cfg.Priority {
		priorityMap[name] = i
	}

	for _, p := range available {
		r.providers = append(r.providers, p)
		
		st := gobreaker.Settings{
			Name:        p.Name(),
			MaxRequests: 3,
			Interval:    time.Minute * 5,
			Timeout:     time.Minute,
			ReadyToTrip: func(counts gobreaker.Counts) bool {
				return counts.ConsecutiveFailures > 3
			},
		}
		r.breakers[p.Name()] = gobreaker.NewCircuitBreaker(st)
	}

	// Sort providers by priority (simplified logic, real one would stable sort)
	// For now, we trust the `available` list is ordered or we just reorder it.
	ordered := make([]Searcher, 0, len(available))
	for _, name := range cfg.Priority {
		for _, p := range r.providers {
			if p.Name() == name {
				ordered = append(ordered, p)
				break
			}
		}
	}
	r.providers = ordered

	return r
}

func (r *Router) Search(ctx context.Context, q Query) ([]Result, error) {
	if r.fanout {
		return r.searchFanout(ctx, q)
	}
	return r.searchSequential(ctx, q)
}

func (r *Router) searchSequential(ctx context.Context, q Query) ([]Result, error) {
	for _, p := range r.providers {
		if !p.Healthy() {
			continue
		}

		cb := r.breakers[p.Name()]
		var results []Result

		// Execute with Circuit Breaker
		_, err := cb.Execute(func() (interface{}, error) {
			// Execute with Backoff for transient errors
			b := backoff.NewExponentialBackOff()
			b.MaxElapsedTime = 10 * time.Second
			b.InitialInterval = 500 * time.Millisecond
			
			err := backoff.Retry(func() error {
				res, sErr := p.Search(ctx, q)
				if sErr != nil {
					slog.Warn("Search provider error", "provider", p.Name(), "error", sErr)
					return sErr
				}
				if stats := domain.StatsFromContext(ctx); stats != nil {
					stats.AddWebRequest()
				}
				results = res
				return nil
			}, backoff.WithContext(b, ctx))
			
			return nil, err
		})

		if err == nil {
			slog.Info("Search succeeded", "provider", p.Name(), "results", len(results))
			return results, nil
		}
		
		slog.Error("Provider failed entirely", "provider", p.Name(), "error", err)
	}

	return nil, fmt.Errorf("all search providers failed")
}

func (r *Router) searchFanout(ctx context.Context, q Query) ([]Result, error) {
	// Simple fanout to top 2 healthy providers
	var targets []Searcher
	for _, p := range r.providers {
		if p.Healthy() {
			targets = append(targets, p)
		}
		if len(targets) == 2 {
			break
		}
	}

	if len(targets) == 0 {
		return nil, fmt.Errorf("no healthy search providers available")
	}

	var wg sync.WaitGroup
	var mu sync.Mutex
	allResults := make([]Result, 0)
	errs := make([]error, 0)

	for _, p := range targets {
		wg.Add(1)
		go func(prov Searcher) {
			defer wg.Done()
			
			cb := r.breakers[prov.Name()]
			var provResults []Result
			
			_, err := cb.Execute(func() (interface{}, error) {
				b := backoff.NewExponentialBackOff()
				b.MaxElapsedTime = 10 * time.Second
				b.InitialInterval = 500 * time.Millisecond
				
				return nil, backoff.Retry(func() error {
					res, sErr := prov.Search(ctx, q)
					if sErr != nil {
						return sErr
					}
					if stats := domain.StatsFromContext(ctx); stats != nil {
						stats.AddWebRequest()
					}
					provResults = res
					return nil
				}, backoff.WithContext(b, ctx))
			})
			
			mu.Lock()
			defer mu.Unlock()
			if err != nil {
				slog.Error("Fanout provider failed", "provider", prov.Name(), "error", err)
				errs = append(errs, err)
			} else {
				allResults = append(allResults, provResults...)
			}
		}(p)
	}

	wg.Wait()

	if len(allResults) == 0 && len(errs) > 0 {
		return nil, fmt.Errorf("all fanout providers failed: %v", errs)
	}

	// Dedup
	seenURLs := make(map[string]bool)
	var deduped []Result
	for _, res := range allResults {
		if !seenURLs[res.URL] {
			seenURLs[res.URL] = true
			deduped = append(deduped, res)
		}
	}

	return deduped, nil
}
