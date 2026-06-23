package db

import (
	"database/sql"
	"strings"

	"dailyread/internal/domain"
)

type Repository struct {
	db *sql.DB
}

func NewRepository(db *sql.DB) *Repository {
	return &Repository{db: db}
}

// User methods
func (r *Repository) CreateUser(u *domain.User) error {
	_, err := r.db.Exec(`
		INSERT INTO users (id, email, password_hash, created_at)
		VALUES (?, ?, ?, ?)
	`, u.ID, u.Email, u.PasswordHash, u.CreatedAt)
	return err
}

func (r *Repository) GetUserByEmail(email string) (*domain.User, error) {
	u := &domain.User{}
	err := r.db.QueryRow(`
		SELECT id, email, password_hash, created_at FROM users WHERE email = ?
	`, email).Scan(&u.ID, &u.Email, &u.PasswordHash, &u.CreatedAt)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return u, nil
}

// Config methods
func (r *Repository) CreateUserConfig(c *domain.UserConfig) error {
	_, err := r.db.Exec(`
		INSERT INTO user_configs (
			user_id, schedule_enabled, schedule_cron, schedule_timezone, smtp_host, smtp_port,
			smtp_user, smtp_pass_encrypted, openai_key_encrypted, models_provider
		) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
	`, c.UserID, c.ScheduleEnabled, c.ScheduleCron, c.ScheduleTimezone, c.SMTPHost, c.SMTPPort, c.SMTPUser, c.SMTPPassEncrypted, c.OpenAIKeyEncrypted, c.ModelsProvider)
	return err
}

func (r *Repository) GetUserConfig(userID string) (*domain.UserConfig, error) {
	c := &domain.UserConfig{}
	err := r.db.QueryRow(`
		SELECT user_id, schedule_enabled, schedule_cron, schedule_timezone, smtp_host, smtp_port,
			smtp_user, smtp_pass_encrypted, openai_key_encrypted, models_provider
		FROM user_configs WHERE user_id = ?
	`, userID).Scan(&c.UserID, &c.ScheduleEnabled, &c.ScheduleCron, &c.ScheduleTimezone, &c.SMTPHost, &c.SMTPPort, &c.SMTPUser, &c.SMTPPassEncrypted, &c.OpenAIKeyEncrypted, &c.ModelsProvider)
	
	if err == sql.ErrNoRows {
		// Return a default config if one doesn't exist yet
		return &domain.UserConfig{
			UserID:           userID,
			ScheduleEnabled:  false,
			ScheduleCron:     "0 9 * * 6",
			ScheduleTimezone: "UTC",
			ModelsProvider:   "openai",
		}, nil
	}
	return c, err
}

func (r *Repository) UpdateUserConfig(c *domain.UserConfig) error {
	_, err := r.db.Exec(`
		UPDATE user_configs SET
			schedule_enabled = ?, schedule_cron = ?, schedule_timezone = ?, smtp_host = ?, smtp_port = ?,
			smtp_user = ?, smtp_pass_encrypted = ?, openai_key_encrypted = ?, models_provider = ?
		WHERE user_id = ?
	`, c.ScheduleEnabled, c.ScheduleCron, c.ScheduleTimezone, c.SMTPHost, c.SMTPPort, c.SMTPUser, c.SMTPPassEncrypted, c.OpenAIKeyEncrypted, c.ModelsProvider, c.UserID)
	return err
}

// Interest methods
func (r *Repository) GetUserInterests(userID string) ([]domain.UserInterest, error) {
	rows, err := r.db.Query(`
		SELECT id, user_id, tag, intensity, is_primary, types FROM interests WHERE user_id = ?
	`, userID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var ints []domain.UserInterest
	for rows.Next() {
		var i domain.UserInterest
		var typesStr string
		if err := rows.Scan(&i.ID, &i.UserID, &i.Tag, &i.Intensity, &i.IsPrimary, &typesStr); err != nil {
			return nil, err
		}
		if typesStr != "" {
			i.Types = strings.Split(typesStr, ",")
		}
		ints = append(ints, i)
	}
	return ints, nil
}

func (r *Repository) CreateInterest(i *domain.UserInterest) error {
	typesStr := strings.Join(i.Types, ",")
	_, err := r.db.Exec(`
		INSERT INTO interests (id, user_id, tag, intensity, is_primary, types)
		VALUES (?, ?, ?, ?, ?, ?)
	`, i.ID, i.UserID, i.Tag, i.Intensity, i.IsPrimary, typesStr)
	return err
}

func (r *Repository) DeleteInterest(id string, userID string) error {
	_, err := r.db.Exec(`DELETE FROM interests WHERE id = ? AND user_id = ?`, id, userID)
	return err
}

func (r *Repository) SetPrimaryInterest(id string, userID string) error {
	tx, err := r.db.Begin()
	if err != nil {
		return err
	}
	
	// Reset all
	if _, err := tx.Exec(`UPDATE interests SET is_primary = 0 WHERE user_id = ?`, userID); err != nil {
		tx.Rollback()
		return err
	}
	
	// Set one
	if _, err := tx.Exec(`UPDATE interests SET is_primary = 1 WHERE id = ? AND user_id = ?`, id, userID); err != nil {
		tx.Rollback()
		return err
	}
	
	return tx.Commit()
}

// Global methods for scheduler
func (r *Repository) GetAllUserIDs() ([]string, error) {
	rows, err := r.db.Query(`SELECT id FROM users`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	
	var ids []string
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			return nil, err
		}
		ids = append(ids, id)
	}
	return ids, nil
}
