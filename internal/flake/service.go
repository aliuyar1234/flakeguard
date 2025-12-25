package flake

import (
	"context"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
)

// Service handles flake business logic and queries
type Service struct {
	pool *pgxpool.Pool
}

// NewService creates a new flake service
func NewService(pool *pgxpool.Pool) *Service {
	return &Service{pool: pool}
}

// ListFlakes returns a list of flaky tests with filtering and pagination
func (s *Service) ListFlakes(ctx context.Context, projectID uuid.UUID, req ListFlakesRequest) ([]FlakeListItem, int, error) {
	// Calculate cutoff date
	cutoffDate := time.Now().AddDate(0, 0, -req.Days)

	// Build query with filters
	query := `
		SELECT
			fs.test_case_id,
			tc.test_id,
			cr.repo_full_name,
			cj.job_name,
			NULL as job_variant,
			fs.mixed_outcome_runs,
			fs.total_runs_seen,
			fs.flake_score,
			fs.first_seen_at,
			fs.last_seen_at
		FROM flake_stats fs
		JOIN test_cases tc ON fs.test_case_id = tc.id
		JOIN flake_events fe ON fs.test_case_id = fe.test_case_id
		JOIN ci_runs cr ON fe.ci_run_id = cr.id
		JOIN ci_run_attempts cra ON cra.ci_run_id = cr.id
		JOIN ci_jobs cj ON cj.ci_run_attempt_id = cra.id
		WHERE tc.project_id = $1
			AND fs.last_seen_at >= $2
	`

	args := []interface{}{projectID, cutoffDate}
	argNum := 3

	// Add repo filter
	if req.Repo != "" {
		query += fmt.Sprintf(" AND cr.repo_full_name = $%d", argNum)
		args = append(args, req.Repo)
		argNum++
	}

	// Add job_name filter
	if req.JobName != "" {
		query += fmt.Sprintf(" AND cj.job_name = $%d", argNum)
		args = append(args, req.JobName)
		argNum++
	}

	// Group by to get distinct test cases
	query += `
		GROUP BY
			fs.test_case_id,
			tc.test_id,
			cr.repo_full_name,
			cj.job_name,
			fs.mixed_outcome_runs,
			fs.total_runs_seen,
			fs.flake_score,
			fs.first_seen_at,
			fs.last_seen_at
		ORDER BY fs.flake_score DESC, fs.last_seen_at DESC
	`

	// Add pagination
	query += fmt.Sprintf(" LIMIT $%d OFFSET $%d", argNum, argNum+1)
	args = append(args, req.Limit, req.Offset)

	// Execute query
	rows, err := s.pool.Query(ctx, query, args...)
	if err != nil {
		return nil, 0, err
	}
	defer rows.Close()

	var flakes []FlakeListItem
	for rows.Next() {
		var item FlakeListItem
		if err := rows.Scan(
			&item.TestCaseID,
			&item.TestIdentifier,
			&item.RepoFullName,
			&item.JobName,
			&item.JobVariant,
			&item.MixedOutcomeRuns,
			&item.TotalRunsSeen,
			&item.FlakeScore,
			&item.FirstSeenAt,
			&item.LastSeenAt,
		); err != nil {
			return nil, 0, err
		}
		flakes = append(flakes, item)
	}

	if err := rows.Err(); err != nil {
		return nil, 0, err
	}

	// Get total count (simplified - just return current count for now)
	// In production, you'd run a separate COUNT query
	total := len(flakes)

	return flakes, total, nil
}

// GetFlakeDetail returns detailed information about a specific flaky test
func (s *Service) GetFlakeDetail(ctx context.Context, projectID, testCaseID uuid.UUID, evidenceLimit, evidenceOffset int) (*FlakeDetail, int, error) {
	// Query flake stats
	statsQuery := `
		SELECT
			fs.test_case_id,
			tc.test_id,
			cr.repo_full_name,
			cj.job_name,
			NULL as job_variant,
			fs.mixed_outcome_runs,
			fs.total_runs_seen,
			fs.flake_score,
			fs.last_failure_message,
			fs.first_seen_at,
			fs.last_seen_at
		FROM flake_stats fs
		JOIN test_cases tc ON fs.test_case_id = tc.id
		LEFT JOIN flake_events fe ON fs.test_case_id = fe.test_case_id
		LEFT JOIN ci_runs cr ON fe.ci_run_id = cr.id
		LEFT JOIN ci_run_attempts cra ON cra.ci_run_id = cr.id
		LEFT JOIN ci_jobs cj ON cj.ci_run_attempt_id = cra.id
		WHERE fs.test_case_id = $1
			AND tc.project_id = $2
		LIMIT 1
	`

	var detail FlakeDetail
	err := s.pool.QueryRow(ctx, statsQuery, testCaseID, projectID).Scan(
		&detail.TestCaseID,
		&detail.TestIdentifier,
		&detail.RepoFullName,
		&detail.JobName,
		&detail.JobVariant,
		&detail.MixedOutcomeRuns,
		&detail.TotalRunsSeen,
		&detail.FlakeScore,
		&detail.LastFailureMessage,
		&detail.FirstSeenAt,
		&detail.LastSeenAt,
	)
	if err != nil {
		return nil, 0, fmt.Errorf("flake not found")
	}

	// Query evidence (flake events)
	evidenceQuery := `
		SELECT
			fe.ci_run_id,
			NULL as run_url,
			cr.commit_sha,
			cj.job_name,
			NULL as job_variant,
			fe.failed_attempt_number,
			fe.passed_attempt_number,
			fe.created_at
		FROM flake_events fe
		JOIN ci_runs cr ON fe.ci_run_id = cr.id
		JOIN ci_run_attempts cra ON cra.ci_run_id = cr.id
		JOIN ci_jobs cj ON cj.ci_run_attempt_id = cra.id
		WHERE fe.test_case_id = $1
		ORDER BY fe.created_at DESC
		LIMIT $2 OFFSET $3
	`

	rows, err := s.pool.Query(ctx, evidenceQuery, testCaseID, evidenceLimit, evidenceOffset)
	if err != nil {
		return nil, 0, err
	}
	defer rows.Close()

	var evidence []FlakeEvidence
	for rows.Next() {
		var ev FlakeEvidence
		if err := rows.Scan(
			&ev.CIRunID,
			&ev.RunURL,
			&ev.SHA,
			&ev.JobName,
			&ev.JobVariant,
			&ev.FailedAttemptNumber,
			&ev.PassedAttemptNumber,
			&ev.CreatedAt,
		); err != nil {
			return nil, 0, err
		}
		evidence = append(evidence, ev)
	}

	if err := rows.Err(); err != nil {
		return nil, 0, err
	}

	detail.Evidence = evidence

	// Count total evidence
	var evidenceTotal int
	countQuery := `SELECT COUNT(*) FROM flake_events WHERE test_case_id = $1`
	if err := s.pool.QueryRow(ctx, countQuery, testCaseID).Scan(&evidenceTotal); err != nil {
		return nil, 0, err
	}

	return &detail, evidenceTotal, nil
}
