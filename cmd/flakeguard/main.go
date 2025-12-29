package main

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/aliuyar1234/flakeguard/internal/app"
	"github.com/aliuyar1234/flakeguard/internal/config"
	"github.com/aliuyar1234/flakeguard/internal/retention"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/joho/godotenv"
	"github.com/robfig/cron/v3"
	"github.com/rs/zerolog/log"
)

const (
	retentionJunitDays  = 30
	retentionEventsDays = 180
)

func main() {
        if len(os.Args) > 1 && os.Args[1] == "admin" {
                os.Exit(runAdmin(os.Args[2:]))
        }

        _ = godotenv.Load()

        cfg, err := config.Load()
        if err != nil {
                fmt.Fprintf(os.Stderr, "Configuration error: %v\n", err)
		os.Exit(1)
	}

	ctx := context.Background()
	application, err := app.New(ctx, cfg)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Failed to initialize application: %v\n", err)
		os.Exit(1)
	}

	cronScheduler, err := setupRetentionCron(cfg, application.DB)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Failed to setup retention cron: %v\n", err)
		os.Exit(1)
	}
	cronScheduler.Start()
	defer cronScheduler.Stop()

	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, os.Interrupt, syscall.SIGTERM)

	errChan := make(chan error, 1)
	go func() {
		errChan <- application.Start()
	}()

	select {
	case err := <-errChan:
		if err != nil && !errors.Is(err, http.ErrServerClosed) {
			log.Error().Err(err).Msg("Server error")
			os.Exit(1)
		}
	case sig := <-sigChan:
		log.Info().Str("signal", sig.String()).Msg("Received shutdown signal")
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		if err := application.Shutdown(shutdownCtx); err != nil {
			log.Error().Err(err).Msg("Shutdown failed")
			os.Exit(1)
		}
	}
}

func setupRetentionCron(cfg *config.Config, pool *pgxpool.Pool) (*cron.Cron, error) {
	c := cron.New(cron.WithLocation(time.UTC))

	schedule := "0 3 * * *"
	if cfg.IsDev() {
		schedule = "* * * * *"
	}

	_, err := c.AddFunc(schedule, func() {
		defer func() {
			if r := recover(); r != nil {
				log.Error().Interface("panic", r).Msg("Retention job panicked")
			}
		}()

		ctx := context.Background()
		if err := retention.RunRetentionJob(ctx, pool, retentionJunitDays, retentionEventsDays); err != nil {
			log.Error().Err(err).Msg("Retention job failed")
		}
	})
	if err != nil {
		return nil, fmt.Errorf("failed to schedule retention job: %w", err)
	}

	return c, nil
}
