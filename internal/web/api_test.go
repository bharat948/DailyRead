package web

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"dailyread/internal/db"
	"dailyread/internal/domain"
	"dailyread/internal/pipeline"

	"github.com/google/uuid"
)

func newTestServer(t *testing.T) (*Server, string) {
	t.Helper()
	database, err := db.InitDB(t.TempDir())
	if err != nil {
		t.Fatalf("InitDB: %v", err)
	}
	t.Cleanup(func() { database.Close() })
	repo := db.NewRepository(database)

	userID := uuid.New().String()
	if err := repo.CreateUser(&domain.User{ID: userID, Email: "a@b.com", PasswordHash: "x", CreatedAt: time.Now()}); err != nil {
		t.Fatalf("CreateUser: %v", err)
	}

	srv, err := NewServer(repo, pipeline.New(repo), func(string, bool, string, string) {})
	if err != nil {
		t.Fatalf("NewServer: %v", err)
	}
	return srv, userID
}

func do(t *testing.T, srv *Server, method, path, body string) *httptest.ResponseRecorder {
	t.Helper()
	var r *http.Request
	if body != "" {
		r = httptest.NewRequest(method, path, strings.NewReader(body))
	} else {
		r = httptest.NewRequest(method, path, nil)
	}
	w := httptest.NewRecorder()
	srv.ServeHTTP(w, r)
	return w
}

func TestAPIHealthz(t *testing.T) {
	srv, _ := newTestServer(t)
	w := do(t, srv, "GET", "/api/healthz", "")
	if w.Code != http.StatusOK {
		t.Fatalf("healthz status = %d, body=%s", w.Code, w.Body.String())
	}
	if !strings.Contains(w.Body.String(), `"status":"ok"`) {
		t.Errorf("healthz body = %s", w.Body.String())
	}
}

func TestAPIInterestsCRUD(t *testing.T) {
	srv, uid := newTestServer(t)

	// Empty to start.
	w := do(t, srv, "GET", "/api/interests?user="+uid, "")
	if w.Code != http.StatusOK {
		t.Fatalf("list interests status = %d", w.Code)
	}

	// Add one.
	w = do(t, srv, "POST", "/api/interests?user="+uid,
		`{"tag":"distributed-systems","intensity":"high","primary":true,"types":["case_study"]}`)
	if w.Code != http.StatusCreated {
		t.Fatalf("add interest status = %d, body=%s", w.Code, w.Body.String())
	}

	// Now listed.
	w = do(t, srv, "GET", "/api/interests?user="+uid, "")
	if !strings.Contains(w.Body.String(), "distributed-systems") {
		t.Errorf("expected interest in list, got %s", w.Body.String())
	}
}

func TestAPIRunsAndProfileReadback(t *testing.T) {
	srv, uid := newTestServer(t)

	// No runs yet -> 200 with empty list.
	w := do(t, srv, "GET", "/api/runs?user="+uid, "")
	if w.Code != http.StatusOK {
		t.Fatalf("list runs status = %d", w.Code)
	}

	// Fresh profile -> version 0.
	w = do(t, srv, "GET", "/api/profile?user="+uid, "")
	if w.Code != http.StatusOK || !strings.Contains(w.Body.String(), `"version":0`) {
		t.Errorf("profile readback = %d / %s", w.Code, w.Body.String())
	}

	// Unknown run -> 404.
	w = do(t, srv, "GET", "/api/runs/does-not-exist", "")
	if w.Code != http.StatusNotFound {
		t.Errorf("unknown run status = %d, want 404", w.Code)
	}
}
