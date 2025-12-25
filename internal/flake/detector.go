package flake

import (
	"context"
	"fmt"

	"github.com/flakeguard/flakeguard/internal/config"
	"github.com/flakeguard/flakeguard/internal/projects"
	"github.com/flakeguard/flakeguard/internal/slack"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/rs/zerolog/log"
)

// Detector handles flake detection logic
type Detector struct {
	pool        *pgxpool.Pool
	slackClient *slack.Client
	baseURL     string
}

// NewDetector creates a new flake detector
func NewDetector(pool *pgxpool.Pool) *Detector {
	return &Detector{
		pool: pool,
	}
}

// NewDetectorWithSlack creates a new flake detector with Slack notifications enabled
func NewDetectorWithSlack(pool *pgxpool.Pool, cfg *config.Config) *Detector {
	return &Detector{
		pool:        pool,
		slackClient: slack.NewClient(cfg.SlackTimeoutMS),
		baseURL:     cfg.BaseURL,
	}
}

// testAttempt represents a single test attempt
type testAttempt struct {
	TestCaseID    uuid.UUID
	AttemptNumber int
	Status        string
	FailureMsg    *string
}

// DetectFlakes detects flaky tests within a CI run
// Returns number of flake events created
func (d *Detector) DetectFlakes(ctx context.Context, projectID, ciRunID uuid.UUID) (int, error) {
	log.Debug().
		Str("project_id", projectID.String()).
		Str("ci_run_id", ciRunID.String()).
		Msg("Starting flake detection")

	// Start transaction for detection + stats update
	tx, err := d.pool.Begin(ctx)
	if err != nil {
		return 0, fmt.Errorf("failed to begin transaction: %w", err)
	}
	defer tx.Rollback(ctx)

	// Query all test attempts for this run, grouped by test
	attempts, err := d.getTestAttempts(ctx, tx, ciRunID)
	if err != nil {
		return 0, fmt.Errorf("failed to get test attempts: %w", err)
	}

	// Group attempts by test_case_id
	attemptsByTest := make(map[uuid.UUID][]testAttempt)
	for _, attempt := range attempts {
		attemptsByTest[attempt.TestCaseID] = append(attemptsByTest[attempt.TestCaseID], attempt)
	}

	flakeEventsCreated := 0
	statsService := NewStatsService(d.pool)

	// Detect flakes for each test
	for testCaseID, testAttempts := range attemptsByTest {
		flakeDetected, failedAttempt, passedAttempt, failureMsg := d.detectFlakePattern(testAttempts)

		if flakeDetected {
			// Create flake event
			eventID, err := d.insertFlakeEvent(ctx, tx, testCaseID, ciRunID, failedAttempt, passedAttempt)
			if err != nil {
				// Log but don't fail on duplicate constraint violations (idempotency)
				if isDuplicateKeyError(err) {
					log.Debug().
						Str("test_case_id", testCaseID.String()).
						Str("ci_run_id", ciRunID.String()).
						Msg("Flake event already exists (duplicate ingestion)")
					continue
				}
				return 0, fmt.Errorf("failed to insert flake event: %w", err)
			}

			log.Info().
				Str("event_id", eventID.String()).
				Str("test_case_id", testCaseID.String()).
				Str("ci_run_id", ciRunID.String()).
				Int("failed_attempt", failedAttempt).
				Int("passed_attempt", passedAttempt).
				Msg("Flake detected")

			// Update stats
			if err := statsService.UpdateStats(ctx, tx, testCaseID, ciRunID, failureMsg); err != nil {
				return 0, fmt.Errorf("failed to update stats: %w", err)
			}

			flakeEventsCreated++
		}
	}

	// Commit transaction
	if err := tx.Commit(ctx); err != nil {
		return 0, fmt.Errorf("failed to commit transaction: %w", err)
	}

	// Send Slack notifications asynchronously after transaction commit
	// Only send if Slack client is configured
	if d.slackClient != nil && flakeEventsCreated > 0 {
		// Trigger async notifications for all detected flakes
		for testCaseID, testAttempts := range attemptsByTest {
			flakeDetected, failedAttempt, passedAttempt, _ := d.detectFlakePattern(testAttempts)
			if flakeDetected {
				// Send notification asynchronously
				go d.notifySlackAsync(projectID, ciRunID, testCaseID, failedAttempt, passedAttempt)
			}
		}
	}

	log.Info().
		Str("ci_run_id", ciRunID.String()).
		Int("flake_events_created", flakeEventsCreated).
		Msg("Flake detection complete")

	return flakeEventsCreated, nil
}

// getTestAttempts retrieves all test attempts for a CI run
func (d *Detector) getTestAttempts(ctx context.Context, tx pgx.Tx, ciRunID uuid.UUID) ([]testAttempt, error) {
	query := `
		SELECT
			tr.test_case_id,
			cra.attempt_number,
			tr.status,
			tr.failure_message
		FROM test_results tr
		JOIN ci_jobs cj ON tr.ci_job_id = cj.id
		JOIN ci_run_attempts cra ON cj.ci_run_attempt_id = cra.id
		WHERE cra.ci_run_id = $1
		ORDER BY tr.test_case_id, cra.attempt_number
	`

	rows, err := tx.Query(ctx, query, ciRunID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var attempts []testAttempt
	for rows.Next() {
		var a testAttempt
		if err := rows.Scan(&a.TestCaseID, &a.AttemptNumber, &a.Status, &a.FailureMsg); err != nil {
			return nil, err
		}
		attempts = append(attempts, a)
	}

	return attempts, rows.Err()
}

// detectFlakePattern detects fail->pass pattern
// Returns: (flakeDetected, failedAttemptNum, passedAttemptNum, failureMessage)
func (d *Detector) detectFlakePattern(attempts []testAttempt) (bool, int, int, *string) {
	// Need at least 2 attempts
	if len(attempts) < 2 {
		return false, 0, 0, nil
	}

	// Find earliest failure and first subsequent pass
	var earliestFailure *testAttempt

	for i := range attempts {
		if isFailed(attempts[i].Status) {
			if earliestFailure == nil {
				earliestFailure = &attempts[i]
			}

			// Look for a pass after this failure
			for j := i + 1; j < len(attempts); j++ {
				if isPassed(attempts[j].Status) {
					// Found fail->pass pattern
					return true, earliestFailure.AttemptNumber, attempts[j].AttemptNumber, earliestFailure.FailureMsg
				}
			}
		}
	}

	// No fail->pass pattern found
	return false, 0, 0, nil
}

// insertFlakeEvent creates a flake event record
func (d *Detector) insertFlakeEvent(ctx context.Context, tx pgx.Tx, testCaseID, ciRunID uuid.UUID, failedAttempt, passedAttempt int) (uuid.UUID, error) {
	query := `
		INSERT INTO flake_events (test_case_id, ci_run_id, failed_attempt_number, passed_attempt_number)
		VALUES ($1, $2, $3, $4)
		RETURNING id
	`

	var eventID uuid.UUID
	err := tx.QueryRow(ctx, query, testCaseID, ciRunID, failedAttempt, passedAttempt).Scan(&eventID)
	return eventID, err
}

// isFailed checks if status represents a failure
func isFailed(status string) bool {
	return status == "failed" || status == "error"
}

// isPassed checks if status represents a pass
func isPassed(status string) bool {
	return status == "passed"
}

// isDuplicateKeyError checks if error is a unique constraint violation
func isDuplicateKeyError(err error) bool {
	if err == nil {
		return false
	}
	// Check for PostgreSQL unique violation error code 23505
	return err.Error() != "" && (
		// pgx error messages contain "duplicate key value violates unique constraint"
		len(err.Error()) > 20 && err.Error()[:20] == "ERROR: duplicate key" ||
		// or check for the error code
		err.Error() == "23505")
}

// notifySlackAsync sends a Slack notification for a flake event
// This runs in a goroutine and uses a background context to ensure it completes
// even if the original request context is cancelled
func (d *Detector) notifySlackAsync(projectID, ciRunID, testCaseID uuid.UUID, failedAttempt, passedAttempt int) {
	// Use background context (not the request context) to ensure notification completes
	ctx := context.Background()

	// Load project Slack settings
	projectService := projects.NewService(d.pool)
	project, err := projectService.GetByID(ctx, projectID)
	if err != nil {
		log.Warn().
			Err(err).
			Str("project_id", projectID.String()).
			Msg("Failed to load project for Slack notification")
		return
	}

	// Skip if Slack not enabled
	if !project.SlackEnabled || !project.SlackWebhookURL.Valid || project.SlackWebhookURL.String == "" {
		log.Debug().
			Str("project_id", projectID.String()).
			Bool("slack_enabled", project.SlackEnabled).
			Msg("Slack not configured, skipping notification")
		return
	}

	// Get flake details for the message
	flakeInfo, err := d.getFlakeInfo(ctx, ciRunID, testCaseID)
	if err != nil {
		log.Warn().
			Err(err).
			Str("test_case_id", testCaseID.String()).
			Str("ci_run_id", ciRunID.String()).
			Msg("Failed to get flake info for Slack notification")
		return
	}

	// Build dashboard URL
	dashboardURL := d.buildDashboardURL(project.OrgID, projectID, testCaseID)

	// Build and send Slack message
	msg := slack.FlakeMessage{
		Repo:          flakeInfo.RepoFullName,
		Workflow:      flakeInfo.WorkflowName,
		Job:           flakeInfo.JobName,
		TestID:        flakeInfo.TestIdentifier,
		FailedAttempt: failedAttempt,
		PassedAttempt: passedAttempt,
		DashboardURL:  dashboardURL,
	}

	// Send notification (errors are logged inside PostFlakeNotification)
	d.slackClient.PostFlakeNotification(ctx, project.SlackWebhookURL.String, msg)
}

// flakeInfo contains information needed for Slack notifications
type flakeInfo struct {
	RepoFullName   string
	WorkflowName   string
	JobName        string
	TestIdentifier string
}

// getFlakeInfo retrieves flake information for Slack notifications
func (d *Detector) getFlakeInfo(ctx context.Context, ciRunID, testCaseID uuid.UUID) (*flakeInfo, error) {
	query := `
		SELECT
			cr.repo_full_name,
			cr.workflow_name,
			cj.job_name,
			tc.test_id
		FROM test_cases tc
		JOIN ci_runs cr ON cr.id = $1
		JOIN ci_run_attempts cra ON cra.ci_run_id = cr.id
		JOIN ci_jobs cj ON cj.ci_run_attempt_id = cra.id
		WHERE tc.id = $2
		LIMIT 1
	`

	var info flakeInfo
	err := d.pool.QueryRow(ctx, query, ciRunID, testCaseID).Scan(
		&info.RepoFullName,
		&info.WorkflowName,
		&info.JobName,
		&info.TestIdentifier,
	)

	if err != nil {
		return nil, fmt.Errorf("failed to query flake info: %w", err)
	}

	return &info, nil
}

// buildDashboardURL constructs the dashboard URL for a flake
func (d *Detector) buildDashboardURL(orgID, projectID, testCaseID uuid.UUID) string {
	if d.baseURL == "" {
		return fmt.Sprintf("http://localhost:8080/orgs/%s/projects/%s/flakes/%s",
			orgID.String(), projectID.String(), testCaseID.String())
	}
	return fmt.Sprintf("%s/orgs/%s/projects/%s/flakes/%s",
		d.baseURL, orgID.String(), projectID.String(), testCaseID.String())
}
