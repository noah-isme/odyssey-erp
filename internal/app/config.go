package app

import (
	"errors"
	"time"

	"github.com/kelseyhightower/envconfig"
)

// Config holds runtime configuration for the application.
type Config struct {
	AppEnv            string        `envconfig:"APP_ENV" default:"development"`
	AppAddr           string        `envconfig:"APP_ADDR" default:":8080"`
	AppReadTimeout    time.Duration `envconfig:"APP_READ_TIMEOUT" default:"15s"`
	AppWriteTimeout   time.Duration `envconfig:"APP_WRITE_TIMEOUT" default:"15s"`
	AppRequestTimeout time.Duration `envconfig:"APP_REQUEST_TIMEOUT" default:"30s"`

	LogFormat string `envconfig:"LOG_FORMAT" default:"pretty"`

	PGDSN string `envconfig:"PG_DSN" default:"postgres://odyssey:odyssey@localhost:5432/odyssey?sslmode=disable"`

	RedisAddr     string        `envconfig:"REDIS_ADDR" default:"127.0.0.1:6379"`
	SessionSecret string        `envconfig:"SESSION_SECRET" required:"true"`
	SessionTTL    time.Duration `envconfig:"SESSION_TTL" default:"720h"`

	CSRFSecret string `envconfig:"CSRF_SECRET" required:"true"`

	SMTPHost string `envconfig:"SMTP_HOST" default:"127.0.0.1"`
	SMTPPort int    `envconfig:"SMTP_PORT" default:"1025"`
	SMTPFrom string `envconfig:"SMTP_FROM" default:"no-reply@odyssey.local"`

	GotenbergURL string `envconfig:"GOTENBERG_URL" default:"http://127.0.0.1:3000"`
}

// LoadConfig reads configuration from environment variables.
func LoadConfig() (*Config, error) {
	var cfg Config
	if err := envconfig.Process("", &cfg); err != nil {
		return nil, err
	}
	if cfg.SessionSecret == "" {
		return nil, errors.New("session secret must be provided")
	}
	if cfg.CSRFSecret == "" {
		return nil, errors.New("csrf secret must be provided")
	}
	return &cfg, nil
}

// IsProduction returns true when the application runs in production.
func (c *Config) IsProduction() bool {
	return c != nil && c.AppEnv == "production"
}
