package flake

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
)

var ErrFlakeNotFound = errors.New("flake not found")

// Service handles flake business logic and queries.
type Service struct {
	pool *pgxpool.Pool
}

func NewService(pool *pgxpool.Pool) *Service {
	return &Service{pool: pool}
}

// ListFlakes returns a list of flaky tests with filtering and pagination (used by web UI).
func (s *Service) ListFlakes(ctx context.Context, projectID uuid.UUID, req ListFlakesRequest) ([]FlakeListItem, int, error) {
	cutoffDate := time.Now().AddDate(0, 0, -req.Days)

	where := `
		WHERE tc.project_id = $1
		  AND fs.last_seen_at >= $2
	`

	args := []any{projectID, cutoffDate}
	argNum := 3

	if req.Repo != "" {
		where += fmt.Sprintf(" AND tc.repo_full_name = $%d", argNum)
		args = append(args, req.Repo)
		argNum++
	}

	if req.JobName != "" {
		where += fmt.Sprintf(" AND tc.job_name = $%d", argNum)
		args = append(args, req.JobName)
		argNum++
	}

	countQuery := `
		SELECT COUNT(*)
		FROM flake_stats fs
		JOIN test_cases tc ON tc.id = fs.test_case_id
	` + where

	var total int
	if err := s.pool.QueryRow(ctx, countQuery, args...).Scan(&total); err != nil {
		return nil, 0, err
	}

	listQuery := `
		SELECT
			fs.test_case_id,
			tc.repo_full_name,
			tc.job_name,
			tc.job_variant,
			tc.test_identifier,
			fs.flake_score,
			fs.mixed_outcome_runs,
			fs.total_runs_seen,
			fs.first_seen_at,
			fs.last_seen_at
		FROM flake_stats fs
		JOIN test_cases tc ON tc.id = fs.test_case_id
	` + where + `
		ORDER BY fs.flake_score DESC, fs.last_seen_at DESC
	` + fmt.Sprintf(" LIMIT $%d OFFSET $%d", argNum, argNum+1)

	args = append(args, req.Limit, req.Offset)

	rows, err := s.pool.Query(ctx, listQuery, args...)
	if err != nil {
		return nil, 0, err
	}
	defer rows.Close()

	var flakes []FlakeListItem
	for rows.Next() {
		var item FlakeListItem
		if err := rows.Scan(
			&item.TestCaseID,
			&item.RepoFullName,
			&item.JobName,
			&item.JobVariant,
			&item.TestIdentifier,
			&item.FlakeScore,
			&item.MixedOutcomeRuns,
			&item.TotalRunsSeen,
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

	return flakes, total, nil
}

func (s *Service) GetFlakeDetail(ctx context.Context, projectID, testCaseID uuid.UUID, days, evidenceLimit, evidenceOffset int) (*FlakeDetail, int, error) {
	cutoffDate := time.Now().AddDate(0, 0, -days)

	statsQuery := `
		SELECT
			tc.id,
			tc.repo_full_name,
			tc.job_name,
			tc.job_variant,
			tc.test_identifier,
			fs.flake_score,
			fs.mixed_outcome_runs,
			fs.total_runs_seen,
			fs.last_failure_message,
			fs.first_seen_at,
			fs.last_seen_at
		FROM flake_stats fs
		JOIN test_cases tc ON tc.id = fs.test_case_id
		WHERE tc.project_id = $1
		  AND tc.id = $2
		LIMIT 1
	`

	var detail FlakeDetail
	if err := s.pool.QueryRow(ctx, statsQuery, projectID, testCaseID).Scan(
		&detail.TestCaseID,
		&detail.RepoFullName,
		&detail.JobName,
		&detail.JobVariant,
		&detail.TestIdentifier,
		&detail.FlakeScore,
		&detail.MixedOutcomeRuns,
		&detail.TotalRunsSeen,
		&detail.LastFailureMessage,
		&detail.FirstSeenAt,
		&detail.LastSeenAt,
	); err != nil {
		return nil, 0, ErrFlakeNotFound
	}

	evidenceQuery := `
		SELECT
			cr.github_run_id,
			cr.run_url,
			cr.sha,
			fe.failed_attempt_number,
			fe.passed_attempt_number,
			failed.completed_at,
			passed.completed_at
		FROM flake_events fe
		JOIN ci_runs cr ON cr.id = fe.ci_run_id
		LEFT JOIN ci_run_attempts failed ON failed.ci_run_id = cr.id AND failed.attempt_number = fe.failed_attempt_number
		LEFT JOIN ci_run_attempts passed ON passed.ci_run_id = cr.id AND passed.attempt_number = fe.passed_attempt_number
		WHERE fe.test_case_id = $1
		  AND fe.created_at >= $2
		ORDER BY fe.created_at DESC
		LIMIT $3 OFFSET $4
	`

	rows, err := s.pool.Query(ctx, evidenceQuery, testCaseID, cutoffDate, evidenceLimit, evidenceOffset)
	if err != nil {
		return nil, 0, err
	}
	defer rows.Close()

	var evidence []FlakeEvidence
	for rows.Next() {
		var ev FlakeEvidence
		var failedAt sql.NullTime
		var passedAt sql.NullTime

		if err := rows.Scan(
			&ev.GitHubRunID,
			&ev.RunURL,
			&ev.SHA,
			&ev.AttemptFailed,
			&ev.AttemptPassed,
			&failedAt,
			&passedAt,
		); err != nil {
			return nil, 0, err
		}
		if failedAt.Valid {
			ev.FailedAt = &failedAt.Time
		}
		if passedAt.Valid {
			ev.PassedAt = &passedAt.Time
		}
		evidence = append(evidence, ev)
	}
	if err := rows.Err(); err != nil {
		return nil, 0, err
	}

	detail.Evidence = evidence

	var evidenceTotal int
	countQuery := `
		SELECT COUNT(*)
		FROM flake_events
		WHERE test_case_id = $1
		  AND created_at >= $2
	`
	if err := s.pool.QueryRow(ctx, countQuery, testCaseID, cutoffDate).Scan(&evidenceTotal); err != nil {
		return nil, 0, err
	}

	return &detail, evidenceTotal, nil
}
