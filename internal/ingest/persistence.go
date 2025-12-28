package ingest

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/flakeguard/flakeguard/internal/config"
	"github.com/flakeguard/flakeguard/internal/flake"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/rs/zerolog/log"
)

// PersistenceService handles database operations for ingestion.
type PersistenceService struct {
	pool   *pgxpool.Pool
	config *config.Config
}

func NewPersistenceService(pool *pgxpool.Pool, cfg *config.Config) *PersistenceService {
	return &PersistenceService{
		pool:   pool,
		config: cfg,
	}
}

type IngestionResult struct {
	IngestionID      uuid.UUID
	TestResultsCount int
	JUnitFilesCount  int
	FlakeEventsCount int
}

func (s *PersistenceService) PersistIngestion(
	ctx context.Context,
	projectID uuid.UUID,
	apiKeyID uuid.UUID,
	metadata *IngestionMetadata,
	files []JUnitFile,
	testResults []TestResult,
) (*IngestionResult, error) {
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to begin transaction: %w", err)
	}
	defer tx.Rollback(ctx)

	metaJSON, err := json.Marshal(metadata)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal meta: %w", err)
	}

	ingestionID, err := s.createIngestion(ctx, tx, projectID, apiKeyID, string(metaJSON), len(files))
	if err != nil {
		return nil, fmt.Errorf("failed to create ingestion: %w", err)
	}

	ciRunID, err := s.upsertCIRun(ctx, tx, projectID, metadata)
	if err != nil {
		return nil, fmt.Errorf("failed to upsert CI run: %w", err)
	}

	ciRunAttemptID, err := s.upsertCIRunAttempt(ctx, tx, ciRunID, metadata.GitHubRunAttempt, metadata.StartedAtTime(), metadata.CompletedAtTime())
	if err != nil {
		return nil, fmt.Errorf("failed to upsert CI run attempt: %w", err)
	}

	ciJobID, err := s.upsertCIJob(ctx, tx, ciRunAttemptID, metadata.JobName, metadata.JobVariant)
	if err != nil {
		return nil, fmt.Errorf("failed to upsert CI job: %w", err)
	}

	for _, file := range files {
		if err := s.storeJUnitFile(ctx, tx, ingestionID, file); err != nil {
			return nil, fmt.Errorf("failed to store JUnit file: %w", err)
		}
	}

	testResultsInserted := 0
	for _, result := range testResults {
		testCaseID, err := s.upsertTestCase(ctx, tx, projectID, metadata, result)
		if err != nil {
			return nil, fmt.Errorf("failed to upsert test case: %w", err)
		}

		inserted, err := s.insertTestResult(ctx, tx, testCaseID, ciJobID, result)
		if err != nil {
			return nil, fmt.Errorf("failed to insert test result: %w", err)
		}
		if inserted {
			testResultsInserted++
		}
	}

	if err := s.updateIngestionTestResultsCount(ctx, tx, ingestionID, testResultsInserted); err != nil {
		return nil, fmt.Errorf("failed to update ingestion counts: %w", err)
	}

	if err := tx.Commit(ctx); err != nil {
		return nil, fmt.Errorf("failed to commit transaction: %w", err)
	}

	flakeEventsCount := 0
	detector := flake.NewDetectorWithSlack(s.pool, s.config)
	if detector != nil {
		n, err := detector.DetectFlakes(ctx, projectID, ciRunID)
		if err != nil {
			log.Error().
				Err(err).
				Str("project_id", projectID.String()).
				Str("ci_run_id", ciRunID.String()).
				Msg("Flake detection failed")
		} else {
			flakeEventsCount = n
		}
	}

	return &IngestionResult{
		IngestionID:      ingestionID,
		TestResultsCount: testResultsInserted,
		JUnitFilesCount:  len(files),
		FlakeEventsCount: flakeEventsCount,
	}, nil
}

func (s *PersistenceService) createIngestion(
	ctx context.Context,
	tx pgx.Tx,
	projectID uuid.UUID,
	apiKeyID uuid.UUID,
	metaJSON string,
	junitFilesCount int,
) (uuid.UUID, error) {
	var ingestionID uuid.UUID
	query := `
		INSERT INTO ingestions (project_id, api_key_id, meta, junit_files_count, test_results_count)
		VALUES ($1, $2, $3::jsonb, $4, 0)
		RETURNING id
	`
	err := tx.QueryRow(ctx, query, projectID, apiKeyID, metaJSON, junitFilesCount).Scan(&ingestionID)
	return ingestionID, err
}

func (s *PersistenceService) updateIngestionTestResultsCount(ctx context.Context, tx pgx.Tx, ingestionID uuid.UUID, testResultsCount int) error {
	query := `
		UPDATE ingestions
		SET test_results_count = $2
		WHERE id = $1
	`
	_, err := tx.Exec(ctx, query, ingestionID, testResultsCount)
	return err
}

func (s *PersistenceService) upsertCIRun(ctx context.Context, tx pgx.Tx, projectID uuid.UUID, meta *IngestionMetadata) (uuid.UUID, error) {
	var ciRunID uuid.UUID
	query := `
		INSERT INTO ci_runs (
			project_id, repo_full_name, workflow_name, workflow_ref,
			github_run_id, github_run_number, run_url, sha, branch, event, pr_number,
			first_seen_at, last_seen_at
		) VALUES (
			$1, $2, $3, $4,
			$5, $6, $7, $8, $9, $10::ci_event, $11,
			NOW(), NOW()
		)
		ON CONFLICT (project_id, repo_full_name, github_run_id)
		DO UPDATE SET
			workflow_name = EXCLUDED.workflow_name,
			workflow_ref = EXCLUDED.workflow_ref,
			github_run_number = EXCLUDED.github_run_number,
			run_url = EXCLUDED.run_url,
			sha = EXCLUDED.sha,
			branch = EXCLUDED.branch,
			event = EXCLUDED.event,
			pr_number = EXCLUDED.pr_number,
			last_seen_at = NOW()
		RETURNING id
	`

	event := NormalizeEventType(meta.Event)
	err := tx.QueryRow(ctx, query,
		projectID,
		meta.RepoFullName,
		meta.WorkflowName,
		meta.WorkflowRef,
		meta.GitHubRunID,
		meta.GitHubRunNumber,
		meta.RunURL,
		meta.SHA,
		meta.Branch,
		event,
		meta.PRNumber,
	).Scan(&ciRunID)

	return ciRunID, err
}

func (s *PersistenceService) upsertCIRunAttempt(
	ctx context.Context,
	tx pgx.Tx,
	ciRunID uuid.UUID,
	attemptNumber int,
	startedAt, completedAt time.Time,
) (uuid.UUID, error) {
	var ciRunAttemptID uuid.UUID
	query := `
		INSERT INTO ci_run_attempts (ci_run_id, attempt_number, started_at, completed_at)
		VALUES ($1, $2, $3, $4)
		ON CONFLICT (ci_run_id, attempt_number)
		DO UPDATE SET started_at = EXCLUDED.started_at, completed_at = EXCLUDED.completed_at
		RETURNING id
	`

	err := tx.QueryRow(ctx, query, ciRunID, attemptNumber, startedAt, completedAt).Scan(&ciRunAttemptID)
	return ciRunAttemptID, err
}

func (s *PersistenceService) upsertCIJob(ctx context.Context, tx pgx.Tx, ciRunAttemptID uuid.UUID, jobName, jobVariant string) (uuid.UUID, error) {
	var ciJobID uuid.UUID
	query := `
		INSERT INTO ci_jobs (ci_run_attempt_id, job_name, job_variant)
		VALUES ($1, $2, $3)
		ON CONFLICT (ci_run_attempt_id, job_name, job_variant)
		DO UPDATE SET job_name = EXCLUDED.job_name
		RETURNING id
	`

	err := tx.QueryRow(ctx, query, ciRunAttemptID, jobName, jobVariant).Scan(&ciJobID)
	return ciJobID, err
}

func (s *PersistenceService) storeJUnitFile(ctx context.Context, tx pgx.Tx, ingestionID uuid.UUID, file JUnitFile) error {
	query := `
		INSERT INTO junit_files (ingestion_id, filename, sha256, size_bytes, content_truncated, content)
		VALUES ($1, $2, $3, $4, $5, $6)
	`
	_, err := tx.Exec(ctx, query, ingestionID, file.Filename, file.SHA256, file.SizeBytes, file.ContentTruncated, file.Content)
	return err
}

func (s *PersistenceService) upsertTestCase(ctx context.Context, tx pgx.Tx, projectID uuid.UUID, meta *IngestionMetadata, result TestResult) (uuid.UUID, error) {
	var testCaseID uuid.UUID
	query := `
		INSERT INTO test_cases (project_id, repo_full_name, job_name, job_variant, test_identifier)
		VALUES ($1, $2, $3, $4, $5)
		ON CONFLICT (project_id, repo_full_name, job_name, job_variant, test_identifier)
		DO UPDATE SET last_seen_at = NOW()
		RETURNING id
	`

	err := tx.QueryRow(ctx, query,
		projectID,
		meta.RepoFullName,
		meta.JobName,
		meta.JobVariant,
		result.TestIdentifier,
	).Scan(&testCaseID)

	return testCaseID, err
}

func (s *PersistenceService) insertTestResult(ctx context.Context, tx pgx.Tx, testCaseID, ciJobID uuid.UUID, result TestResult) (bool, error) {
	query := `
		INSERT INTO test_results (test_case_id, ci_job_id, status, duration_ms, failure_message, failure_output)
		VALUES ($1, $2, $3::test_status, $4, $5, $6)
		ON CONFLICT (test_case_id, ci_job_id) DO NOTHING
	`

	tag, err := tx.Exec(ctx, query,
		testCaseID,
		ciJobID,
		result.Status,
		nullInt(result.DurationMS),
		nullString(result.FailureMessage),
		nullString(result.FailureOutput),
	)
	if err != nil {
		return false, err
	}
	return tag.RowsAffected() > 0, nil
}

func nullString(s string) *string {
	if s == "" {
		return nil
	}
	return &s
}

func nullInt(v int) *int {
	if v == 0 {
		return nil
	}
	return &v
}

type JUnitFile struct {
	Filename         string
	SHA256           string
	SizeBytes        int
	ContentTruncated bool
	Content          []byte
}
