package web

import (
	"embed"
	"fmt"
	"html/template"
	"net/http"

	"dailyread/internal/config"
)

//go:embed templates/*
var templateFS embed.FS

type Server struct {
	loader *config.Loader
	tmpl   *template.Template
	mux    *http.ServeMux
	
	// Inject the pipeline runner function so we can trigger it from the web
	triggerRun func()
}

func NewServer(loader *config.Loader, triggerRun func()) (*Server, error) {
	tmpl, err := template.ParseFS(templateFS, "templates/*.html")
	if err != nil {
		return nil, fmt.Errorf("failed to parse templates: %w", err)
	}

	s := &Server{
		loader:     loader,
		tmpl:       tmpl,
		mux:        http.NewServeMux(),
		triggerRun: triggerRun,
	}

	s.routes()
	return s, nil
}

func (s *Server) routes() {
	s.mux.HandleFunc("GET /", s.handleIndex)
	s.mux.HandleFunc("POST /run", s.handleRun)
	s.mux.HandleFunc("POST /interests/add", s.handleAddInterest)
	s.mux.HandleFunc("POST /interests/delete", s.handleDeleteInterest)
}

func (s *Server) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	s.mux.ServeHTTP(w, r)
}
