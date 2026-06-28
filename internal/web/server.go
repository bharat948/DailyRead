package web

import (
	"crypto/rand"
	"embed"
	"fmt"
	"html/template"
	"net/http"

	"dailyread/internal/db"
	"dailyread/internal/pipeline"
	"github.com/gorilla/sessions"
)

//go:embed templates/*
var templateFS embed.FS

var store *sessions.CookieStore

type Server struct {
	repo *db.Repository
	tmpl *template.Template
	mux  *http.ServeMux

	// The pipeline service, used to trigger runs from the web UI and JSON API.
	pipe *pipeline.Service

	// Inject the scheduler updater so we can modify jobs when settings change
	updateSchedule func(userID string, enabled bool, expr string, timezone string)
}

func initSessionStore() {
	// In production, load this from an env var
	key := make([]byte, 32)
	rand.Read(key)
	store = sessions.NewCookieStore(key)
	store.Options = &sessions.Options{
		Path:     "/",
		MaxAge:   86400 * 7,
		HttpOnly: true,
		Secure:   false, // Set true in production (requires HTTPS)
		SameSite: http.SameSiteLaxMode,
	}
}

func NewServer(repo *db.Repository, pipe *pipeline.Service, updateSchedule func(string, bool, string, string)) (*Server, error) {
	initSessionStore()

	tmpl, err := template.ParseFS(templateFS, "templates/*.html")
	if err != nil {
		return nil, fmt.Errorf("failed to parse templates: %w", err)
	}

	s := &Server{
		repo:           repo,
		tmpl:           tmpl,
		mux:            http.NewServeMux(),
		pipe:           pipe,
		updateSchedule: updateSchedule,
	}

	s.routes()
	return s, nil
}

func (s *Server) routes() {
	// Public routes
	s.mux.HandleFunc("GET /login", s.handleLoginForm)
	s.mux.HandleFunc("POST /login", s.handleLoginPost)
	s.mux.HandleFunc("GET /register", s.handleRegisterForm)
	s.mux.HandleFunc("POST /register", s.handleRegisterPost)
	s.mux.HandleFunc("POST /logout", s.handleLogout)

	// Protected routes (we'll implement a simple middleware wrapper later if needed, 
	// for now we just check session in handlers or create a small wrap function)
	s.mux.HandleFunc("GET /", s.requireAuth(s.handleIndex))
	s.mux.HandleFunc("POST /run", s.requireAuth(s.handleRun))
	s.mux.HandleFunc("POST /interests/add", s.requireAuth(s.handleAddInterest))
	s.mux.HandleFunc("POST /interests/delete", s.requireAuth(s.handleDeleteInterest))
	s.mux.HandleFunc("POST /settings", s.requireAuth(s.handleSettingsPost))

	// JSON HTTP API (auth-free, single-user/local).
	s.registerAPI()
}

// requireAuth is a simple middleware to ensure the user is logged in
func (s *Server) requireAuth(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		session, _ := store.Get(r, "dailyread-session")
		if auth, ok := session.Values["authenticated"].(bool); !ok || !auth {
			http.Redirect(w, r, "/login", http.StatusSeeOther)
			return
		}
		next.ServeHTTP(w, r)
	}
}

func (s *Server) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	s.mux.ServeHTTP(w, r)
}
