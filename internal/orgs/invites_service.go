package orgs

import (
	"context"
	"errors"
	"fmt"
	"net/mail"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
)

const inviteTTL = 7 * 24 * time.Hour

func normalizeInviteEmail(email string) (string, error) {
	email = strings.TrimSpace(email)
	if email == "" {
		return "", errors.New("email is required")
	}
	if len(email) > 320 {
		return "", errors.New("email is too long")
	}
	if _, err := mail.ParseAddress(email); err != nil {
		return "", errors.New("invalid email address")
	}
	return email, nil
}

func (s *Service) CreateInvite(ctx context.Context, orgID, actorUserID uuid.UUID, email string, role OrgRole) (*Invite, string, error) {
	email, err := normalizeInviteEmail(email)
	if err != nil {
		return nil, "", err
	}

	if !role.IsValid() {
		return nil, "", ErrInvalidOrgRole
	}
	if role == RoleOwner {
		return nil, "", ErrCannotInviteOwner
	}

	actorRole, err := s.RequireOrgMutatePermission(ctx, actorUserID, orgID)
	if err != nil {
		return nil, "", err
	}
	if actorRole == RoleAdmin && role == RoleAdmin {
		return nil, "", ErrInsufficientPermissions
	}

	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return nil, "", fmt.Errorf("failed to begin transaction: %w", err)
	}
	defer func() {
		_ = tx.Rollback(ctx)
	}()

	// Revoke any existing open invites for this email in the org.
	_, err = tx.Exec(ctx, `
		UPDATE org_invites
		SET revoked_at = NOW(), revoked_by_user_id = $3
		WHERE org_id = $1
		  AND email = $2
		  AND accepted_at IS NULL
		  AND revoked_at IS NULL
	`, orgID, email, actorUserID)
	if err != nil {
		return nil, "", fmt.Errorf("failed to revoke existing invites: %w", err)
	}

	var invite Invite
	for attempt := 0; attempt < 3; attempt++ {
		token, tokenHash, err := GenerateInviteToken()
		if err != nil {
			return nil, "", err
		}

		expiresAt := time.Now().UTC().Add(inviteTTL)

		err = tx.QueryRow(ctx, `
			INSERT INTO org_invites (
			  org_id, email, role, token_hash, created_by_user_id, expires_at
			)
			VALUES ($1, $2, $3, $4, $5, $6)
			RETURNING id, org_id, email, role, created_at, expires_at
		`, orgID, email, role, tokenHash, actorUserID, expiresAt).Scan(
			&invite.ID,
			&invite.OrgID,
			&invite.Email,
			&invite.Role,
			&invite.CreatedAt,
			&invite.ExpiresAt,
		)
		if err == nil {
			if err := tx.Commit(ctx); err != nil {
				return nil, "", fmt.Errorf("failed to commit transaction: %w", err)
			}
			return &invite, token, nil
		}

		var pgErr *pgconn.PgError
		if errors.As(err, &pgErr) && pgErr.Code == "23505" {
			// Token hash collision (extremely unlikely); retry.
			continue
		}
		return nil, "", fmt.Errorf("failed to create invite: %w", err)
	}

	return nil, "", fmt.Errorf("failed to create invite: token collision retry exhausted")
}

func (s *Service) ListInvites(ctx context.Context, orgID, actorUserID uuid.UUID) ([]InviteListItem, error) {
	if _, err := s.RequireOrgMutatePermission(ctx, actorUserID, orgID); err != nil {
		return nil, err
	}

	rows, err := s.pool.Query(ctx, `
		SELECT
		  i.id,
		  i.email,
		  i.role,
		  i.created_at,
		  i.expires_at,
		  u.email AS created_by_email
		FROM org_invites i
		INNER JOIN users u ON u.id = i.created_by_user_id
		WHERE i.org_id = $1
		  AND i.accepted_at IS NULL
		  AND i.revoked_at IS NULL
		  AND i.expires_at > NOW()
		ORDER BY i.created_at DESC
	`, orgID)
	if err != nil {
		return nil, fmt.Errorf("failed to list invites: %w", err)
	}
	defer rows.Close()

	var invites []InviteListItem
	for rows.Next() {
		var inv InviteListItem
		if err := rows.Scan(&inv.ID, &inv.Email, &inv.Role, &inv.CreatedAt, &inv.ExpiresAt, &inv.CreatedByEmail); err != nil {
			return nil, fmt.Errorf("failed to scan invite: %w", err)
		}
		invites = append(invites, inv)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("error iterating invites: %w", err)
	}

	return invites, nil
}

func (s *Service) RevokeInvite(ctx context.Context, orgID, inviteID, actorUserID uuid.UUID) error {
	if _, err := s.RequireOrgMutatePermission(ctx, actorUserID, orgID); err != nil {
		return err
	}

	tag, err := s.pool.Exec(ctx, `
		UPDATE org_invites
		SET revoked_at = NOW(), revoked_by_user_id = $3
		WHERE id = $1
		  AND org_id = $2
		  AND accepted_at IS NULL
		  AND revoked_at IS NULL
	`, inviteID, orgID, actorUserID)
	if err != nil {
		return fmt.Errorf("failed to revoke invite: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return ErrInviteNotFound
	}

	return nil
}

func (s *Service) AcceptInvite(ctx context.Context, token string, userID uuid.UUID) (inviteID, orgID uuid.UUID, role OrgRole, err error) {
	if !ValidateInviteTokenFormat(token) {
		return uuid.Nil, uuid.Nil, "", ErrInviteNotFound
	}
	tokenHash := HashInviteToken(token)

	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return uuid.Nil, uuid.Nil, "", fmt.Errorf("failed to begin transaction: %w", err)
	}
	defer func() {
		_ = tx.Rollback(ctx)
	}()

	var invite Invite
	var acceptedAt *time.Time
	var revokedAt *time.Time
	err = tx.QueryRow(ctx, `
		SELECT id, org_id, email, role, created_at, expires_at, accepted_at, revoked_at
		FROM org_invites
		WHERE token_hash = $1
		FOR UPDATE
	`, tokenHash).Scan(
		&invite.ID,
		&invite.OrgID,
		&invite.Email,
		&invite.Role,
		&invite.CreatedAt,
		&invite.ExpiresAt,
		&acceptedAt,
		&revokedAt,
	)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return uuid.Nil, uuid.Nil, "", ErrInviteNotFound
		}
		return uuid.Nil, uuid.Nil, "", fmt.Errorf("failed to load invite: %w", err)
	}

	if acceptedAt != nil || revokedAt != nil {
		return uuid.Nil, uuid.Nil, "", ErrInviteNotActive
	}
	if !invite.ExpiresAt.After(time.Now().UTC()) {
		return uuid.Nil, uuid.Nil, "", ErrInviteExpired
	}

	var userEmail string
	err = tx.QueryRow(ctx, `SELECT email FROM users WHERE id = $1`, userID).Scan(&userEmail)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return uuid.Nil, uuid.Nil, "", fmt.Errorf("user not found")
		}
		return uuid.Nil, uuid.Nil, "", fmt.Errorf("failed to load user: %w", err)
	}
	if !strings.EqualFold(userEmail, invite.Email) {
		return uuid.Nil, uuid.Nil, "", ErrInviteEmailMismatch
	}

	_, err = tx.Exec(ctx, `
		INSERT INTO org_memberships (org_id, user_id, role)
		VALUES ($1, $2, $3)
		ON CONFLICT (org_id, user_id) DO NOTHING
	`, invite.OrgID, userID, invite.Role)
	if err != nil {
		return uuid.Nil, uuid.Nil, "", fmt.Errorf("failed to create membership: %w", err)
	}

	tag, err := tx.Exec(ctx, `
		UPDATE org_invites
		SET accepted_at = NOW(), accepted_by_user_id = $2
		WHERE id = $1
		  AND accepted_at IS NULL
		  AND revoked_at IS NULL
	`, invite.ID, userID)
	if err != nil {
		return uuid.Nil, uuid.Nil, "", fmt.Errorf("failed to mark invite accepted: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return uuid.Nil, uuid.Nil, "", ErrInviteNotActive
	}

	var finalRole OrgRole
	if err := tx.QueryRow(ctx, `
		SELECT role
		FROM org_memberships
		WHERE org_id = $1 AND user_id = $2
	`, invite.OrgID, userID).Scan(&finalRole); err != nil {
		return uuid.Nil, uuid.Nil, "", fmt.Errorf("failed to load membership: %w", err)
	}

	if err := tx.Commit(ctx); err != nil {
		return uuid.Nil, uuid.Nil, "", fmt.Errorf("failed to commit transaction: %w", err)
	}

	return invite.ID, invite.OrgID, finalRole, nil
}
