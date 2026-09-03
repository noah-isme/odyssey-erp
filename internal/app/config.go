package app

import (
	"errors"
	"os"
	"strings"
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
	WorkerMetricsAddr string        `envconfig:"WORKER_METRICS_ADDR" default:":9091"`

	LogFormat string `envconfig:"LOG_FORMAT" default:"pretty"`

	// ReleaseProfile controls the route surface exposed by a deployment.
	// Development defaults to the complete local surface; staging and production
	// must set it explicitly so certification cannot silently drift.
	ReleaseProfile string `envconfig:"RELEASE_PROFILE" default:"full"`

	PGDSN string `envconfig:"PG_DSN" default:"postgres://odyssey:odyssey@localhost:5432/odyssey?sslmode=disable"`

	RedisAddr     string        `envconfig:"REDIS_ADDR" default:"127.0.0.1:6379"`
	SessionSecret string        `envconfig:"SESSION_SECRET" required:"true"`
	SessionTTL    time.Duration `envconfig:"SESSION_TTL" default:"720h"`

	CSRFSecret string `envconfig:"CSRF_SECRET" required:"true"`

	// ConnectorDevelopmentMode must be explicitly enabled before provider
	// adapters may use local/test fakes. It defaults to false so production
	// cannot silently accept simulated credentials or no-op success paths.
	ConnectorDevelopmentMode bool `envconfig:"CONNECTORS_DEVELOPMENT_MODE" default:"false"`

	SMTPHost     string `envconfig:"SMTP_HOST" default:"127.0.0.1"`
	SMTPPort     int    `envconfig:"SMTP_PORT" default:"1025"`
	SMTPFrom     string `envconfig:"SMTP_FROM" default:"no-reply@odyssey.local"`
	SMTPUsername string `envconfig:"SMTP_USERNAME"`
	SMTPPassword string `envconfig:"SMTP_PASSWORD"`

	GotenbergURL        string `envconfig:"GOTENBERG_URL" default:"http://127.0.0.1:3000"`
	BoardPackStorageDir string `envconfig:"BOARD_PACK_STORAGE" default:"./var/boardpacks"`

	BoardPackStorageDriver     string `envconfig:"BOARD_PACK_STORAGE_DRIVER" default:"local"`
	BoardPackS3Endpoint        string `envconfig:"BOARD_PACK_S3_ENDPOINT"`
	BoardPackS3Region          string `envconfig:"BOARD_PACK_S3_REGION" default:"us-east-1"`
	BoardPackS3Bucket          string `envconfig:"BOARD_PACK_S3_BUCKET" default:"odyssey-boardpacks"`
	BoardPackS3AccessKeyID     string `envconfig:"BOARD_PACK_S3_ACCESS_KEY_ID"`
	BoardPackS3SecretAccessKey string `envconfig:"BOARD_PACK_S3_SECRET_ACCESS_KEY"`
	BoardPackS3UsePathStyle    bool   `envconfig:"BOARD_PACK_S3_USE_PATH_STYLE" default:"true"`
	BoardPackS3AutoCreate      bool   `envconfig:"BOARD_PACK_S3_AUTO_CREATE_BUCKET" default:"false"`

	FXProvider     string        `envconfig:"FX_PROVIDER" default:"exchangerate-api"`
	FXAPIBaseURL   string        `envconfig:"FX_API_BASE_URL" default:"https://open.er-api.com/v6"`
	FXAPIKey       string        `envconfig:"FX_API_KEY"`
	FXBaseCurrency string        `envconfig:"FX_BASE_CURRENCY" default:"IDR"`
	FXFetchTimeout time.Duration `envconfig:"FX_FETCH_TIMEOUT" default:"10s"`
	FXMaxRateAge   time.Duration `envconfig:"FX_MAX_RATE_AGE" default:"48h"`
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
	if (cfg.AppEnv == "staging" || cfg.IsProduction()) && strings.TrimSpace(os.Getenv("RELEASE_PROFILE")) == "" {
		return nil, errors.New("RELEASE_PROFILE must be set explicitly for staging or production")
	}
	if _, err := ParseReleaseProfile(cfg.ReleaseProfile); err != nil {
		return nil, err
	}
	if cfg.GotenbergURL != "" && !strings.Contains(cfg.GotenbergURL, "://") {
		cfg.GotenbergURL = "http://" + cfg.GotenbergURL
	}
	return &cfg, nil
}

// IsProduction returns true when the application runs in production.
func (c *Config) IsProduction() bool {
	return c != nil && c.AppEnv == "production"
}
