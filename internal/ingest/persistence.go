package ingest

import (
	"context"
	"fmt"

	"github.com/flakeguard/flakeguard/internal/config"
	"github.com/flakeguard/flakeguard/internal/flake"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

// PersistenceService handles database operations for ingestion
type PersistenceService struct {
	pool   *pgxpool.Pool
	config *config.Config
}

// NewPersistenceService creates a new persistence service
func NewPersistenceService(pool *pgxpool.Pool, cfg *config.Config) *PersistenceService {
	return &PersistenceService{
		pool:   pool,
		config: cfg,
	}
}

// IngestionResult contains the results of an ingestion operation
type IngestionResult struct {
	IngestionID       uuid.UUID
	TestResultsCount  int
	JUnitFilesCount   int
	FlakeEventsCount  int // Always 0 in this milestone
}

// PersistIngestion persists an entire ingestion within a transaction
func (s *PersistenceService) PersistIngestion(
	ctx context.Context,
	projectID uuid.UUID,
	apiKeyID uuid.UUID,
	metadata *IngestionMetadata,
	files []JUnitFile,
	testResults []TestResult,
) (*IngestionResult, error) {
	// Start transaction
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to begin transaction: %w", err)
	}
	defer tx.Rollback(ctx)

	// Create ingestion record
	ingestionID, err := s.createIngestion(ctx, tx, projectID, apiKeyID)
	if err != nil {
		return nil, fmt.Errorf("failed to create ingestion: %w", err)
	}

	// Upsert CI run
	ciRunID, err := s.upsertCIRun(ctx, tx, projectID, metadata)
	if err != nil {
		return nil, fmt.Errorf("failed to upsert CI run: %w", err)
	}

	// Upsert CI run attempt
	ciRunAttemptID, err := s.upsertCIRunAttempt(ctx, tx, ciRunID, metadata.RunAttempt)
	if err != nil {
		return nil, fmt.Errorf("failed to upsert CI run attempt: %w", err)
	}

	// Upsert CI job
	ciJobID, err := s.upsertCIJob(ctx, tx, ciRunAttemptID, metadata)
	if err != nil {
		return nil, fmt.Errorf("failed to upsert CI job: %w", err)
	}

	// Store JUnit files
	for _, file := range files {
		if err := s.storeJUnitFile(ctx, tx, ingestionID, file); err != nil {
			return nil, fmt.Errorf("failed to store JUnit file: %w", err)
		}
	}

	// Process test results
	testResultsInserted := 0
	for _, result := range testResults {
		// Upsert test case
		testCaseID, err := s.upsertTestCase(ctx, tx, projectID, metadata, result)
		if err != nil {
			return nil, fmt.Errorf("failed to upsert test case: %w", err)
		}

		// Insert test result (ON CONFLICT DO NOTHING for idempotency)
		inserted, err := s.insertTestResult(ctx, tx, testCaseID, ciJobID, result)
		if err != nil {
			return nil, fmt.Errorf("failed to insert test result: %w", err)
		}
		if inserted {
			testResultsInserted++
		}
	}

	// Commit transaction
	if err := tx.Commit(ctx); err != nil {
		return nil, fmt.Errorf("failed to commit transaction: %w", err)
	}

	// Detect flakes after test results are stored
	// Use NewDetectorWithSlack to enable Slack notifications
	detector := flake.NewDetectorWithSlack(s.pool, s.config)
	flakeEventsCount, err := detector.DetectFlakes(ctx, projectID, ciRunID)
	if err != nil {
		// Log error but don't fail ingestion
		// Flake detection is not critical to ingestion success
		return nil, fmt.Errorf("flake detection failed: %w", err)
	}

	return &IngestionResult{
		IngestionID:      ingestionID,
		TestResultsCount: testResultsInserted,
		JUnitFilesCount:  len(files),
		FlakeEventsCount: flakeEventsCount,
	}, nil
}

// createIngestion creates a new ingestion record
func (s *PersistenceService) createIngestion(ctx context.Context, tx pgx.Tx, projectID, apiKeyID uuid.UUID) (uuid.UUID, error) {
	var ingestionID uuid.UUID
	query := `
		INSERT INTO ingestions (project_id, api_key_id)
		VALUES ($1, $2)
		RETURNING id
	`
	err := tx.QueryRow(ctx, query, projectID, apiKeyID).Scan(&ingestionID)
	return ingestionID, err
}

// upsertCIRun upserts a CI run record
func (s *PersistenceService) upsertCIRun(ctx context.Context, tx pgx.Tx, projectID uuid.UUID, meta *IngestionMetadata) (uuid.UUID, error) {
	var ciRunID uuid.UUID
	query := `
		INSERT INTO ci_runs (
			project_id, repo_full_name, run_number, event, branch, commit_sha, workflow_name, last_seen_at
		) VALUES ($1, $2, $3, $4, $5, $6, $7, NOW())
		ON CONFLICT (project_id, repo_full_name, run_number)
		DO UPDATE SET last_seen_at = NOW()
		RETURNING id
	`

	eventType := NormalizeEventType(meta.EventType)

	err := tx.QueryRow(ctx, query,
		projectID,
		meta.RepoFullName,
		meta.RunNumber,
		eventType,
		meta.Branch,
		meta.SHA,
		meta.WorkflowName,
	).Scan(&ciRunID)

	return ciRunID, err
}

// upsertCIRunAttempt upserts a CI run attempt record
func (s *PersistenceService) upsertCIRunAttempt(ctx context.Context, tx pgx.Tx, ciRunID uuid.UUID, attemptNumber int) (uuid.UUID, error) {
	var ciRunAttemptID uuid.UUID
	query := `
		INSERT INTO ci_run_attempts (ci_run_id, attempt_number)
		VALUES ($1, $2)
		ON CONFLICT (ci_run_id, attempt_number)
		DO UPDATE SET ci_run_id = ci_run_attempts.ci_run_id
		RETURNING id
	`

	err := tx.QueryRow(ctx, query, ciRunID, attemptNumber).Scan(&ciRunAttemptID)
	return ciRunAttemptID, err
}

// upsertCIJob upserts a CI job record
func (s *PersistenceService) upsertCIJob(ctx context.Context, tx pgx.Tx, ciRunAttemptID uuid.UUID, meta *IngestionMetadata) (uuid.UUID, error) {
	var ciJobID uuid.UUID
	query := `
		INSERT INTO ci_jobs (ci_run_attempt_id, job_name, runner_os, runner_arch)
		VALUES ($1, $2, $3, $4)
		ON CONFLICT (ci_run_attempt_id, job_name)
		DO UPDATE SET runner_os = EXCLUDED.runner_os, runner_arch = EXCLUDED.runner_arch
		RETURNING id
	`

	err := tx.QueryRow(ctx, query,
		ciRunAttemptID,
		meta.JobName,
		meta.RunnerOS,
		meta.RunnerArch,
	).Scan(&ciJobID)

	return ciJobID, err
}

// storeJUnitFile stores a JUnit file record
func (s *PersistenceService) storeJUnitFile(ctx context.Context, tx pgx.Tx, ingestionID uuid.UUID, file JUnitFile) error {
	query := `
		INSERT INTO junit_files (ingestion_id, filename, content_bytes)
		VALUES ($1, $2, $3)
	`

	_, err := tx.Exec(ctx, query, ingestionID, file.Filename, file.ContentBytes)
	return err
}

// upsertTestCase upserts a test case record
func (s *PersistenceService) upsertTestCase(ctx context.Context, tx pgx.Tx, projectID uuid.UUID, meta *IngestionMetadata, result TestResult) (uuid.UUID, error) {
	var testCaseID uuid.UUID
	query := `
		INSERT INTO test_cases (project_id, test_id, classname, name, last_seen_at)
		VALUES ($1, $2, $3, $4, NOW())
		ON CONFLICT (project_id, test_id)
		DO UPDATE SET last_seen_at = NOW()
		RETURNING id
	`

	err := tx.QueryRow(ctx, query,
		projectID,
		result.TestIdentifier,
		result.Classname,
		result.Name,
	).Scan(&testCaseID)

	return testCaseID, err
}

// insertTestResult inserts a test result record (idempotent via ON CONFLICT DO NOTHING)
func (s *PersistenceService) insertTestResult(ctx context.Context, tx pgx.Tx, testCaseID, ciJobID uuid.UUID, result TestResult) (bool, error) {
	query := `
		INSERT INTO test_results (test_case_id, ci_job_id, status, duration_ms, failure_message, failure_output)
		VALUES ($1, $2, $3, $4, $5, $6)
		ON CONFLICT (test_case_id, ci_job_id) DO NOTHING
	`

	tag, err := tx.Exec(ctx, query,
		testCaseID,
		ciJobID,
		result.Status,
		result.DurationMS,
		nullString(result.FailureMessage),
		nullString(result.FailureOutput),
	)

	if err != nil {
		return false, err
	}

	// Check if row was inserted
	inserted := tag.RowsAffected() > 0
	return inserted, nil
}

// nullString converts an empty string to NULL
func nullString(s string) *string {
	if s == "" {
		return nil
	}
	return &s
}

// JUnitFile represents a JUnit file to be stored
type JUnitFile struct {
	Filename     string
	ContentBytes int
}
