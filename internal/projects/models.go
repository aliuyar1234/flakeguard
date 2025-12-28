package projects

import (
	"database/sql"
	"time"

	"github.com/google/uuid"
)

// Project represents a project within an organization
type Project struct {
	ID              uuid.UUID      `db:"id"`
	OrgID           uuid.UUID      `db:"org_id"`
	Name            string         `db:"name"`
	Slug            string         `db:"slug"`
	DefaultBranch   string         `db:"default_branch"`
	SlackEnabled    bool           `db:"slack_enabled"`
	SlackWebhookURL sql.NullString `db:"slack_webhook_url"`
	CreatedByUserID uuid.UUID      `db:"created_by_user_id"`
	CreatedAt       time.Time      `db:"created_at"`
	UpdatedAt       time.Time      `db:"updated_at"`
}

// SlackConfig represents the Slack configuration for a project
// This is used for API requests/responses
type SlackConfig struct {
	WebhookURL string `json:"webhook_url"`
	Enabled    bool   `json:"enabled"`
}

// SlackStatus represents the Slack configuration status without exposing the URL
type SlackStatus struct {
	Enabled       bool `json:"enabled"`
	WebhookURLSet bool `json:"webhook_url_set"`
}

// HasSlackConfigured returns true if Slack webhook is configured
func (p *Project) HasSlackConfigured() bool {
	return p.SlackEnabled && p.SlackWebhookURL.Valid && p.SlackWebhookURL.String != ""
}
