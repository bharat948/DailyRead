package web

import (
	"log/slog"
	"net/http"
	"strings"

	"dailyread/internal/config"
)

type TemplateData struct {
	Config      *config.Config
	Flash       string
	FlashIsError bool
}

// setFlash handles a simple cookie-based flash message for UX
func setFlash(w http.ResponseWriter, message string, isError bool) {
	val := message
	if isError {
		val = "ERROR:" + message
	}
	http.SetCookie(w, &http.Cookie{
		Name:     "flash",
		Value:    val,
		Path:     "/",
		MaxAge:   10,
		HttpOnly: true,
	})
}

func getFlash(w http.ResponseWriter, r *http.Request) (string, bool) {
	c, err := r.Cookie("flash")
	if err != nil {
		return "", false
	}
	// Clear the cookie
	http.SetCookie(w, &http.Cookie{
		Name:     "flash",
		Value:    "",
		Path:     "/",
		MaxAge:   -1,
		HttpOnly: true,
	})
	
	val := c.Value
	if strings.HasPrefix(val, "ERROR:") {
		return strings.TrimPrefix(val, "ERROR:"), true
	}
	return val, false
}

func (s *Server) handleIndex(w http.ResponseWriter, r *http.Request) {
	cfg := s.loader.Get()
	flash, isErr := getFlash(w, r)
	
	data := TemplateData{
		Config:      cfg,
		Flash:       flash,
		FlashIsError: isErr,
	}

	if err := s.tmpl.ExecuteTemplate(w, "index.html", data); err != nil {
		slog.Error("Failed to execute template", "error", err)
		http.Error(w, "Internal Server Error", http.StatusInternalServerError)
	}
}

func (s *Server) handleRun(w http.ResponseWriter, r *http.Request) {
	if s.triggerRun != nil {
		go s.triggerRun() // Execute asynchronously
	}
	
	setFlash(w, "Pipeline execution started in the background! Check console logs.", false)
	http.Redirect(w, r, "/", http.StatusSeeOther)
}

func (s *Server) handleAddInterest(w http.ResponseWriter, r *http.Request) {
	if err := r.ParseForm(); err != nil {
		setFlash(w, "Invalid form data", true)
		http.Redirect(w, r, "/", http.StatusSeeOther)
		return
	}

	tag := strings.TrimSpace(r.FormValue("tag"))
	intensity := r.FormValue("intensity")
	typesStr := r.FormValue("types")
	
	if tag == "" || intensity == "" {
		setFlash(w, "Tag and Intensity are required", true)
		http.Redirect(w, r, "/", http.StatusSeeOther)
		return
	}

	var types []string
	for _, t := range strings.Split(typesStr, ",") {
		tt := strings.TrimSpace(t)
		if tt != "" {
			types = append(types, tt)
		}
	}

	// Update config
	cfg := s.loader.Get()
	
	// Check if exists
	for _, inc := range cfg.Interests {
		if inc.Tag == tag {
			setFlash(w, "Interest tag already exists", true)
			http.Redirect(w, r, "/", http.StatusSeeOther)
			return
		}
	}

	newInterest := config.InterestConfig{
		Tag:       tag,
		Intensity: intensity,
		Types:     types,
		Primary:   false, // New ones default to false safely
	}

	cfg.Interests = append(cfg.Interests, newInterest)
	
	if err := s.loader.Save(); err != nil {
		slog.Error("Failed to save config", "error", err)
		setFlash(w, "Failed to save configuration", true)
	} else {
		setFlash(w, "Interest added successfully", false)
	}

	http.Redirect(w, r, "/", http.StatusSeeOther)
}

func (s *Server) handleDeleteInterest(w http.ResponseWriter, r *http.Request) {
	tag := r.FormValue("tag")
	if tag == "" {
		http.Redirect(w, r, "/", http.StatusSeeOther)
		return
	}

	cfg := s.loader.Get()
	
	var updated []config.InterestConfig
	deleted := false
	
	for _, inc := range cfg.Interests {
		if inc.Tag == tag {
			if inc.Primary {
				setFlash(w, "Cannot delete the primary interest", true)
				http.Redirect(w, r, "/", http.StatusSeeOther)
				return
			}
			deleted = true
			continue
		}
		updated = append(updated, inc)
	}

	if deleted {
		cfg.Interests = updated
		if err := s.loader.Save(); err != nil {
			slog.Error("Failed to save config", "error", err)
			setFlash(w, "Failed to save configuration", true)
		} else {
			setFlash(w, "Interest deleted successfully", false)
		}
	}

	http.Redirect(w, r, "/", http.StatusSeeOther)
}
