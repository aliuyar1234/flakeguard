package orgs

import (
	"context"
	"errors"
	"fmt"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
)

func (s *Service) UpdateMemberRole(ctx context.Context, orgID, actorUserID, targetUserID uuid.UUID, newRole OrgRole) (previousRole OrgRole, err error) {
	if !newRole.IsValid() {
		return "", ErrInvalidOrgRole
	}

	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return "", fmt.Errorf("failed to begin transaction: %w", err)
	}
	defer func() {
		_ = tx.Rollback(ctx)
	}()

	var actorRole OrgRole
	if err := tx.QueryRow(ctx, `
		SELECT role
		FROM org_memberships
		WHERE org_id = $1 AND user_id = $2
	`, orgID, actorUserID).Scan(&actorRole); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return "", ErrNotMember
		}
		return "", fmt.Errorf("failed to load actor role: %w", err)
	}
	if !actorRole.CanMutate() {
		return "", ErrInsufficientPermissions
	}

	var currentRole OrgRole
	if err := tx.QueryRow(ctx, `
		SELECT role
		FROM org_memberships
		WHERE org_id = $1 AND user_id = $2
		FOR UPDATE
	`, orgID, targetUserID).Scan(&currentRole); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return "", ErrMemberNotFound
		}
		return "", fmt.Errorf("failed to load member role: %w", err)
	}

	if actorRole == RoleAdmin {
		if targetUserID == actorUserID {
			if newRole != RoleMember && newRole != RoleViewer {
				return "", ErrInsufficientPermissions
			}
		} else {
			if currentRole != RoleMember && currentRole != RoleViewer {
				return "", ErrInsufficientPermissions
			}
			if newRole != RoleMember && newRole != RoleViewer {
				return "", ErrInsufficientPermissions
			}
		}
	}

	if currentRole == RoleOwner && newRole != RoleOwner {
		rows, err := tx.Query(ctx, `
			SELECT user_id
			FROM org_memberships
			WHERE org_id = $1 AND role = $2
			FOR UPDATE
		`, orgID, RoleOwner)
		if err != nil {
			return "", fmt.Errorf("failed to lock owners: %w", err)
		}
		var owners int
		for rows.Next() {
			owners++
		}
		if err := rows.Err(); err != nil {
			rows.Close()
			return "", fmt.Errorf("failed to lock owners: %w", err)
		}
		rows.Close()
		if owners <= 1 {
			return "", ErrCannotDemoteLastOwner
		}
	}

	if _, err := tx.Exec(ctx, `
		UPDATE org_memberships
		SET role = $3
		WHERE org_id = $1 AND user_id = $2
	`, orgID, targetUserID, newRole); err != nil {
		return "", fmt.Errorf("failed to update member role: %w", err)
	}

	if err := tx.Commit(ctx); err != nil {
		return "", fmt.Errorf("failed to commit transaction: %w", err)
	}

	return currentRole, nil
}

func (s *Service) RemoveMember(ctx context.Context, orgID, actorUserID, targetUserID uuid.UUID) (removedRole OrgRole, err error) {
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return "", fmt.Errorf("failed to begin transaction: %w", err)
	}
	defer func() {
		_ = tx.Rollback(ctx)
	}()

	var actorRole OrgRole
	if err := tx.QueryRow(ctx, `
		SELECT role
		FROM org_memberships
		WHERE org_id = $1 AND user_id = $2
	`, orgID, actorUserID).Scan(&actorRole); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return "", ErrNotMember
		}
		return "", fmt.Errorf("failed to load actor role: %w", err)
	}
	if !actorRole.CanMutate() {
		return "", ErrInsufficientPermissions
	}

	var targetRole OrgRole
	if err := tx.QueryRow(ctx, `
		SELECT role
		FROM org_memberships
		WHERE org_id = $1 AND user_id = $2
		FOR UPDATE
	`, orgID, targetUserID).Scan(&targetRole); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return "", ErrMemberNotFound
		}
		return "", fmt.Errorf("failed to load member role: %w", err)
	}

	if actorRole == RoleAdmin && targetUserID != actorUserID {
		if targetRole != RoleMember && targetRole != RoleViewer {
			return "", ErrInsufficientPermissions
		}
	}

	if targetRole == RoleOwner {
		rows, err := tx.Query(ctx, `
			SELECT user_id
			FROM org_memberships
			WHERE org_id = $1 AND role = $2
			FOR UPDATE
		`, orgID, RoleOwner)
		if err != nil {
			return "", fmt.Errorf("failed to lock owners: %w", err)
		}
		var owners int
		for rows.Next() {
			owners++
		}
		if err := rows.Err(); err != nil {
			rows.Close()
			return "", fmt.Errorf("failed to lock owners: %w", err)
		}
		rows.Close()
		if owners <= 1 {
			return "", ErrCannotRemoveLastOwner
		}
	}

	tag, err := tx.Exec(ctx, `
		DELETE FROM org_memberships
		WHERE org_id = $1 AND user_id = $2
	`, orgID, targetUserID)
	if err != nil {
		return "", fmt.Errorf("failed to remove member: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return "", ErrMemberNotFound
	}

	if err := tx.Commit(ctx); err != nil {
		return "", fmt.Errorf("failed to commit transaction: %w", err)
	}

	return targetRole, nil
}
