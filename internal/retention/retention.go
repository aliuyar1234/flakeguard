package retention

import (
	"context"
	"database/sql"
	"fmt"
	"time"

	"github.com/rs/zerolog/log"
)

// ClearOldJunitContent clears the content field from junit_files older than the specified days.
// This frees storage while preserving metadata (filename, test count, timestamps).
// The function is idempotent - safe to run repeatedly.
//
// Returns the number of rows updated.
func ClearOldJunitContent(ctx context.Context, db *sql.DB, retentionDays int) (int64, error) {
	query := `
		UPDATE junit_files
		SET content = NULL
		WHERE created_at < NOW() - INTERVAL '1 day' * $1
		  AND content IS NOT NULL
	`

	result, err := db.ExecContext(ctx, query, retentionDays)
	if err != nil {
		return 0, fmt.Errorf("failed to clear old junit content: %w", err)
	}

	rowsAffected, err := result.RowsAffected()
	if err != nil {
		return 0, fmt.Errorf("failed to get rows affected: %w", err)
	}

	return rowsAffected, nil
}

// DeleteOldFlakeEvents deletes flake_events rows older than the specified days.
// Note: This does NOT delete flake_stats - aggregated statistics are preserved.
// The function is idempotent - safe to run repeatedly.
//
// Returns the number of rows deleted.
func DeleteOldFlakeEvents(ctx context.Context, db *sql.DB, retentionDays int) (int64, error) {
	query := `
		DELETE FROM flake_events
		WHERE created_at < NOW() - INTERVAL '1 day' * $1
	`

	result, err := db.ExecContext(ctx, query, retentionDays)
	if err != nil {
		return 0, fmt.Errorf("failed to delete old flake events: %w", err)
	}

	rowsAffected, err := result.RowsAffected()
	if err != nil {
		return 0, fmt.Errorf("failed to get rows affected: %w", err)
	}

	return rowsAffected, nil
}

// RunRetentionJob executes both retention operations and logs the results.
// This is the main entry point called by the cron scheduler.
func RunRetentionJob(ctx context.Context, db *sql.DB, junitDays, eventsDays int) error {
	log.Info().
		Int("junit_retention_days", junitDays).
		Int("events_retention_days", eventsDays).
		Msg("Starting retention job")

	startTime := time.Now()

	// Clear old JUnit content
	junitCleared, err := ClearOldJunitContent(ctx, db, junitDays)
	if err != nil {
		log.Error().Err(err).Msg("Failed to clear old junit content")
		return fmt.Errorf("junit content cleanup failed: %w", err)
	}

	// Delete old flake events
	eventsDeleted, err := DeleteOldFlakeEvents(ctx, db, eventsDays)
	if err != nil {
		log.Error().Err(err).Msg("Failed to delete old flake events")
		return fmt.Errorf("flake events cleanup failed: %w", err)
	}

	duration := time.Since(startTime)

	log.Info().
		Int64("junit_content_cleared", junitCleared).
		Int64("flake_events_deleted", eventsDeleted).
		Dur("duration", duration).
		Msg("Retention job completed")

	return nil
}
