package audit

import (
	"context"
	"encoding/json"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/rs/zerolog/log"
)

// Event types for audit logging
const (
	EventOrgCreated      = "org.created"
	EventProjectCreated  = "project.created"
	EventAPIKeyCreated   = "apikey.created"
	EventAPIKeyRevoked   = "apikey.revoked"
	EventSlackConfigured = "slack.configured"
	EventSlackRemoved    = "slack.removed"
)

// Event represents an audit log entry
type Event struct {
	ID          uuid.UUID              `db:"id"`
	OrgID       uuid.NullUUID          `db:"org_id"`
	ProjectID   uuid.NullUUID          `db:"project_id"`
	ActorUserID uuid.NullUUID          `db:"actor_user_id"`
	Action      string                 `db:"action"`
	Meta        map[string]interface{} `db:"meta"`
	CreatedAt   time.Time              `db:"created_at"`
}

// Writer provides methods to write audit log entries
type Writer struct {
	pool *pgxpool.Pool
}

// NewWriter creates a new audit log writer
func NewWriter(pool *pgxpool.Pool) *Writer {
	return &Writer{pool: pool}
}

// LogParams contains parameters for logging an audit event
type LogParams struct {
	OrgID       *uuid.UUID
	ProjectID   *uuid.UUID
	ActorUserID *uuid.UUID
	Action      string
	Meta        map[string]interface{}
}

// Log writes an audit log entry to the database
func (w *Writer) Log(ctx context.Context, params LogParams) error {
	// Convert meta to JSON (always set, default to empty object)
	var metaJSON []byte
	var err error
	if params.Meta != nil {
		metaJSON, err = json.Marshal(params.Meta)
		if err != nil {
			log.Error().Err(err).Msg("Failed to marshal audit meta")
			return err
		}
	} else {
		metaJSON = []byte("{}")
	}

	query := `
		INSERT INTO audit_log (org_id, project_id, actor_user_id, action, meta)
		VALUES ($1, $2, $3, $4, $5)
	`

	_, err = w.pool.Exec(ctx, query,
		params.OrgID,
		params.ProjectID,
		params.ActorUserID,
		params.Action,
		metaJSON,
	)

	if err != nil {
		log.Error().Err(err).Str("action", params.Action).Msg("Failed to write audit log")
		return err
	}

	log.Info().
		Str("action", params.Action).
		Interface("org_id", params.OrgID).
		Interface("project_id", params.ProjectID).
		Interface("actor_user_id", params.ActorUserID).
		Msg("Audit event logged")

	return nil
}

// LogOrgCreated logs an organization creation event
func (w *Writer) LogOrgCreated(ctx context.Context, orgID, userID uuid.UUID, slug string) error {
	return w.Log(ctx, LogParams{
		OrgID:       &orgID,
		ActorUserID: &userID,
		Action:      EventOrgCreated,
		Meta: map[string]interface{}{
			"slug": slug,
		},
	})
}

// LogProjectCreated logs a project creation event
func (w *Writer) LogProjectCreated(ctx context.Context, orgID, projectID, userID uuid.UUID, slug string) error {
	return w.Log(ctx, LogParams{
		OrgID:       &orgID,
		ProjectID:   &projectID,
		ActorUserID: &userID,
		Action:      EventProjectCreated,
		Meta: map[string]interface{}{
			"slug": slug,
		},
	})
}

// LogAPIKeyCreated logs an API key creation event
func (w *Writer) LogAPIKeyCreated(ctx context.Context, orgID, projectID, apiKeyID, userID uuid.UUID, name string) error {
	return w.Log(ctx, LogParams{
		OrgID:       &orgID,
		ProjectID:   &projectID,
		ActorUserID: &userID,
		Action:      EventAPIKeyCreated,
		Meta: map[string]interface{}{
			"name": name,
		},
	})
}

// LogAPIKeyRevoked logs an API key revocation event
func (w *Writer) LogAPIKeyRevoked(ctx context.Context, orgID, projectID, apiKeyID, userID uuid.UUID, name string) error {
	return w.Log(ctx, LogParams{
		OrgID:       &orgID,
		ProjectID:   &projectID,
		ActorUserID: &userID,
		Action:      EventAPIKeyRevoked,
		Meta: map[string]interface{}{
			"name": name,
		},
	})
}

// LogSlackConfigured logs a Slack webhook configuration event
func (w *Writer) LogSlackConfigured(ctx context.Context, orgID, projectID, userID uuid.UUID) error {
	return w.Log(ctx, LogParams{
		OrgID:       &orgID,
		ProjectID:   &projectID,
		ActorUserID: &userID,
		Action:      EventSlackConfigured,
		Meta:        map[string]interface{}{},
	})
}

// LogSlackRemoved logs a Slack webhook removal event
func (w *Writer) LogSlackRemoved(ctx context.Context, orgID, projectID, userID uuid.UUID) error {
	return w.Log(ctx, LogParams{
		OrgID:       &orgID,
		ProjectID:   &projectID,
		ActorUserID: &userID,
		Action:      EventSlackRemoved,
		Meta:        map[string]interface{}{},
	})
}
