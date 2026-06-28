package web

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"
	"strings"

	"dailyread/internal/domain"
	"github.com/google/uuid"
)

// registerAPI mounts the JSON HTTP API. It is auth-free by design: DailyRead runs
// single-user/local for now. The user is resolved from ?user=, the X-User-ID
// header, or the sole account if exactly one exists.
func (s *Server) registerAPI() {
	s.mux.HandleFunc("GET /api/healthz", s.handleHealthz)

	s.mux.HandleFunc("POST /api/runs", s.handleAPITriggerRun)
	s.mux.HandleFunc("GET /api/runs", s.handleAPIListRuns)
	s.mux.HandleFunc("GET /api/runs/{id}", s.handleAPIGetRun)

	s.mux.HandleFunc("GET /api/digests", s.handleAPIDigests)
	s.mux.HandleFunc("GET /api/profile", s.handleAPIProfile)

	s.mux.HandleFunc("GET /api/topics/{topic}", s.handleAPITopic)

	s.mux.HandleFunc("GET /api/interests", s.handleAPIInterests)
	s.mux.HandleFunc("POST /api/interests", s.handleAPIAddInterest)
	s.mux.HandleFunc("DELETE /api/interests/{id}", s.handleAPIDeleteInterest)
}

func (s *Server) handleHealthz(w http.ResponseWriter, _ *http.Request) {
	writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}

// POST /api/runs — trigger a pipeline run (async); returns the created run.
func (s *Server) handleAPITriggerRun(w http.ResponseWriter, r *http.Request) {
	userID, err := s.resolveUser(r)
	if err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}
	run, err := s.pipe.TriggerAsync(userID, "manual")
	if err != nil {
		writeError(w, http.StatusInternalServerError, err)
		return
	}
	writeJSON(w, http.StatusAccepted, run)
}

// GET /api/runs — list a user's runs, newest first.
func (s *Server) handleAPIListRuns(w http.ResponseWriter, r *http.Request) {
	userID, err := s.resolveUser(r)
	if err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}
	runs, err := s.repo.GetRunsByUser(userID, queryLimit(r, 20))
	if err != nil {
		writeError(w, http.StatusInternalServerError, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"runs": runs})
}

// GET /api/runs/{id} — a single run plus the items it delivered.
func (s *Server) handleAPIGetRun(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	run, err := s.repo.GetRun(id)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err)
		return
	}
	if run == nil {
		writeError(w, http.StatusNotFound, fmt.Errorf("run %s not found", id))
		return
	}
	items, err := s.repo.GetDigestItemsByRun(id)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"run": run, "items": items})
}

// GET /api/digests — a user's recently delivered items (the past-digest memory).
func (s *Server) handleAPIDigests(w http.ResponseWriter, r *http.Request) {
	userID, err := s.resolveUser(r)
	if err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}
	items, err := s.repo.GetRecentDigestItems(userID, queryLimit(r, 50))
	if err != nil {
		writeError(w, http.StatusInternalServerError, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"items": items})
}

// GET /api/profile — a user's compacted long-term profile.
func (s *Server) handleAPIProfile(w http.ResponseWriter, r *http.Request) {
	userID, err := s.resolveUser(r)
	if err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}
	p, err := s.repo.GetUserProfile(userID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err)
		return
	}
	writeJSON(w, http.StatusOK, p)
}

// GET /api/topics/{topic} — peek into the global research corpus for a topic.
func (s *Server) handleAPITopic(w http.ResponseWriter, r *http.Request) {
	topic := r.PathValue("topic")
	res, err := s.repo.GetTopicResources(topic, queryLimit(r, 20))
	if err != nil {
		writeError(w, http.StatusInternalServerError, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"topic": topic, "resources": res})
}

// GET /api/interests — list a user's interests.
func (s *Server) handleAPIInterests(w http.ResponseWriter, r *http.Request) {
	userID, err := s.resolveUser(r)
	if err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}
	interests, err := s.repo.GetUserInterests(userID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"interests": interests})
}

// POST /api/interests — add an interest. Body: {tag, intensity, types[], primary}.
func (s *Server) handleAPIAddInterest(w http.ResponseWriter, r *http.Request) {
	userID, err := s.resolveUser(r)
	if err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}
	var body struct {
		Tag       string   `json:"tag"`
		Intensity string   `json:"intensity"`
		Types     []string `json:"types"`
		Primary   bool     `json:"primary"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeError(w, http.StatusBadRequest, fmt.Errorf("invalid JSON body: %w", err))
		return
	}
	if strings.TrimSpace(body.Tag) == "" {
		writeError(w, http.StatusBadRequest, fmt.Errorf("tag is required"))
		return
	}
	if body.Intensity == "" {
		body.Intensity = "medium"
	}
	it := &domain.UserInterest{
		ID:        uuid.New().String(),
		UserID:    userID,
		Tag:       strings.TrimSpace(body.Tag),
		Intensity: body.Intensity,
		Types:     body.Types,
		IsPrimary: body.Primary,
	}
	if err := s.repo.CreateInterest(it); err != nil {
		writeError(w, http.StatusInternalServerError, err)
		return
	}
	writeJSON(w, http.StatusCreated, it)
}

// DELETE /api/interests/{id} — remove an interest owned by the user.
func (s *Server) handleAPIDeleteInterest(w http.ResponseWriter, r *http.Request) {
	userID, err := s.resolveUser(r)
	if err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}
	if err := s.repo.DeleteInterest(r.PathValue("id"), userID); err != nil {
		writeError(w, http.StatusInternalServerError, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// ---- helpers ----

// resolveUser determines which user an API request targets.
func (s *Server) resolveUser(r *http.Request) (string, error) {
	if u := r.URL.Query().Get("user"); u != "" {
		return u, nil
	}
	if u := r.Header.Get("X-User-ID"); u != "" {
		return u, nil
	}
	ids, err := s.repo.GetAllUserIDs()
	if err != nil {
		return "", err
	}
	switch len(ids) {
	case 0:
		return "", fmt.Errorf("no users exist; register one first")
	case 1:
		return ids[0], nil
	default:
		return "", fmt.Errorf("multiple users; specify ?user=ID or X-User-ID header")
	}
}

func queryLimit(r *http.Request, def int) int {
	if v := r.URL.Query().Get("limit"); v != "" {
		if n, err := strconv.Atoi(v); err == nil && n > 0 && n <= 500 {
			return n
		}
	}
	return def
}

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}

func writeError(w http.ResponseWriter, status int, err error) {
	writeJSON(w, status, map[string]string{"error": err.Error()})
}
