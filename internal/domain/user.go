package domain

import (
	"time"
)

type User struct {
	ID           string
	Email        string
	PasswordHash string
	CreatedAt    time.Time
}

type UserConfig struct {
	UserID             string
	ScheduleEnabled    bool
	ScheduleCron       string
	ScheduleTimezone   string
	SMTPHost           string
	SMTPPort           int
	SMTPUser           string
	SMTPPassEncrypted  string
	OpenAIKeyEncrypted string
	ModelsProvider     string
}

type UserInterest struct {
	ID        string
	UserID    string
	Tag       string
	Intensity string
	IsPrimary bool
	Types     []string
}
