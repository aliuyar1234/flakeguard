package audit

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
)

type Reader struct {
	pool *pgxpool.Pool
}

func NewReader(pool *pgxpool.Pool) *Reader {
	return &Reader{pool: pool}
}

type ListItem struct {
	ID          uuid.UUID      `json:"id"`
	Action      string         `json:"action"`
	OrgID       uuid.UUID      `json:"org_id"`
	ProjectID   *uuid.UUID     `json:"project_id,omitempty"`
	ActorUserID *uuid.UUID     `json:"actor_user_id,omitempty"`
	ActorEmail  string         `json:"actor_email,omitempty"`
	Meta        map[string]any `json:"meta"`
	CreatedAt   time.Time      `json:"created_at"`
}

func (r *Reader) ListByOrg(ctx context.Context, orgID uuid.UUID, limit int) ([]ListItem, error) {
	if limit <= 0 || limit > 200 {
		limit = 50
	}

	rows, err := r.pool.Query(ctx, `
		SELECT
		  al.id,
		  al.org_id,
		  al.project_id,
		  al.actor_user_id,
		  u.email,
		  al.action,
		  al.meta,
		  al.created_at
		FROM audit_log al
		LEFT JOIN users u ON u.id = al.actor_user_id
		WHERE al.org_id = $1
		ORDER BY al.created_at DESC
		LIMIT $2
	`, orgID, limit)
	if err != nil {
		return nil, fmt.Errorf("failed to query audit log: %w", err)
	}
	defer rows.Close()

	var out []ListItem
	for rows.Next() {
		var item ListItem
		var projectID uuid.NullUUID
		var actorUserID uuid.NullUUID
		var actorEmail *string
		var metaRaw []byte

		if err := rows.Scan(&item.ID, &item.OrgID, &projectID, &actorUserID, &actorEmail, &item.Action, &metaRaw, &item.CreatedAt); err != nil {
			return nil, fmt.Errorf("failed to scan audit row: %w", err)
		}

		if projectID.Valid {
			item.ProjectID = &projectID.UUID
		}
		if actorUserID.Valid {
			item.ActorUserID = &actorUserID.UUID
		}
		if actorEmail != nil {
			item.ActorEmail = *actorEmail
		}

		item.Meta = map[string]any{}
		if len(metaRaw) > 0 {
			_ = json.Unmarshal(metaRaw, &item.Meta)
		}

		out = append(out, item)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("error iterating audit rows: %w", err)
	}

	return out, nil
}
