package delivery

import (
	"fmt"
	"log/slog"
	"net/smtp"
	"os"

	"dailyread/internal/config"
)

type Sender interface {
	Send(digest *Digest) error
}

type SMTPSender struct {
	cfg config.EmailConfig
	to  string
}

func NewSender(cfg config.EmailConfig, user config.UserConfig) Sender {
	if cfg.Channel != "smtp" {
		slog.Warn("Unsupported email channel, falling back to DryRunSender", "channel", cfg.Channel)
		return &DryRunSender{}
	}

	// Verify required env vars
	if os.Getenv("SMTP_HOST") == "" || os.Getenv("SMTP_PORT") == "" {
		slog.Warn("SMTP credentials not fully configured in env, falling back to DryRunSender")
		return &DryRunSender{}
	}

	return &SMTPSender{
		cfg: cfg,
		to:  user.Email,
	}
}

func (s *SMTPSender) Send(digest *Digest) error {
	host := os.Getenv("SMTP_HOST")
	port := os.Getenv("SMTP_PORT")
	user := os.Getenv("SMTP_USER")
	pass := os.Getenv("SMTP_PASS")

	slog.Info("Sending digest via SMTP", "to", s.to, "host", host)

	addr := fmt.Sprintf("%s:%s", host, port)
	var auth smtp.Auth
	if user != "" && pass != "" {
		auth = smtp.PlainAuth("", user, pass, host)
	}

	// Construct MIME email with HTML
	headers := "MIME-version: 1.0;\nContent-Type: text/html; charset=\"UTF-8\";\n\n"
	msg := fmt.Sprintf("To: %s\r\nSubject: %s\r\n%s\r\n%s", s.to, digest.Subject, headers, digest.HTML)

	err := smtp.SendMail(addr, auth, user, []string{s.to}, []byte(msg))
	if err != nil {
		return fmt.Errorf("smtp send failed: %w", err)
	}

	slog.Info("Digest sent successfully")
	return nil
}

type DryRunSender struct{}

func (s *DryRunSender) Send(digest *Digest) error {
	slog.Info("=== DRY RUN DELIVERY ===")
	fmt.Println("\n" + digest.Text)
	slog.Info("=== END DRY RUN ===")
	return nil
}
