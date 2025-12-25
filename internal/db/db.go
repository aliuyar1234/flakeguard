package db

import (
	"context"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
)

// Connect creates and returns a new PostgreSQL connection pool
func Connect(ctx context.Context, dsn string) (*pgxpool.Pool, error) {
	// Parse and configure the connection pool
	config, err := pgxpool.ParseConfig(dsn)
	if err != nil {
		return nil, fmt.Errorf("failed to parse database DSN: %w", err)
	}

	// Configure pool settings
	config.MaxConns = 25                      // Maximum connections in pool
	config.MinConns = 5                       // Minimum idle connections
	config.MaxConnLifetime = time.Hour        // Max lifetime of a connection
	config.MaxConnIdleTime = 30 * time.Minute // Max idle time before closing
	config.HealthCheckPeriod = time.Minute    // How often to check connection health

	// Create the connection pool
	pool, err := pgxpool.NewWithConfig(ctx, config)
	if err != nil {
		return nil, fmt.Errorf("failed to create connection pool: %w", err)
	}

	// Verify connectivity
	if err := pool.Ping(ctx); err != nil {
		pool.Close()
		return nil, fmt.Errorf("failed to ping database: %w", err)
	}

	return pool, nil
}

// Close gracefully closes the database connection pool
func Close(pool *pgxpool.Pool) {
	if pool != nil {
		pool.Close()
	}
}
