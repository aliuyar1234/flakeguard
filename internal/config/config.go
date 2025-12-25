package config

import (
	"fmt"
	"os"
	"strings"
)

// Config holds all application configuration
type Config struct {
	// DatabaseDSN is the PostgreSQL connection string (REQUIRED)
	DatabaseDSN string

	// JWTSecret is used for signing JWT tokens (REQUIRED, min 32 bytes)
	JWTSecret string

	// HTTPAddr is the address to bind the HTTP server to (default: :8080)
	HTTPAddr string

	// LogLevel controls logging verbosity (default: info)
	// Valid values: debug, info, warn, error
	LogLevel string

	// Env is the environment name (default: prod)
	// Valid values: dev, prod
	// When set to 'dev', migrations auto-run on startup
	Env string

	// SessionDays is the number of days a session remains valid (default: 7)
	SessionDays int

	// RateLimitRPM is the rate limit in requests per minute per API key (default: 120)
	RateLimitRPM int

	// MaxUploadBytes is the maximum total upload size in bytes (default: 5MB)
	MaxUploadBytes int64

	// MaxFileBytes is the maximum single file size in bytes (default: 1MB)
	MaxFileBytes int64

	// MaxUploadFiles is the maximum number of files per upload request (default: 20)
	MaxUploadFiles int

	// SlackTimeoutMS is the timeout for Slack webhook requests in milliseconds (default: 2000)
	SlackTimeoutMS int

	// BaseURL is the base URL for the FlakeGuard dashboard (required for Slack notifications)
	// Example: https://flakeguard.example.com
	BaseURL string

	// RetentionJunitDays is the number of days to retain JUnit file content (default: 30)
	// After this period, the content field is cleared to save storage
	RetentionJunitDays int

	// RetentionEventsDays is the number of days to retain flake events (default: 180)
	// After this period, flake_events rows are deleted (flake_stats preserved)
	RetentionEventsDays int
}

// Load reads configuration from environment variables
func Load() (*Config, error) {
	cfg := &Config{
		DatabaseDSN:         os.Getenv("FG_DATABASE_DSN"),
		JWTSecret:           os.Getenv("FG_JWT_SECRET"),
		HTTPAddr:            getEnvOrDefault("FG_HTTP_ADDR", ":8080"),
		LogLevel:            getEnvOrDefault("FG_LOG_LEVEL", "info"),
		Env:                 getEnvOrDefault("FG_ENV", "prod"),
		SessionDays:         getEnvIntOrDefault("FG_SESSION_DAYS", 7),
		RateLimitRPM:        getEnvIntOrDefault("FG_RATE_LIMIT_RPM", 120),
		MaxUploadBytes:      getEnvInt64OrDefault("FG_MAX_UPLOAD_BYTES", 5*1024*1024),  // 5MB
		MaxFileBytes:        getEnvInt64OrDefault("FG_MAX_FILE_BYTES", 1*1024*1024),    // 1MB
		MaxUploadFiles:      getEnvIntOrDefault("FG_MAX_UPLOAD_FILES", 20),             // 20 files
		SlackTimeoutMS:      getEnvIntOrDefault("FG_SLACK_TIMEOUT_MS", 2000),           // 2 seconds
		BaseURL:             getEnvOrDefault("FG_BASE_URL", ""),
		RetentionJunitDays:  getEnvIntOrDefault("FG_RETENTION_JUNIT_DAYS", 30),         // 30 days
		RetentionEventsDays: getEnvIntOrDefault("FG_RETENTION_EVENTS_DAYS", 180),       // 180 days
	}

	// Validate required fields
	if cfg.DatabaseDSN == "" {
		return nil, fmt.Errorf("FG_DATABASE_DSN is required")
	}

	if cfg.JWTSecret == "" {
		return nil, fmt.Errorf("FG_JWT_SECRET is required")
	}

	if len(cfg.JWTSecret) < 32 {
		return nil, fmt.Errorf("FG_JWT_SECRET must be at least 32 bytes (currently %d bytes)", len(cfg.JWTSecret))
	}

	// Validate log level
	validLogLevels := map[string]bool{
		"debug": true,
		"info":  true,
		"warn":  true,
		"error": true,
	}
	if !validLogLevels[cfg.LogLevel] {
		return nil, fmt.Errorf("FG_LOG_LEVEL must be one of: debug, info, warn, error (got: %s)", cfg.LogLevel)
	}

	// Validate environment
	validEnvs := map[string]bool{
		"dev":  true,
		"prod": true,
	}
	if !validEnvs[cfg.Env] {
		return nil, fmt.Errorf("FG_ENV must be one of: dev, prod (got: %s)", cfg.Env)
	}

	// Validate Slack timeout
	if cfg.SlackTimeoutMS <= 0 || cfg.SlackTimeoutMS > 30000 {
		return nil, fmt.Errorf("FG_SLACK_TIMEOUT_MS must be between 1 and 30000 (got: %d)", cfg.SlackTimeoutMS)
	}

	return cfg, nil
}

// IsDev returns true if running in development mode
func (c *Config) IsDev() bool {
	return c.Env == "dev"
}

// RedactedValues returns a map of config values with secrets redacted
// Useful for logging configuration at startup
func (c *Config) RedactedValues() map[string]string {
	baseURL := c.BaseURL
	if baseURL == "" {
		baseURL = "[not set]"
	}
	return map[string]string{
		"FG_DATABASE_DSN":          redactDSN(c.DatabaseDSN),
		"FG_JWT_SECRET":            "[REDACTED]",
		"FG_HTTP_ADDR":             c.HTTPAddr,
		"FG_LOG_LEVEL":             c.LogLevel,
		"FG_ENV":                   c.Env,
		"FG_SESSION_DAYS":          fmt.Sprintf("%d", c.SessionDays),
		"FG_RATE_LIMIT_RPM":        fmt.Sprintf("%d", c.RateLimitRPM),
		"FG_MAX_UPLOAD_BYTES":      fmt.Sprintf("%d", c.MaxUploadBytes),
		"FG_MAX_FILE_BYTES":        fmt.Sprintf("%d", c.MaxFileBytes),
		"FG_MAX_UPLOAD_FILES":      fmt.Sprintf("%d", c.MaxUploadFiles),
		"FG_SLACK_TIMEOUT_MS":      fmt.Sprintf("%d", c.SlackTimeoutMS),
		"FG_BASE_URL":              baseURL,
		"FG_RETENTION_JUNIT_DAYS":  fmt.Sprintf("%d", c.RetentionJunitDays),
		"FG_RETENTION_EVENTS_DAYS": fmt.Sprintf("%d", c.RetentionEventsDays),
	}
}

// redactDSN redacts password from database connection string
func redactDSN(dsn string) string {
	// Simple redaction: replace everything between :// and @ with [REDACTED]
	if start := strings.Index(dsn, "://"); start != -1 {
		if end := strings.Index(dsn[start+3:], "@"); end != -1 {
			return dsn[:start+3] + "[REDACTED]" + dsn[start+3+end:]
		}
	}
	return dsn
}

// getEnvOrDefault returns environment variable value or default if not set
func getEnvOrDefault(key, defaultValue string) string {
	if value := os.Getenv(key); value != "" {
		return value
	}
	return defaultValue
}

// getEnvIntOrDefault returns environment variable as int or default if not set or invalid
func getEnvIntOrDefault(key string, defaultValue int) int {
	if value := os.Getenv(key); value != "" {
		if intVal, err := fmt.Sscanf(value, "%d", new(int)); err == nil && intVal == 1 {
			var result int
			fmt.Sscanf(value, "%d", &result)
			return result
		}
	}
	return defaultValue
}

// getEnvInt64OrDefault returns environment variable as int64 or default if not set or invalid
func getEnvInt64OrDefault(key string, defaultValue int64) int64 {
	if value := os.Getenv(key); value != "" {
		var result int64
		if _, err := fmt.Sscanf(value, "%d", &result); err == nil {
			return result
		}
	}
	return defaultValue
}
