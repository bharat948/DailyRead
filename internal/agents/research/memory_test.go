package research

import (
	"context"
	"testing"

	"dailyread/internal/db"
	"dailyread/internal/domain"
	"dailyread/internal/search"
)

type fakeSearcher struct {
	calls   int
	results []search.Result
}

func (f *fakeSearcher) Search(_ context.Context, _ search.Query) ([]search.Result, error) {
	f.calls++
	return f.results, nil
}

type fakeFetcher struct {
	calls   int
	content string
}

func (f *fakeFetcher) Fetch(_ context.Context, _ string) (string, error) {
	f.calls++
	return f.content, nil
}

func newMemRepo(t *testing.T) *db.Repository {
	t.Helper()
	database, err := db.InitDB(t.TempDir())
	if err != nil {
		t.Fatalf("InitDB: %v", err)
	}
	t.Cleanup(func() { database.Close() })
	return db.NewRepository(database)
}

func TestMemSearchCachesAndDistills(t *testing.T) {
	repo := newMemRepo(t)
	fs := &fakeSearcher{results: []search.Result{
		{Title: "Raft post-mortem", URL: "https://www.ex.com/a?utm_source=x", Snippet: "snip", Source: "tavily", Score: 0.6},
	}}
	r := New(nil, fs, &fakeFetcher{}, repo, "m", 3)
	ctx := context.Background()

	// First search hits the provider and distills into the corpus.
	if _, err := r.memSearch(ctx, "distributed-systems", "raft consensus"); err != nil {
		t.Fatalf("memSearch 1: %v", err)
	}
	// Second identical search must be served from cache (no provider call).
	if _, err := r.memSearch(ctx, "distributed-systems", "  Raft   Consensus "); err != nil {
		t.Fatalf("memSearch 2: %v", err)
	}
	if fs.calls != 1 {
		t.Errorf("provider called %d times, want 1 (second should be a cache hit)", fs.calls)
	}

	// The result was distilled into the global corpus + topic index.
	got, err := repo.GetResource(domain.HashURL("https://ex.com/a"))
	if err != nil || got == nil {
		t.Fatalf("expected resource in corpus: %v", err)
	}
	if got.Title != "Raft post-mortem" {
		t.Errorf("resource title = %q", got.Title)
	}
	warm, _ := repo.GetTopicResources("distributed-systems", 10)
	if len(warm) != 1 {
		t.Errorf("topic index = %d resources, want 1", len(warm))
	}
}

func TestMemFetchServesFromCorpus(t *testing.T) {
	repo := newMemRepo(t)
	ff := &fakeFetcher{content: "full article text here"}
	r := New(nil, &fakeSearcher{}, ff, repo, "m", 3)
	ctx := context.Background()

	url := "https://ex.com/article"
	if _, err := r.memFetch(ctx, url); err != nil {
		t.Fatalf("memFetch 1: %v", err)
	}
	// Second fetch of same URL should come from the corpus, not the network.
	out, err := r.memFetch(ctx, url+"/") // normalizes to same hash
	if err != nil {
		t.Fatalf("memFetch 2: %v", err)
	}
	if ff.calls != 1 {
		t.Errorf("fetcher called %d times, want 1 (second should hit corpus)", ff.calls)
	}
	if out == "" {
		t.Errorf("expected cached content returned")
	}
}
