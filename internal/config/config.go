// Package config loads and validates runtime configuration from the
// environment. Secrets are never hard-coded.
package config

import (
	"errors"
	"fmt"
	"os"
	"strconv"
	"strings"
	"time"
)

type Env string

const (
	Development Env = "development"
	Production  Env = "production"
)

type Config struct {
	Env         Env
	Port        string
	DatabaseURL string

	JWTSecret       []byte
	StudentTokenTTL time.Duration
	AdminSessionTTL time.Duration
	CookieSecure    bool
	AllowedOrigins  []string

	// Student sign-in. Google ID tokens must be issued for one of these
	// client IDs; Telegram Login payloads are verified with the bot token.
	GoogleClientIDs  []string
	TelegramBotToken string
	// The login bot's @username (without @), for Telegram's login widget.
	TelegramBotUsername string

	// Push notifications (Firebase Cloud Messaging): the service account
	// that sends them. Empty: no push.
	FirebaseProjectID   string
	FirebaseClientEmail string
	FirebasePrivateKey  string
	// FirestoreDatabase is the Firestore database for live group exam rooms
	// (FIRESTORE_DATABASE, default "(default)").
	FirestoreDatabase string

	// Media uploads go to Cloudinary when set, otherwise to local disk.
	CloudinaryURL string
	UploadDir     string
	PublicBaseURL string

	// Creates the first owner admin on startup when no admin exists yet.
	BootstrapAdminEmail    string
	BootstrapAdminPassword string

	// Outgoing email for password reset codes. Without SMTP_HOST, students
	// with a phone login (or no mail server) get their code from an admin.
	SMTPHost string
	SMTPPort string
	SMTPUser string
	SMTPPass string
	SMTPFrom string

	// Chapa online payments (https://developer.chapa.co). Without the
	// secret key students send payment proof for an admin to check.
	ChapaSecretKey     string
	ChapaWebhookSecret string
	ChapaBaseURL       string
	// Where Chapa sends the student after paying (the app's deep link).
	ChapaReturnURL string

	// AI features (tutor, study analysis, review plans, report summaries)
	// use the Anthropic Messages API. Without a key they are switched off.
	AnthropicAPIKey string
	AIModel         string
	AIBaseURL       string
	// Questions a student may ask the AI per day.
	AIDailyLimit int
}

func (c Config) ChapaEnabled() bool { return c.ChapaSecretKey != "" }

func (c Config) AIEnabled() bool { return c.AnthropicAPIKey != "" }

func (c Config) MailEnabled() bool { return c.SMTPHost != "" && c.SMTPFrom != "" }

func (c Config) IsDev() bool { return c.Env == Development }

func env(key, def string) string {
	if v := strings.TrimSpace(os.Getenv(key)); v != "" {
		return v
	}
	return def
}

func envInt(key string, def int) int {
	if n, err := strconv.Atoi(strings.TrimSpace(os.Getenv(key))); err == nil && n > 0 {
		return n
	}
	return def
}

func list(key string) []string {
	var out []string
	for _, p := range strings.Split(os.Getenv(key), ",") {
		if p = strings.TrimSpace(p); p != "" {
			out = append(out, p)
		}
	}
	return out
}

// Load reads configuration and fails fast on unsafe production settings.
func Load() (Config, error) {
	c := Config{
		Env:                    Env(env("APP_ENV", string(Development))),
		Port:                   env("PORT", "8080"),
		DatabaseURL:            os.Getenv("DATABASE_URL"),
		JWTSecret:              []byte(os.Getenv("JWT_SECRET")),
		StudentTokenTTL:        30 * 24 * time.Hour,
		AdminSessionTTL:        12 * time.Hour,
		AllowedOrigins:         list("CORS_ORIGINS"),
		GoogleClientIDs:        list("GOOGLE_CLIENT_IDS"),
		TelegramBotToken:       os.Getenv("TELEGRAM_LOGIN_BOT_TOKEN"),
		TelegramBotUsername:    strings.TrimPrefix(os.Getenv("TELEGRAM_LOGIN_BOT_USERNAME"), "@"),
		FirebaseProjectID:      os.Getenv("FIREBASE_PROJECT_ID"),
		FirebaseClientEmail:    os.Getenv("FIREBASE_CLIENT_EMAIL"),
		FirebasePrivateKey:     os.Getenv("FIREBASE_PRIVATE_KEY"),
		FirestoreDatabase:      os.Getenv("FIRESTORE_DATABASE"),
		CloudinaryURL:          os.Getenv("CLOUDINARY_URL"),
		UploadDir:              env("UPLOAD_DIR", "uploads"),
		PublicBaseURL:          strings.TrimRight(env("PUBLIC_BASE_URL", "http://localhost:8080"), "/"),
		BootstrapAdminEmail:    os.Getenv("BOOTSTRAP_ADMIN_EMAIL"),
		BootstrapAdminPassword: os.Getenv("BOOTSTRAP_ADMIN_PASSWORD"),
		SMTPHost:               os.Getenv("SMTP_HOST"),
		SMTPPort:               env("SMTP_PORT", "587"),
		SMTPUser:               os.Getenv("SMTP_USER"),
		SMTPPass:               os.Getenv("SMTP_PASS"),
		SMTPFrom:               os.Getenv("SMTP_FROM"),
		ChapaSecretKey:         os.Getenv("CHAPA_SECRET_KEY"),
		ChapaWebhookSecret:     os.Getenv("CHAPA_WEBHOOK_SECRET"),
		ChapaBaseURL:           strings.TrimRight(env("CHAPA_BASE_URL", "https://api.chapa.co"), "/"),
		ChapaReturnURL:         env("CHAPA_RETURN_URL", "victory://payment"),
		AnthropicAPIKey:        os.Getenv("ANTHROPIC_API_KEY"),
		AIModel:                env("AI_MODEL", "claude-sonnet-5-5"),
		AIBaseURL:              strings.TrimRight(env("AI_BASE_URL", "https://api.anthropic.com"), "/"),
		AIDailyLimit:           envInt("AI_DAILY_LIMIT", 40),
	}
	c.CookieSecure = !c.IsDev()

	if c.DatabaseURL == "" {
		return c, errors.New("DATABASE_URL is required")
	}
	switch c.Env {
	case Development:
		if len(c.JWTSecret) == 0 {
			c.JWTSecret = []byte("dev-only-secret-dev-only-secret-dev-only")
		}
		if len(c.AllowedOrigins) == 0 {
			c.AllowedOrigins = []string{"http://localhost:5173", "http://localhost:5174"}
		}
	case Production:
		if len(c.JWTSecret) < 32 {
			return c, errors.New("JWT_SECRET must be at least 32 characters in production")
		}
		if len(c.AllowedOrigins) == 0 {
			return c, errors.New("CORS_ORIGINS must list the admin dashboard origin(s) in production")
		}
	default:
		return c, fmt.Errorf("APP_ENV must be %q or %q", Development, Production)
	}
	return c, nil
}
