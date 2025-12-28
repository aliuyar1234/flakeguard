package flake

import (
	"time"

	"github.com/google/uuid"
)

// FlakeEvent represents a detected flaky test event
type FlakeEvent struct {
	ID                  uuid.UUID `json:"id"`
	TestCaseID          uuid.UUID `json:"test_case_id"`
	CIRunID             uuid.UUID `json:"ci_run_id"`
	FailedAttemptNumber int       `json:"failed_attempt_number"`
	PassedAttemptNumber int       `json:"passed_attempt_number"`
	CreatedAt           time.Time `json:"created_at"`
}

// FlakeStats represents aggregated statistics for a flaky test
type FlakeStats struct {
	TestCaseID         uuid.UUID `json:"test_case_id"`
	MixedOutcomeRuns   int       `json:"mixed_outcome_runs"`
	TotalRunsSeen      int       `json:"total_runs_seen"`
	FlakeScore         float64   `json:"flake_score"`
	LastFailureMessage *string   `json:"last_failure_message"`
	FirstSeenAt        time.Time `json:"first_seen_at"`
	LastSeenAt         time.Time `json:"last_seen_at"`
	UpdatedAt          time.Time `json:"updated_at"`
}

// FlakeListItem represents a flaky test in the list view
type FlakeListItem struct {
	TestCaseID       uuid.UUID `json:"test_case_id"`
	RepoFullName     string    `json:"repo_full_name"`
	JobName          string    `json:"job_name"`
	JobVariant       string    `json:"job_variant"`
	TestIdentifier   string    `json:"test_identifier"`
	FlakeScore       float64   `json:"flake_score"`
	MixedOutcomeRuns int       `json:"mixed_outcome_runs"`
	TotalRunsSeen    int       `json:"total_runs_seen"`
	FirstSeenAt      time.Time `json:"first_seen_at"`
	LastSeenAt       time.Time `json:"last_seen_at"`
}

// FlakeEvidence represents evidence of a single flake event
type FlakeEvidence struct {
	GitHubRunID   int64      `json:"github_run_id"`
	RunURL        string     `json:"run_url"`
	SHA           string     `json:"sha"`
	AttemptFailed int        `json:"attempt_failed"`
	AttemptPassed int        `json:"attempt_passed"`
	FailedAt      *time.Time `json:"failed_at"`
	PassedAt      *time.Time `json:"passed_at"`
}

// FlakeDetail represents the full detail view of a flaky test
type FlakeDetail struct {
	TestCaseID         uuid.UUID       `json:"test_case_id"`
	RepoFullName       string          `json:"repo_full_name"`
	JobName            string          `json:"job_name"`
	JobVariant         string          `json:"job_variant"`
	TestIdentifier     string          `json:"test_identifier"`
	FlakeScore         float64         `json:"flake_score"`
	MixedOutcomeRuns   int             `json:"mixed_outcome_runs"`
	TotalRunsSeen      int             `json:"total_runs_seen"`
	LastFailureMessage *string         `json:"last_failure_message"`
	FirstSeenAt        time.Time       `json:"first_seen_at"`
	LastSeenAt         time.Time       `json:"last_seen_at"`
	Evidence           []FlakeEvidence `json:"evidence"`
}

// FlakeListFilters represents filtering options for flake list queries
type FlakeListFilters struct {
	Days    int
	Repo    string
	JobName string
}

// DetectionContext contains context needed for flake detection
type DetectionContext struct {
	ProjectID  uuid.UUID
	CIRunID    uuid.UUID
	RunURL     *string
	SHA        string
	JobName    string
	JobVariant *string
}
