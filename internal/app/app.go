package app

import (
	"context"
	"fmt"
	"net/http"
	"os"
	"time"

	"github.com/flakeguard/flakeguard/internal/config"
	"github.com/flakeguard/flakeguard/internal/db"
	"github.com/flakeguard/flakeguard/internal/web"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/rs/zerolog"
	"github.com/rs/zerolog/log"
)

// App holds the application state
type App struct {
	Config *config.Config
	DB     *pgxpool.Pool
	Router http.Handler
}

// New creates and initializes a new application instance
func New(ctx context.Context, cfg *config.Config) (*App, error) {
	// Initialize logger
	setupLogger(cfg.LogLevel)

	log.Info().Msg("Initializing FlakeGuard application")
	log.Info().Interface("config", cfg.RedactedValues()).Msg("Configuration loaded")

	// Connect to database
	log.Info().Msg("Connecting to database...")
	pool, err := db.Connect(ctx, cfg.DatabaseDSN)
	if err != nil {
		return nil, fmt.Errorf("failed to connect to database: %w", err)
	}
	log.Info().Msg("Database connection established")

	// Run migrations if in dev mode
	if cfg.IsDev() {
		log.Info().Msg("Development mode: running migrations automatically")
		if err := db.RunMigrations(ctx, pool); err != nil {
			pool.Close()
			return nil, fmt.Errorf("failed to run migrations: %w", err)
		}
	} else {
		log.Info().Msg("Production mode: migrations must be run manually")
	}

	// Initialize templates
	log.Info().Msg("Initializing templates")
	if err := web.InitTemplates("web/templates"); err != nil {
		pool.Close()
		return nil, fmt.Errorf("failed to initialize templates: %w", err)
	}

	// Setup router
	router := NewRouter(pool, cfg)

	app := &App{
		Config: cfg,
		DB:     pool,
		Router: router,
	}

	log.Info().Msg("Application initialized successfully")
	return app, nil
}

// Start starts the HTTP server
func (a *App) Start() error {
	addr := a.Config.HTTPAddr
	log.Info().Str("addr", addr).Msg("Starting HTTP server")

	server := &http.Server{
		Addr:         addr,
		Handler:      a.Router,
		ReadTimeout:  15 * time.Second,
		WriteTimeout: 15 * time.Second,
		IdleTimeout:  60 * time.Second,
	}

	return server.ListenAndServe()
}

// Close gracefully shuts down the application
func (a *App) Close() {
	log.Info().Msg("Shutting down application")
	if a.DB != nil {
		log.Info().Msg("Closing database connection")
		a.DB.Close()
	}
}

// setupLogger configures the global logger
func setupLogger(level string) {
	// Set up pretty console output for development
	log.Logger = log.Output(zerolog.ConsoleWriter{
		Out:        os.Stdout,
		TimeFormat: time.RFC3339,
	})

	// Set log level
	switch level {
	case "debug":
		zerolog.SetGlobalLevel(zerolog.DebugLevel)
	case "info":
		zerolog.SetGlobalLevel(zerolog.InfoLevel)
	case "warn":
		zerolog.SetGlobalLevel(zerolog.WarnLevel)
	case "error":
		zerolog.SetGlobalLevel(zerolog.ErrorLevel)
	default:
		zerolog.SetGlobalLevel(zerolog.InfoLevel)
	}

	log.Debug().Str("level", level).Msg("Logger configured")
}
