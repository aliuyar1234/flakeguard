package audit

import (
	"context"
	"encoding/json"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/rs/zerolog/log"
)

const (
	EventUserSignup           = "user.signup"
	EventLoginFailed          = "auth.login_failed"
	EventOrgCreated           = "org.created"
	EventOrgInviteCreated     = "org.invite_created"
	EventOrgInviteRevoked     = "org.invite_revoked"
	EventOrgInviteAccepted    = "org.invite_accepted"
	EventOrgMemberRoleUpdated = "org.member_role_updated"
	EventOrgMemberRemoved     = "org.member_removed"
	EventProjectCreated       = "project.created"
	EventAPIKeyCreated        = "apikey.created"
	EventAPIKeyRevoked        = "apikey.revoked"
	EventAPIKeyRotated        = "apikey.rotated"
	EventSlackConfigured      = "slack.configured"
	EventSlackCleared         = "slack.cleared"
)

// Event represents an audit log entry.
type Event struct {
	ID          uuid.UUID              `db:"id"`
	OrgID       uuid.NullUUID          `db:"org_id"`
	ProjectID   uuid.NullUUID          `db:"project_id"`
	ActorUserID uuid.NullUUID          `db:"actor_user_id"`
	Action      string                 `db:"action"`
	Meta        map[string]interface{} `db:"meta"`
	CreatedAt   time.Time              `db:"created_at"`
}

// Writer provides methods to write audit log entries.
type Writer struct {
	pool *pgxpool.Pool
}

func NewWriter(pool *pgxpool.Pool) *Writer {
	return &Writer{pool: pool}
}

// LogParams contains parameters for logging an audit event.
type LogParams struct {
	OrgID       *uuid.UUID
	ProjectID   *uuid.UUID
	ActorUserID *uuid.UUID
	Action      string
	Meta        map[string]interface{}
}

func (w *Writer) Log(ctx context.Context, params LogParams) error {
	metaJSON := []byte("{}")
	if params.Meta != nil {
		b, err := json.Marshal(params.Meta)
		if err != nil {
			log.Error().Err(err).Msg("Failed to marshal audit meta")
			return err
		}
		metaJSON = b
	}

	query := `
		INSERT INTO audit_log (org_id, project_id, actor_user_id, action, meta)
		VALUES ($1, $2, $3, $4, $5)
	`

	orgID := toNullUUID(params.OrgID)
	projectID := toNullUUID(params.ProjectID)
	actorUserID := toNullUUID(params.ActorUserID)

	_, err := w.pool.Exec(ctx, query, orgID, projectID, actorUserID, params.Action, metaJSON)
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

func toNullUUID(id *uuid.UUID) uuid.NullUUID {
	if id == nil {
		return uuid.NullUUID{}
	}
	return uuid.NullUUID{UUID: *id, Valid: true}
}

func (w *Writer) LogUserSignup(ctx context.Context, userID uuid.UUID, email string) error {
	return w.Log(ctx, LogParams{
		ActorUserID: &userID,
		Action:      EventUserSignup,
		Meta: map[string]interface{}{
			"email": email,
		},
	})
}

func (w *Writer) LogLoginFailed(ctx context.Context, email, ip string) error {
	return w.Log(ctx, LogParams{
		Action: EventLoginFailed,
		Meta: map[string]interface{}{
			"email": email,
			"ip":    ip,
		},
	})
}

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

func (w *Writer) LogOrgInviteCreated(ctx context.Context, orgID, actorUserID, inviteID uuid.UUID, email, role string) error {
	return w.Log(ctx, LogParams{
		OrgID:       &orgID,
		ActorUserID: &actorUserID,
		Action:      EventOrgInviteCreated,
		Meta: map[string]interface{}{
			"invite_id": inviteID.String(),
			"email":     email,
			"role":      role,
		},
	})
}

func (w *Writer) LogOrgInviteRevoked(ctx context.Context, orgID, actorUserID, inviteID uuid.UUID) error {
	return w.Log(ctx, LogParams{
		OrgID:       &orgID,
		ActorUserID: &actorUserID,
		Action:      EventOrgInviteRevoked,
		Meta: map[string]interface{}{
			"invite_id": inviteID.String(),
		},
	})
}

func (w *Writer) LogOrgInviteAccepted(ctx context.Context, orgID, actorUserID, inviteID uuid.UUID) error {
	return w.Log(ctx, LogParams{
		OrgID:       &orgID,
		ActorUserID: &actorUserID,
		Action:      EventOrgInviteAccepted,
		Meta: map[string]interface{}{
			"invite_id": inviteID.String(),
		},
	})
}

func (w *Writer) LogOrgMemberRoleUpdated(ctx context.Context, orgID, actorUserID, targetUserID uuid.UUID, previousRole, newRole string) error {
	return w.Log(ctx, LogParams{
		OrgID:       &orgID,
		ActorUserID: &actorUserID,
		Action:      EventOrgMemberRoleUpdated,
		Meta: map[string]interface{}{
			"target_user_id": targetUserID.String(),
			"previous_role":  previousRole,
			"new_role":       newRole,
		},
	})
}

func (w *Writer) LogOrgMemberRemoved(ctx context.Context, orgID, actorUserID, targetUserID uuid.UUID, removedRole string) error {
	return w.Log(ctx, LogParams{
		OrgID:       &orgID,
		ActorUserID: &actorUserID,
		Action:      EventOrgMemberRemoved,
		Meta: map[string]interface{}{
			"target_user_id": targetUserID.String(),
			"role":           removedRole,
		},
	})
}

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

func (w *Writer) LogAPIKeyRotated(ctx context.Context, orgID, projectID, oldAPIKeyID, newAPIKeyID, userID uuid.UUID, oldName, newName string) error {
	return w.Log(ctx, LogParams{
		OrgID:       &orgID,
		ProjectID:   &projectID,
		ActorUserID: &userID,
		Action:      EventAPIKeyRotated,
		Meta: map[string]interface{}{
			"old_api_key_id": oldAPIKeyID.String(),
			"new_api_key_id": newAPIKeyID.String(),
			"old_name":       oldName,
			"new_name":       newName,
		},
	})
}

func (w *Writer) LogSlackConfigured(ctx context.Context, orgID, projectID, userID uuid.UUID) error {
	return w.Log(ctx, LogParams{
		OrgID:       &orgID,
		ProjectID:   &projectID,
		ActorUserID: &userID,
		Action:      EventSlackConfigured,
		Meta:        map[string]interface{}{},
	})
}

func (w *Writer) LogSlackCleared(ctx context.Context, orgID, projectID, userID uuid.UUID) error {
	return w.Log(ctx, LogParams{
		OrgID:       &orgID,
		ProjectID:   &projectID,
		ActorUserID: &userID,
		Action:      EventSlackCleared,
		Meta:        map[string]interface{}{},
	})
}

// LogSlackRemoved is kept for backward compatibility.
func (w *Writer) LogSlackRemoved(ctx context.Context, orgID, projectID, userID uuid.UUID) error {
	return w.LogSlackCleared(ctx, orgID, projectID, userID)
}
