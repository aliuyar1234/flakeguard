package ingest

import (
	"fmt"
	"time"
)

// IngestionMetadata contains all meta fields required for a JUnit ingestion (SSOT 8.9.1).
type IngestionMetadata struct {
	ProjectSlug      string `json:"project_slug"`
	RepoFullName     string `json:"repo_full_name"`
	WorkflowName     string `json:"workflow_name"`
	WorkflowRef      string `json:"workflow_ref"`
	GitHubRunID      int64  `json:"github_run_id"`
	GitHubRunAttempt int    `json:"github_run_attempt"`
	GitHubRunNumber  int64  `json:"github_run_number"`
	RunURL           string `json:"run_url"`
	SHA              string `json:"sha"`
	Branch           string `json:"branch"`
	Event            string `json:"event"`
	PRNumber         *int64 `json:"pr_number"`
	JobName          string `json:"job_name"`
	JobVariant       string `json:"job_variant"`
	StartedAt        string `json:"started_at"`
	CompletedAt      string `json:"completed_at"`

	startedAtTime   time.Time
	completedAtTime time.Time
}

type InvalidMetaError struct {
	Message string
}

func (e *InvalidMetaError) Error() string {
	return e.Message
}

func (m *IngestionMetadata) Validate() error {
	switch {
	case m.ProjectSlug == "":
		return &InvalidMetaError{Message: "meta.project_slug is required"}
	case m.RepoFullName == "":
		return &InvalidMetaError{Message: "meta.repo_full_name is required"}
	case m.WorkflowName == "":
		return &InvalidMetaError{Message: "meta.workflow_name is required"}
	case m.WorkflowRef == "":
		return &InvalidMetaError{Message: "meta.workflow_ref is required"}
	case m.GitHubRunID <= 0:
		return &InvalidMetaError{Message: "meta.github_run_id is required"}
	case m.GitHubRunAttempt < 1:
		return &InvalidMetaError{Message: "meta.github_run_attempt is required"}
	case m.GitHubRunNumber <= 0:
		return &InvalidMetaError{Message: "meta.github_run_number is required"}
	case m.RunURL == "":
		return &InvalidMetaError{Message: "meta.run_url is required"}
	case m.SHA == "":
		return &InvalidMetaError{Message: "meta.sha is required"}
	case m.Branch == "":
		return &InvalidMetaError{Message: "meta.branch is required"}
	case m.Event == "":
		return &InvalidMetaError{Message: "meta.event is required"}
	case m.JobName == "":
		return &InvalidMetaError{Message: "meta.job_name is required"}
	case m.StartedAt == "":
		return &InvalidMetaError{Message: "meta.started_at is required"}
	case m.CompletedAt == "":
		return &InvalidMetaError{Message: "meta.completed_at is required"}
	}

	startedAt, err := parseRFC3339("meta.started_at", m.StartedAt)
	if err != nil {
		return err
	}
	completedAt, err := parseRFC3339("meta.completed_at", m.CompletedAt)
	if err != nil {
		return err
	}

	m.startedAtTime = startedAt
	m.completedAtTime = completedAt

	return nil
}

func (m *IngestionMetadata) StartedAtTime() time.Time {
	return m.startedAtTime
}

func (m *IngestionMetadata) CompletedAtTime() time.Time {
	return m.completedAtTime
}

func parseRFC3339(field, value string) (time.Time, error) {
	if parsed, err := time.Parse(time.RFC3339, value); err == nil {
		return parsed, nil
	}
	parsed, err := time.Parse(time.RFC3339Nano, value)
	if err == nil {
		return parsed, nil
	}
	return time.Time{}, &InvalidMetaError{Message: fmt.Sprintf("%s must be RFC3339", field)}
}
