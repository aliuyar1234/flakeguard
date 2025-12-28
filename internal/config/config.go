package config

import (
	"fmt"
	"os"
	"strconv"
	"strings"
)

// Config holds all application configuration.
type Config struct {
	Env      string
	HTTPAddr string
	BaseURL  string

	DBDSN     string
	JWTSecret string

	LogLevel string

	RateLimitRPM int

	MaxUploadBytes int64
	MaxUploadFiles int
	MaxFileBytes   int64

	SlackTimeoutMS int
	SessionDays    int
}

// Load reads configuration from environment variables.
func Load() (*Config, error) {
	cfg := &Config{}

	cfg.Env = strings.TrimSpace(os.Getenv("FG_ENV"))
	if cfg.Env == "" {
		return nil, fmt.Errorf("FG_ENV is required")
	}
	if cfg.Env != "dev" && cfg.Env != "prod" {
		return nil, fmt.Errorf("FG_ENV must be one of: dev, prod (got: %s)", cfg.Env)
	}

	cfg.HTTPAddr = getEnvOrDefault("FG_HTTP_ADDR", ":8080")

	cfg.BaseURL = strings.TrimRight(strings.TrimSpace(os.Getenv("FG_BASE_URL")), "/")
	if cfg.BaseURL == "" {
		return nil, fmt.Errorf("FG_BASE_URL is required")
	}

	cfg.DBDSN = strings.TrimSpace(os.Getenv("FG_DB_DSN"))
	if cfg.DBDSN == "" {
		return nil, fmt.Errorf("FG_DB_DSN is required")
	}

	cfg.JWTSecret = os.Getenv("FG_JWT_SECRET")
	if cfg.JWTSecret == "" {
		return nil, fmt.Errorf("FG_JWT_SECRET is required")
	}
	if cfg.Env == "prod" && len(cfg.JWTSecret) < 32 {
		return nil, fmt.Errorf("FG_JWT_SECRET must be at least 32 characters (currently %d)", len(cfg.JWTSecret))
	}

	cfg.LogLevel = getEnvOrDefault("FG_LOG_LEVEL", "info")
	switch cfg.LogLevel {
	case "debug", "info", "warn", "error":
	default:
		return nil, fmt.Errorf("FG_LOG_LEVEL must be one of: debug, info, warn, error (got: %s)", cfg.LogLevel)
	}

	var err error
	cfg.RateLimitRPM, err = getEnvIntOrDefault("FG_RATE_LIMIT_RPM", 120)
	if err != nil {
		return nil, err
	}

	cfg.MaxUploadBytes, err = getEnvInt64OrDefault("FG_MAX_UPLOAD_BYTES", 5*1024*1024)
	if err != nil {
		return nil, err
	}

	cfg.MaxUploadFiles, err = getEnvIntOrDefault("FG_MAX_UPLOAD_FILES", 20)
	if err != nil {
		return nil, err
	}

	cfg.MaxFileBytes, err = getEnvInt64OrDefault("FG_MAX_FILE_BYTES", 1*1024*1024)
	if err != nil {
		return nil, err
	}

	cfg.SlackTimeoutMS, err = getEnvIntOrDefault("FG_SLACK_TIMEOUT_MS", 2000)
	if err != nil {
		return nil, err
	}
	if cfg.SlackTimeoutMS <= 0 || cfg.SlackTimeoutMS > 30000 {
		return nil, fmt.Errorf("FG_SLACK_TIMEOUT_MS must be between 1 and 30000 (got: %d)", cfg.SlackTimeoutMS)
	}

	cfg.SessionDays, err = getEnvIntOrDefault("FG_SESSION_DAYS", 7)
	if err != nil {
		return nil, err
	}

	return cfg, nil
}

// IsDev returns true if running in development mode.
func (c *Config) IsDev() bool {
	return c.Env == "dev"
}

// RedactedValues returns a map of config values with secrets redacted.
func (c *Config) RedactedValues() map[string]string {
	return map[string]string{
		"FG_ENV":              c.Env,
		"FG_HTTP_ADDR":        c.HTTPAddr,
		"FG_BASE_URL":         c.BaseURL,
		"FG_DB_DSN":           redactDSN(c.DBDSN),
		"FG_JWT_SECRET":       "[REDACTED]",
		"FG_LOG_LEVEL":        c.LogLevel,
		"FG_RATE_LIMIT_RPM":   fmt.Sprintf("%d", c.RateLimitRPM),
		"FG_MAX_UPLOAD_BYTES": fmt.Sprintf("%d", c.MaxUploadBytes),
		"FG_MAX_UPLOAD_FILES": fmt.Sprintf("%d", c.MaxUploadFiles),
		"FG_MAX_FILE_BYTES":   fmt.Sprintf("%d", c.MaxFileBytes),
		"FG_SLACK_TIMEOUT_MS": fmt.Sprintf("%d", c.SlackTimeoutMS),
		"FG_SESSION_DAYS":     fmt.Sprintf("%d", c.SessionDays),
	}
}

func redactDSN(dsn string) string {
	if start := strings.Index(dsn, "://"); start != -1 {
		if end := strings.Index(dsn[start+3:], "@"); end != -1 {
			return dsn[:start+3] + "[REDACTED]" + dsn[start+3+end:]
		}
	}
	return dsn
}

func getEnvOrDefault(key, defaultValue string) string {
	value := strings.TrimSpace(os.Getenv(key))
	if value == "" {
		return defaultValue
	}
	return value
}

func getEnvIntOrDefault(key string, defaultValue int) (int, error) {
	value := strings.TrimSpace(os.Getenv(key))
	if value == "" {
		return defaultValue, nil
	}
	parsed, err := strconv.Atoi(value)
	if err != nil {
		return 0, fmt.Errorf("%s must be an integer (got: %q)", key, value)
	}
	return parsed, nil
}

func getEnvInt64OrDefault(key string, defaultValue int64) (int64, error) {
	value := strings.TrimSpace(os.Getenv(key))
	if value == "" {
		return defaultValue, nil
	}
	parsed, err := strconv.ParseInt(value, 10, 64)
	if err != nil {
		return 0, fmt.Errorf("%s must be an integer (got: %q)", key, value)
	}
	return parsed, nil
}
