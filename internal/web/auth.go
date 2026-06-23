package web

import (
	"log/slog"
	"net/http"
	"strings"
	"time"

	"dailyread/internal/domain"
	"github.com/google/uuid"
	"golang.org/x/crypto/bcrypt"
)

func (s *Server) handleLoginForm(w http.ResponseWriter, r *http.Request) {
	flash, isErr := getFlash(w, r)
	if err := s.tmpl.ExecuteTemplate(w, "login.html", map[string]interface{}{
		"Flash":        flash,
		"FlashIsError": isErr,
	}); err != nil {
		slog.Error("Template err", "err", err)
	}
}

func (s *Server) handleLoginPost(w http.ResponseWriter, r *http.Request) {
	email := strings.ToLower(strings.TrimSpace(r.FormValue("email")))
	password := r.FormValue("password")

	u, err := s.repo.GetUserByEmail(email)
	if err != nil {
		slog.Error("DB err during login", "err", err)
		setFlash(w, "An error occurred", true)
		http.Redirect(w, r, "/login", http.StatusSeeOther)
		return
	}

	if u == nil {
		setFlash(w, "Invalid credentials", true)
		http.Redirect(w, r, "/login", http.StatusSeeOther)
		return
	}

	if err := bcrypt.CompareHashAndPassword([]byte(u.PasswordHash), []byte(password)); err != nil {
		setFlash(w, "Invalid credentials", true)
		http.Redirect(w, r, "/login", http.StatusSeeOther)
		return
	}

	// Success
	session, _ := store.Get(r, "dailyread-session")
	session.Values["authenticated"] = true
	session.Values["user_id"] = u.ID
	session.Save(r, w)

	http.Redirect(w, r, "/", http.StatusSeeOther)
}

func (s *Server) handleRegisterForm(w http.ResponseWriter, r *http.Request) {
	flash, isErr := getFlash(w, r)
	if err := s.tmpl.ExecuteTemplate(w, "register.html", map[string]interface{}{
		"Flash":        flash,
		"FlashIsError": isErr,
	}); err != nil {
		slog.Error("Template err", "err", err)
	}
}

func (s *Server) handleRegisterPost(w http.ResponseWriter, r *http.Request) {
	email := strings.ToLower(strings.TrimSpace(r.FormValue("email")))
	password := r.FormValue("password")

	if email == "" || len(password) < 6 {
		setFlash(w, "Invalid input. Password must be >5 chars.", true)
		http.Redirect(w, r, "/register", http.StatusSeeOther)
		return
	}

	// Check existing
	existing, _ := s.repo.GetUserByEmail(email)
	if existing != nil {
		setFlash(w, "Email already in use", true)
		http.Redirect(w, r, "/register", http.StatusSeeOther)
		return
	}

	hash, err := bcrypt.GenerateFromPassword([]byte(password), bcrypt.DefaultCost)
	if err != nil {
		setFlash(w, "Encryption error", true)
		http.Redirect(w, r, "/register", http.StatusSeeOther)
		return
	}

	id := uuid.New().String()
	u := &domain.User{
		ID:           id,
		Email:        email,
		PasswordHash: string(hash),
		CreatedAt:    time.Now(),
	}

	if err := s.repo.CreateUser(u); err != nil {
		slog.Error("Failed to create user", "err", err)
		setFlash(w, "Failed to create user", true)
		http.Redirect(w, r, "/register", http.StatusSeeOther)
		return
	}
	
	// Create default config
	c := &domain.UserConfig{
		UserID:           id,
		ScheduleCron:     "0 9 * * 6",
		ScheduleTimezone: "UTC",
		ModelsProvider:   "openai",
	}
	s.repo.CreateUserConfig(c)

	// Auto-login
	session, _ := store.Get(r, "dailyread-session")
	session.Values["authenticated"] = true
	session.Values["user_id"] = id
	session.Save(r, w)

	setFlash(w, "Registration successful! Welcome to DailyRead.", false)
	http.Redirect(w, r, "/", http.StatusSeeOther)
}

func (s *Server) handleLogout(w http.ResponseWriter, r *http.Request) {
	session, _ := store.Get(r, "dailyread-session")
	session.Values["authenticated"] = false
	session.Values["user_id"] = ""
	session.Options.MaxAge = -1
	session.Save(r, w)
	http.Redirect(w, r, "/login", http.StatusSeeOther)
}
