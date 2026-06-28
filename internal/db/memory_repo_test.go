package db

import (
	"testing"
	"time"

	"dailyread/internal/domain"
)

func newTestRepo(t *testing.T) *Repository {
	t.Helper()
	database, err := InitDB(t.TempDir())
	if err != nil {
		t.Fatalf("InitDB: %v", err)
	}
	t.Cleanup(func() { database.Close() })
	return NewRepository(database)
}

func mkUser(t *testing.T, r *Repository, id, email string) {
	t.Helper()
	if err := r.CreateUser(&domain.User{ID: id, Email: email, PasswordHash: "x", CreatedAt: time.Now()}); err != nil {
		t.Fatalf("CreateUser: %v", err)
	}
}

func TestGlobalResourceMemory(t *testing.T) {
	r := newTestRepo(t)

	url := "https://www.Example.com/post/?utm_source=tw#section"
	res := &domain.Resource{
		URLHash:      domain.HashURL(url),
		URL:          url,
		Domain:       domain.DomainOf(url),
		Title:        "Raft post-mortem",
		Summary:      "A distilled summary.",
		ContentText:  "full text",
		Type:         "case_study",
		WordCount:    1200,
		QualityScore: 0.8,
	}
	if err := r.UpsertResource(res); err != nil {
		t.Fatalf("UpsertResource: %v", err)
	}

	// Re-upsert with a different surface form of the same URL (www, trailing
	// slash, tracking param, fragment, casing) -> same hash, no dup.
	dupURL := "https://example.com/post/?utm_campaign=x"
	if domain.HashURL(dupURL) != res.URLHash {
		t.Fatalf("URL normalization mismatch: %q vs %q", domain.HashURL(dupURL), res.URLHash)
	}
	if err := r.UpsertResource(&domain.Resource{URLHash: res.URLHash, URL: dupURL}); err != nil {
		t.Fatalf("UpsertResource dup: %v", err)
	}

	got, err := r.GetResource(res.URLHash)
	if err != nil || got == nil {
		t.Fatalf("GetResource: %v (nil=%v)", err, got == nil)
	}
	if got.FetchCount != 2 {
		t.Errorf("fetch_count = %d, want 2 (upsert should bump)", got.FetchCount)
	}
	if got.Title != "Raft post-mortem" {
		t.Errorf("title clobbered on dup upsert: %q", got.Title)
	}
	if got.QualityScore != 0.8 {
		t.Errorf("quality clobbered: %v", got.QualityScore)
	}

	// Topic warm-start index.
	if err := r.LinkTopicResource("distributed-systems", res.URLHash, 0.9); err != nil {
		t.Fatalf("LinkTopicResource: %v", err)
	}
	warm, err := r.GetTopicResources("distributed-systems", 10)
	if err != nil {
		t.Fatalf("GetTopicResources: %v", err)
	}
	if len(warm) != 1 || warm[0].URLHash != res.URLHash {
		t.Fatalf("warm start = %+v, want the one resource", warm)
	}
}

func TestSearchCache(t *testing.T) {
	r := newTestRepo(t)
	q := "  Raft   Consensus  "
	h := domain.HashQuery(q)

	if err := r.PutSearchCache(h, q, `[{"url":"x"}]`, "tavily"); err != nil {
		t.Fatalf("PutSearchCache: %v", err)
	}
	hit, err := r.GetSearchCache(h, 3600)
	if err != nil || hit == nil {
		t.Fatalf("expected cache hit: %v", err)
	}
	if hit.Provider != "tavily" {
		t.Errorf("provider = %q", hit.Provider)
	}
	// Expired window -> miss.
	miss, err := r.GetSearchCache(h, -1)
	if err != nil {
		t.Fatalf("GetSearchCache expired: %v", err)
	}
	if miss != nil {
		t.Errorf("expected miss for expired window, got %+v", miss)
	}
}

func TestUserMemory(t *testing.T) {
	r := newTestRepo(t)
	mkUser(t, r, "u1", "a@b.com")

	run := &domain.Run{ID: "run1", UserID: "u1", Trigger: "manual"}
	if err := r.CreateRun(run); err != nil {
		t.Fatalf("CreateRun: %v", err)
	}

	hash := domain.HashURL("https://example.com/a")
	if err := r.CreateDigestItem(&domain.DigestItem{
		ID: "d1", RunID: "run1", UserID: "u1", URLHash: hash,
		InterestTag: "ai", Title: "Item A", Relevance: 0.7,
	}); err != nil {
		t.Fatalf("CreateDigestItem: %v", err)
	}
	if err := r.MarkSeen("u1", hash, "run1"); err != nil {
		t.Fatalf("MarkSeen: %v", err)
	}

	run.Status = "succeeded"
	run.ItemCount = 1
	run.Stage = "deliver"
	if err := r.FinishRun(run); err != nil {
		t.Fatalf("FinishRun: %v", err)
	}

	items, err := r.GetRecentDigestItems("u1", 10)
	if err != nil || len(items) != 1 {
		t.Fatalf("GetRecentDigestItems = %d items, err=%v", len(items), err)
	}

	seen, err := r.GetSeenHashes("u1")
	if err != nil {
		t.Fatalf("GetSeenHashes: %v", err)
	}
	if !seen[hash] {
		t.Errorf("expected %s in seen set", hash)
	}

	// Profile starts empty, then versions up on each upsert.
	p, _ := r.GetUserProfile("u1")
	if p.Version != 0 {
		t.Errorf("fresh profile version = %d, want 0", p.Version)
	}
	if err := r.UpsertUserProfile("u1", "likes consensus papers"); err != nil {
		t.Fatalf("UpsertUserProfile: %v", err)
	}
	if err := r.UpsertUserProfile("u1", "likes consensus + failure post-mortems"); err != nil {
		t.Fatalf("UpsertUserProfile 2: %v", err)
	}
	p, _ = r.GetUserProfile("u1")
	if p.Version != 2 {
		t.Errorf("profile version = %d, want 2", p.Version)
	}
}
