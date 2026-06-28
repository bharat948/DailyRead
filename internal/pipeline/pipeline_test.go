package pipeline

import (
	"testing"

	"dailyread/internal/domain"
)

func TestFilterUnseen(t *testing.T) {
	seenURL := "https://example.com/already-shown"
	seen := map[string]bool{domain.HashURL(seenURL): true}

	pool := []domain.Candidate{
		{URL: "https://example.com/new-a", Title: "A"},
		{URL: seenURL, Title: "already shown"},            // dropped: seen in a prior run
		{URL: "https://www.example.com/new-a/", Title: "A dup"}, // dropped: dup of A within pool
		{URL: "https://example.com/new-c", Title: "C"},
		{URL: "", Title: "empty url"},                     // dropped: no URL
	}

	fresh, dropped := filterUnseen(pool, seen)

	if len(fresh) != 2 {
		t.Fatalf("fresh = %d, want 2 (A, C); got %+v", len(fresh), fresh)
	}
	if dropped != 2 {
		t.Errorf("dropped = %d, want 2 (seen + in-pool dup)", dropped)
	}
	if fresh[0].Title != "A" || fresh[1].Title != "C" {
		t.Errorf("unexpected fresh order/content: %+v", fresh)
	}
}
