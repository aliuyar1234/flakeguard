package retention

import (
	"context"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/rs/zerolog/log"
)

// ClearOldJunitContent clears the content field from junit_files older than the specified days.
// This frees storage while preserving metadata. The function is idempotent.
func ClearOldJunitContent(ctx context.Context, pool *pgxpool.Pool, retentionDays int) (int64, error) {
	query := `
		UPDATE junit_files
		SET content = NULL
		WHERE created_at < NOW() - INTERVAL '1 day' * $1
		  AND content IS NOT NULL
	`

	tag, err := pool.Exec(ctx, query, retentionDays)
	if err != nil {
		return 0, fmt.Errorf("failed to clear old junit content: %w", err)
	}
	return tag.RowsAffected(), nil
}

// DeleteOldFlakeEvents deletes flake_events rows older than the specified days.
// Note: This does NOT delete flake_stats - aggregated statistics are preserved.
// The function is idempotent.
func DeleteOldFlakeEvents(ctx context.Context, pool *pgxpool.Pool, retentionDays int) (int64, error) {
	query := `
		DELETE FROM flake_events
		WHERE created_at < NOW() - INTERVAL '1 day' * $1
	`

	tag, err := pool.Exec(ctx, query, retentionDays)
	if err != nil {
		return 0, fmt.Errorf("failed to delete old flake events: %w", err)
	}
	return tag.RowsAffected(), nil
}

// RunRetentionJob executes both retention operations and logs the results.
func RunRetentionJob(ctx context.Context, pool *pgxpool.Pool, junitDays, eventsDays int) error {
	log.Info().
		Int("junit_retention_days", junitDays).
		Int("events_retention_days", eventsDays).
		Msg("Starting retention job")

	startTime := time.Now()

	junitCleared, err := ClearOldJunitContent(ctx, pool, junitDays)
	if err != nil {
		log.Error().Err(err).Msg("Failed to clear old junit content")
		return fmt.Errorf("junit content cleanup failed: %w", err)
	}

	eventsDeleted, err := DeleteOldFlakeEvents(ctx, pool, eventsDays)
	if err != nil {
		log.Error().Err(err).Msg("Failed to delete old flake events")
		return fmt.Errorf("flake events cleanup failed: %w", err)
	}

	log.Info().
		Int64("junit_content_cleared", junitCleared).
		Int64("flake_events_deleted", eventsDeleted).
		Dur("duration", time.Since(startTime)).
		Msg("Retention job completed")

	return nil
}
