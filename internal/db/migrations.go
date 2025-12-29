package db

import (
	"context"
	"embed"
	"fmt"
	"sort"
	"strings"

	"github.com/aliuyar1234/flakeguard/migrations"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/rs/zerolog/log"
)

var migrationsFS embed.FS = migrations.FS

// RunMigrations applies all pending database migrations
func RunMigrations(ctx context.Context, pool *pgxpool.Pool) error {
	log.Info().Msg("Running database migrations...")

	// Create migrations tracking table if it doesn't exist
	if err := createMigrationsTable(ctx, pool); err != nil {
		return fmt.Errorf("failed to create migrations table: %w", err)
	}

	// Get list of migration files
	migrations, err := getMigrationFiles()
	if err != nil {
		return fmt.Errorf("failed to read migration files: %w", err)
	}

	// Get already applied migrations
	applied, err := getAppliedMigrations(ctx, pool)
	if err != nil {
		return fmt.Errorf("failed to get applied migrations: %w", err)
	}

	// Apply pending migrations
	for _, migration := range migrations {
		if applied[migration] {
			log.Debug().Str("migration", migration).Msg("Migration already applied, skipping")
			continue
		}

		log.Info().Str("migration", migration).Msg("Applying migration")
		if err := applyMigration(ctx, pool, migration); err != nil {
			return fmt.Errorf("failed to apply migration %s: %w", migration, err)
		}
	}

	log.Info().Msg("All migrations applied successfully")
	return nil
}

// createMigrationsTable creates the schema_migrations table if it doesn't exist
func createMigrationsTable(ctx context.Context, pool *pgxpool.Pool) error {
	query := `
		CREATE TABLE IF NOT EXISTS schema_migrations (
			version VARCHAR(255) PRIMARY KEY,
			applied_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
		)
	`
	_, err := pool.Exec(ctx, query)
	return err
}

// getMigrationFiles returns a sorted list of migration file names
func getMigrationFiles() ([]string, error) {
	entries, err := migrationsFS.ReadDir(".")
	if err != nil {
		return nil, err
	}

	var migrations []string
	for _, entry := range entries {
		if !entry.IsDir() && strings.HasSuffix(entry.Name(), ".sql") {
			migrations = append(migrations, entry.Name())
		}
	}

	sort.Strings(migrations)
	return migrations, nil
}

// getAppliedMigrations returns a set of already applied migration versions
func getAppliedMigrations(ctx context.Context, pool *pgxpool.Pool) (map[string]bool, error) {
	rows, err := pool.Query(ctx, "SELECT version FROM schema_migrations")
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	applied := make(map[string]bool)
	for rows.Next() {
		var version string
		if err := rows.Scan(&version); err != nil {
			return nil, err
		}
		applied[version] = true
	}

	return applied, rows.Err()
}

// applyMigration applies a single migration file
func applyMigration(ctx context.Context, pool *pgxpool.Pool, migration string) error {
	// Read migration file
	content, err := migrationsFS.ReadFile(migration)
	if err != nil {
		return err
	}

	conn, err := pool.Acquire(ctx)
	if err != nil {
		return err
	}
	defer conn.Release()

	// Use the simple query protocol to execute multi-statement SQL migrations.
	_, err = conn.Conn().PgConn().Exec(ctx, string(content)).ReadAll()
	if err != nil {
		return err
	}

	// Record migration as applied
	_, err = pool.Exec(ctx, "INSERT INTO schema_migrations (version) VALUES ($1)", migration)
	return err
}
