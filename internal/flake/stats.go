package flake

import (
	"context"
	"fmt"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/rs/zerolog/log"
)

// StatsService handles flake statistics operations
type StatsService struct {
	pool *pgxpool.Pool
}

// NewStatsService creates a new stats service
func NewStatsService(pool *pgxpool.Pool) *StatsService {
	return &StatsService{pool: pool}
}

const (
	// MaxFailureMessageLength is 1KB
	MaxFailureMessageLength = 1024
)

// UpdateStats updates flake statistics for a test case
// This should be called within a transaction (tx) after detecting a flake
func (s *StatsService) UpdateStats(ctx context.Context, tx pgx.Tx, testCaseID, ciRunID uuid.UUID, failureMessage *string) error {
	// Count total runs this test has appeared in
	totalRuns, err := s.countTotalRuns(ctx, tx, testCaseID)
	if err != nil {
		return fmt.Errorf("failed to count total runs: %w", err)
	}

	// Count unique runs with mixed outcomes (flakes)
	mixedRuns, err := s.countMixedOutcomeRuns(ctx, tx, testCaseID)
	if err != nil {
		return fmt.Errorf("failed to count mixed outcome runs: %w", err)
	}

	// Calculate flake score
	var flakeScore float64
	if totalRuns > 0 {
		flakeScore = float64(mixedRuns) / float64(totalRuns)
	}

	// Truncate failure message to 1KB
	truncatedMsg := truncateMessage(failureMessage)

	// Upsert flake_stats
	query := `
		INSERT INTO flake_stats (
			test_case_id,
			mixed_outcome_runs,
			total_runs_seen,
			flake_score,
			last_failure_message,
			first_seen_at,
			last_seen_at
		) VALUES ($1, $2, $3, $4, $5, NOW(), NOW())
		ON CONFLICT (test_case_id)
		DO UPDATE SET
			mixed_outcome_runs = EXCLUDED.mixed_outcome_runs,
			total_runs_seen = EXCLUDED.total_runs_seen,
			flake_score = EXCLUDED.flake_score,
			last_failure_message = EXCLUDED.last_failure_message,
			last_seen_at = NOW()
	`

	_, err = tx.Exec(ctx, query,
		testCaseID,
		mixedRuns,
		totalRuns,
		flakeScore,
		truncatedMsg,
	)

	if err != nil {
		return fmt.Errorf("failed to upsert flake_stats: %w", err)
	}

	log.Debug().
		Str("test_case_id", testCaseID.String()).
		Int("mixed_outcome_runs", mixedRuns).
		Int("total_runs_seen", totalRuns).
		Float64("flake_score", flakeScore).
		Msg("Updated flake stats")

	return nil
}

// countTotalRuns counts how many unique CI runs this test has appeared in
func (s *StatsService) countTotalRuns(ctx context.Context, tx pgx.Tx, testCaseID uuid.UUID) (int, error) {
	query := `
		SELECT COUNT(DISTINCT cra.ci_run_id)
		FROM test_results tr
		JOIN ci_jobs cj ON tr.ci_job_id = cj.id
		JOIN ci_run_attempts cra ON cj.ci_run_attempt_id = cra.id
		WHERE tr.test_case_id = $1
	`

	var count int
	err := tx.QueryRow(ctx, query, testCaseID).Scan(&count)
	return count, err
}

// countMixedOutcomeRuns counts unique runs where this test had both failures and passes
func (s *StatsService) countMixedOutcomeRuns(ctx context.Context, tx pgx.Tx, testCaseID uuid.UUID) (int, error) {
	// Count distinct ci_run_ids that have flake events for this test
	query := `
		SELECT COUNT(DISTINCT ci_run_id)
		FROM flake_events
		WHERE test_case_id = $1
	`

	var count int
	err := tx.QueryRow(ctx, query, testCaseID).Scan(&count)
	return count, err
}

// truncateMessage truncates a message to MaxFailureMessageLength (1KB)
func truncateMessage(msg *string) *string {
	if msg == nil {
		return nil
	}

	if len(*msg) <= MaxFailureMessageLength {
		return msg
	}

	truncated := (*msg)[:MaxFailureMessageLength]
	return &truncated
}
