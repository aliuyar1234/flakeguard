package main

import (
	"context"
	"database/sql"
	"fmt"
	"os"
	"os/signal"
	"syscall"

	"github.com/flakeguard/flakeguard/internal/app"
	"github.com/flakeguard/flakeguard/internal/config"
	"github.com/flakeguard/flakeguard/internal/retention"
	"github.com/joho/godotenv"
	_ "github.com/lib/pq"
	"github.com/robfig/cron/v3"
	"github.com/rs/zerolog/log"
)

func main() {
	// Load .env file if it exists (development mode)
	if err := godotenv.Load(); err != nil {
		// It's okay if .env doesn't exist (production uses real env vars)
		log.Debug().Err(err).Msg("No .env file found, using system environment variables")
	}

	// Load configuration
	cfg, err := config.Load()
	if err != nil {
		fmt.Fprintf(os.Stderr, "Configuration error: %v\n", err)
		os.Exit(1)
	}

	// Create application context
	ctx := context.Background()

	// Initialize application
	application, err := app.New(ctx, cfg)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Failed to initialize application: %v\n", err)
		os.Exit(1)
	}
	defer application.Close()

	// Setup retention job cron scheduler
	cronScheduler, err := setupRetentionCron(cfg)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Failed to setup retention cron: %v\n", err)
		os.Exit(1)
	}
	cronScheduler.Start()
	defer cronScheduler.Stop()

	// Setup signal handling for graceful shutdown
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, os.Interrupt, syscall.SIGTERM)

	// Start server in a goroutine
	errChan := make(chan error, 1)
	go func() {
		if err := application.Start(); err != nil {
			errChan <- err
		}
	}()

	// Wait for shutdown signal or error
	select {
	case err := <-errChan:
		log.Error().Err(err).Msg("Server error")
		os.Exit(1)
	case sig := <-sigChan:
		log.Info().Str("signal", sig.String()).Msg("Received shutdown signal")
		application.Close()
	}
}

// setupRetentionCron configures the cron scheduler for retention jobs
func setupRetentionCron(cfg *config.Config) (*cron.Cron, error) {
	// Create cron scheduler with logger
	cronLogger := cron.VerbosePrintfLogger(&log.Logger)
	c := cron.New(cron.WithLogger(cronLogger))

	// Create database connection for retention job (separate from connection pool)
	// This ensures retention job doesn't interfere with API requests
	db, err := sql.Open("postgres", cfg.DatabaseDSN)
	if err != nil {
		return nil, fmt.Errorf("failed to create database connection for retention: %w", err)
	}

	// Configure connection pool for retention job
	db.SetMaxOpenConns(2)
	db.SetMaxIdleConns(1)

	// Determine cron schedule based on environment
	var schedule string
	if cfg.IsDev() {
		// Development: run every minute for testing
		schedule = "* * * * *"
		log.Info().Msg("Retention job scheduled: every minute (development mode)")
	} else {
		// Production: run daily at 03:00 UTC
		schedule = "0 3 * * *"
		log.Info().Msg("Retention job scheduled: daily at 03:00 UTC (production mode)")
	}

	// Add retention job with panic recovery
	_, err = c.AddFunc(schedule, func() {
		defer func() {
			if r := recover(); r != nil {
				log.Error().Interface("panic", r).Msg("Retention job panicked")
			}
		}()

		ctx := context.Background()
		err := retention.RunRetentionJob(
			ctx,
			db,
			cfg.RetentionJunitDays,
			cfg.RetentionEventsDays,
		)
		if err != nil {
			log.Error().Err(err).Msg("Retention job failed")
		}
	})

	if err != nil {
		return nil, fmt.Errorf("failed to schedule retention job: %w", err)
	}

	log.Info().
		Int("junit_retention_days", cfg.RetentionJunitDays).
		Int("events_retention_days", cfg.RetentionEventsDays).
		Msg("Retention job configured")

	return c, nil
}
