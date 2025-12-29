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

type ListByOrgOptions struct {
	Limit       int
	Offset      int
	Action      string
	ActorEmail  string
	ActorUserID *uuid.UUID
}

func (r *Reader) ListByOrgPage(ctx context.Context, orgID uuid.UUID, opts ListByOrgOptions) ([]ListItem, int, error) {
	limit := opts.Limit
	if limit <= 0 || limit > 200 {
		limit = 50
	}

	offset := opts.Offset
	if offset < 0 {
		offset = 0
	}

	where := `WHERE al.org_id = $1`
	args := []any{orgID}
	argNum := 2

	if opts.Action != "" {
		where += fmt.Sprintf(" AND al.action ILIKE $%d", argNum)
		args = append(args, "%"+opts.Action+"%")
		argNum++
	}

	if opts.ActorEmail != "" {
		where += fmt.Sprintf(" AND u.email ILIKE $%d", argNum)
		args = append(args, "%"+opts.ActorEmail+"%")
		argNum++
	}

	if opts.ActorUserID != nil {
		where += fmt.Sprintf(" AND al.actor_user_id = $%d", argNum)
		args = append(args, *opts.ActorUserID)
		argNum++
	}

	countQuery := `
		SELECT COUNT(*)
		FROM audit_log al
		LEFT JOIN users u ON u.id = al.actor_user_id
	` + where

	var total int
	if err := r.pool.QueryRow(ctx, countQuery, args...).Scan(&total); err != nil {
		return nil, 0, fmt.Errorf("failed to count audit log: %w", err)
	}

	listQuery := `
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
	` + where + fmt.Sprintf(`
		ORDER BY al.created_at DESC
		LIMIT $%d OFFSET $%d
	`, argNum, argNum+1)

	args = append(args, limit, offset)

	rows, err := r.pool.Query(ctx, listQuery, args...)
	if err != nil {
		return nil, 0, fmt.Errorf("failed to query audit log: %w", err)
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
			return nil, 0, fmt.Errorf("failed to scan audit row: %w", err)
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
		return nil, 0, fmt.Errorf("error iterating audit rows: %w", err)
	}

	return out, total, nil
}
