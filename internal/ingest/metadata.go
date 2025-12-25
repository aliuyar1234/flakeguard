package ingest

import (
	"errors"
	"fmt"
)

// IngestionMetadata contains all metadata fields required for an ingestion
type IngestionMetadata struct {
	// Required fields
	RepoFullName string `json:"repo_full_name"` // e.g., "owner/repo"
	SHA          string `json:"sha"`            // git commit SHA
	Branch       string `json:"branch"`         // git branch name
	WorkflowName string `json:"workflow_name"`  // GitHub Actions workflow name
	RunNumber    int    `json:"run_number"`     // GitHub Actions run number
	RunAttempt   int    `json:"run_attempt"`    // GitHub Actions attempt number (1-indexed)
	JobName      string `json:"job_name"`       // GitHub Actions job name
	JobVariant   string `json:"job_variant"`    // Job variant identifier (e.g., "ubuntu-latest-go-1.22")
	RunnerOS     string `json:"runner_os"`      // e.g., "Linux", "Windows", "macOS"
	RunnerArch   string `json:"runner_arch"`    // e.g., "X64", "ARM64"
	EventType    string `json:"event_type"`     // e.g., "push", "pull_request", "workflow_dispatch"
	RunURL       string `json:"run_url"`        // GitHub Actions run URL
	ProjectSlug  string `json:"project_slug"`   // FlakeGuard project slug
	TriggeredBy  string `json:"triggered_by"`   // User or actor who triggered the run
	RunID        int    `json:"run_id"`         // GitHub Actions run ID
}

var (
	// ErrMissingMetadata is returned when required metadata fields are missing
	ErrMissingMetadata = errors.New("missing required metadata fields")
)

// Validate checks that all required metadata fields are present
func (m *IngestionMetadata) Validate() error {
	var missing []string

	if m.RepoFullName == "" {
		missing = append(missing, "repo_full_name")
	}
	if m.SHA == "" {
		missing = append(missing, "sha")
	}
	if m.Branch == "" {
		missing = append(missing, "branch")
	}
	if m.WorkflowName == "" {
		missing = append(missing, "workflow_name")
	}
	if m.RunNumber == 0 {
		missing = append(missing, "run_number")
	}
	if m.RunAttempt == 0 {
		missing = append(missing, "run_attempt")
	}
	if m.JobName == "" {
		missing = append(missing, "job_name")
	}
	if m.JobVariant == "" {
		missing = append(missing, "job_variant")
	}
	if m.RunnerOS == "" {
		missing = append(missing, "runner_os")
	}
	if m.RunnerArch == "" {
		missing = append(missing, "runner_arch")
	}
	if m.EventType == "" {
		missing = append(missing, "event_type")
	}
	if m.RunURL == "" {
		missing = append(missing, "run_url")
	}
	if m.ProjectSlug == "" {
		missing = append(missing, "project_slug")
	}
	if m.TriggeredBy == "" {
		missing = append(missing, "triggered_by")
	}
	if m.RunID == 0 {
		missing = append(missing, "run_id")
	}

	if len(missing) > 0 {
		return fmt.Errorf("%w: %v", ErrMissingMetadata, missing)
	}

	return nil
}
