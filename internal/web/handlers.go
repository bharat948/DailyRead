package web

import (
	"log/slog"
	"net/http"
	"strings"

	"dailyread/internal/domain"
	"github.com/google/uuid"
)

type TemplateData struct {
	User         *domain.User
	Config       *domain.UserConfig
	Interests    []domain.UserInterest
	Flash        string
	FlashIsError bool
	ScheduleDay  string
	ScheduleTime string
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

func (s *Server) getUserSession(r *http.Request) string {
	session, _ := store.Get(r, "dailyread-session")
	userID, _ := session.Values["user_id"].(string)
	return userID
}

func (s *Server) handleIndex(w http.ResponseWriter, r *http.Request) {
	userID := s.getUserSession(r)
	if userID == "" {
		http.Redirect(w, r, "/login", http.StatusSeeOther)
		return
	}

	// Let's implement GetUserByID if needed. For now just config.
	cfg, err := s.repo.GetUserConfig(userID)
	if err != nil {
		slog.Error("Failed to fetch user config", "err", err)
		http.Error(w, "Internal error", 500)
		return
	}

	interests, _ := s.repo.GetUserInterests(userID)

	flash, isErr := getFlash(w, r)
	
	// Parse cron for UI
	scheduleDay := "6"
	scheduleTime := "09:00"
	if cfg != nil && cfg.ScheduleCron != "" {
		parts := strings.Split(cfg.ScheduleCron, " ")
		if len(parts) >= 5 {
			scheduleDay = parts[4]
			minute := parts[0]
			hour := parts[1]
			if len(minute) == 1 { minute = "0" + minute }
			if len(hour) == 1 { hour = "0" + hour }
			scheduleTime = hour + ":" + minute
		}
	}

	data := TemplateData{
		Config:       cfg,
		Interests:    interests,
		Flash:        flash,
		FlashIsError: isErr,
		ScheduleDay:  scheduleDay,
		ScheduleTime: scheduleTime,
	}

	if err := s.tmpl.ExecuteTemplate(w, "index.html", data); err != nil {
		slog.Error("Failed to execute template", "error", err)
	}
}

func (s *Server) handleRun(w http.ResponseWriter, r *http.Request) {
	userID := s.getUserSession(r)
	if s.pipe != nil && userID != "" {
		if _, err := s.pipe.TriggerAsync(userID, "manual"); err != nil {
			slog.Error("failed to trigger run from dashboard", "user_id", userID, "error", err)
		}
	}

	setFlash(w, "Pipeline execution started in the background! Check console logs.", false)
	http.Redirect(w, r, "/", http.StatusSeeOther)
}

func (s *Server) handleAddInterest(w http.ResponseWriter, r *http.Request) {
	userID := s.getUserSession(r)
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

	i := &domain.UserInterest{
		ID:        uuid.New().String(),
		UserID:    userID,
		Tag:       tag,
		Intensity: intensity,
		Types:     types,
		IsPrimary: false,
	}

	if err := s.repo.CreateInterest(i); err != nil {
		slog.Error("Failed to add interest", "err", err)
		setFlash(w, "Failed to add interest", true)
	} else {
		setFlash(w, "Interest added successfully", false)
	}

	http.Redirect(w, r, "/", http.StatusSeeOther)
}

func (s *Server) handleDeleteInterest(w http.ResponseWriter, r *http.Request) {
	userID := s.getUserSession(r)
	id := r.FormValue("id")
	if id == "" {
		http.Redirect(w, r, "/", http.StatusSeeOther)
		return
	}

	if err := s.repo.DeleteInterest(id, userID); err != nil {
		setFlash(w, "Failed to delete interest", true)
	} else {
		setFlash(w, "Interest removed", false)
	}

	http.Redirect(w, r, "/", http.StatusSeeOther)
}

func (s *Server) handleSettingsPost(w http.ResponseWriter, r *http.Request) {
	userID := s.getUserSession(r)
	if err := r.ParseForm(); err != nil {
		http.Redirect(w, r, "/", http.StatusSeeOther)
		return
	}

	cfg, _ := s.repo.GetUserConfig(userID)
	if cfg == nil {
		cfg = &domain.UserConfig{UserID: userID}
	}

	cfg.ScheduleEnabled = r.FormValue("schedule_enabled") == "on"
	
	day := r.FormValue("schedule_day")
	timeStr := r.FormValue("schedule_time")
	if day != "" && timeStr != "" {
		parts := strings.Split(timeStr, ":")
		if len(parts) == 2 {
			cfg.ScheduleCron = parts[1] + " " + parts[0] + " * * " + day
		}
	}

	cfg.ScheduleTimezone = r.FormValue("schedule_timezone")
	cfg.ModelsProvider = r.FormValue("models_provider")
	
	// Optional BYOC credentials
	if val := r.FormValue("openai_key"); val != "" {
		cfg.OpenAIKeyEncrypted = val // TODO: Encrypt at rest in production
	}
	if val := r.FormValue("smtp_host"); val != "" {
		cfg.SMTPHost = val
	}
	if val := r.FormValue("smtp_user"); val != "" {
		cfg.SMTPUser = val
	}
	if val := r.FormValue("smtp_pass"); val != "" {
		cfg.SMTPPassEncrypted = val // TODO: Encrypt at rest
	}

	if err := s.repo.UpdateUserConfig(cfg); err != nil {
		slog.Error("Failed to update settings", "err", err)
		setFlash(w, "Failed to save settings", true)
	} else {
		if s.updateSchedule != nil {
			s.updateSchedule(userID, cfg.ScheduleEnabled, cfg.ScheduleCron, cfg.ScheduleTimezone)
		}
		setFlash(w, "Settings saved successfully", false)
	}

	http.Redirect(w, r, "/", http.StatusSeeOther)
}
