package orgs

import (
	"context"
	"errors"
	"fmt"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/rs/zerolog/log"
)

var (
	// ErrOrgNotFound is returned when an organization is not found
	ErrOrgNotFound = errors.New("organization not found")

	// ErrSlugConflict is returned when an organization slug already exists
	ErrSlugConflict = errors.New("organization slug already exists")

	// ErrNotMember is returned when a user is not a member of an organization
	ErrNotMember = errors.New("user is not a member of this organization")

	// ErrInsufficientPermissions is returned when a user lacks required permissions
	ErrInsufficientPermissions = errors.New("insufficient permissions")
)

// Service provides organization-related operations
type Service struct {
	pool *pgxpool.Pool
}

// NewService creates a new organization service
func NewService(pool *pgxpool.Pool) *Service {
	return &Service{pool: pool}
}

// GetByID retrieves an organization by ID
func (s *Service) GetByID(ctx context.Context, orgID uuid.UUID) (*Org, error) {
	var org Org

	query := `
		SELECT id, name, slug, created_by_user_id, created_at, updated_at
		FROM orgs
		WHERE id = $1
	`

	err := s.pool.QueryRow(ctx, query, orgID).Scan(
		&org.ID,
		&org.Name,
		&org.Slug,
		&org.CreatedByUserID,
		&org.CreatedAt,
		&org.UpdatedAt,
	)

	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, ErrOrgNotFound
		}
		return nil, fmt.Errorf("failed to get organization: %w", err)
	}

	return &org, nil
}

// GetBySlug retrieves an organization by slug
func (s *Service) GetBySlug(ctx context.Context, slug string) (*Org, error) {
	var org Org

	query := `
		SELECT id, name, slug, created_by_user_id, created_at, updated_at
		FROM orgs
		WHERE slug = $1
	`

	err := s.pool.QueryRow(ctx, query, slug).Scan(
		&org.ID,
		&org.Name,
		&org.Slug,
		&org.CreatedByUserID,
		&org.CreatedAt,
		&org.UpdatedAt,
	)

	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, ErrOrgNotFound
		}
		return nil, fmt.Errorf("failed to get organization: %w", err)
	}

	return &org, nil
}

// ListUserOrgs retrieves all organizations for a user with their roles
func (s *Service) ListUserOrgs(ctx context.Context, userID uuid.UUID) ([]OrgWithRole, error) {
	query := `
		SELECT o.id, o.name, o.slug, o.created_by_user_id, o.created_at, o.updated_at, m.role
		FROM orgs o
		INNER JOIN org_memberships m ON o.id = m.org_id
		WHERE m.user_id = $1
		ORDER BY o.created_at DESC
	`

	rows, err := s.pool.Query(ctx, query, userID)
	if err != nil {
		return nil, fmt.Errorf("failed to list user orgs: %w", err)
	}
	defer rows.Close()

	var orgs []OrgWithRole
	for rows.Next() {
		var org OrgWithRole
		err := rows.Scan(
			&org.ID,
			&org.Name,
			&org.Slug,
			&org.CreatedByUserID,
			&org.CreatedAt,
			&org.UpdatedAt,
			&org.Role,
		)
		if err != nil {
			return nil, fmt.Errorf("failed to scan org: %w", err)
		}
		orgs = append(orgs, org)
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("error iterating org rows: %w", err)
	}

	return orgs, nil
}

// CreateWithOwner creates a new organization and makes the user an OWNER
// This must be done in a transaction
func (s *Service) CreateWithOwner(ctx context.Context, name, slug string, userID uuid.UUID) (*Org, error) {
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to begin transaction: %w", err)
	}
	defer tx.Rollback(ctx)

	// Create organization
	var org Org
	query := `
		INSERT INTO orgs (name, slug, created_by_user_id)
		VALUES ($1, $2, $3)
		RETURNING id, name, slug, created_by_user_id, created_at, updated_at
	`

	err = tx.QueryRow(ctx, query, name, slug, userID).Scan(
		&org.ID,
		&org.Name,
		&org.Slug,
		&org.CreatedByUserID,
		&org.CreatedAt,
		&org.UpdatedAt,
	)

	if err != nil {
		var pgErr *pgconn.PgError
		if errors.As(err, &pgErr) && pgErr.Code == "23505" { // unique_violation
			return nil, ErrSlugConflict
		}
		return nil, fmt.Errorf("failed to create organization: %w", err)
	}

	// Create membership with OWNER role
	memberQuery := `
		INSERT INTO org_memberships (org_id, user_id, role)
		VALUES ($1, $2, $3)
	`

	_, err = tx.Exec(ctx, memberQuery, org.ID, userID, RoleOwner)
	if err != nil {
		return nil, fmt.Errorf("failed to create membership: %w", err)
	}

	// Commit transaction
	if err := tx.Commit(ctx); err != nil {
		return nil, fmt.Errorf("failed to commit transaction: %w", err)
	}

	return &org, nil
}

// ListMembers retrieves all members of an organization
func (s *Service) ListMembers(ctx context.Context, orgID uuid.UUID) ([]MemberInfo, error) {
	query := `
		SELECT m.user_id, u.email, m.role, m.created_at
		FROM org_memberships m
		INNER JOIN users u ON m.user_id = u.id
		WHERE m.org_id = $1
		ORDER BY m.created_at ASC
	`

	rows, err := s.pool.Query(ctx, query, orgID)
	if err != nil {
		return nil, fmt.Errorf("failed to list members: %w", err)
	}
	defer rows.Close()

	var members []MemberInfo
	for rows.Next() {
		var member MemberInfo
		err := rows.Scan(
			&member.UserID,
			&member.Email,
			&member.Role,
			&member.CreatedAt,
		)
		if err != nil {
			return nil, fmt.Errorf("failed to scan member: %w", err)
		}
		members = append(members, member)
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("error iterating member rows: %w", err)
	}

	return members, nil
}

// CheckOrgRole verifies that a user has the required role in an organization
// Returns the user's actual role if they are a member
// Returns ErrNotMember if the user is not a member
// Returns ErrInsufficientPermissions if the user's role is insufficient
func (s *Service) CheckOrgRole(ctx context.Context, userID, orgID uuid.UUID, requiredRole OrgRole) (OrgRole, error) {
	var role OrgRole

	query := `
		SELECT role FROM org_memberships
		WHERE org_id = $1 AND user_id = $2
	`

	err := s.pool.QueryRow(ctx, query, orgID, userID).Scan(&role)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			// User is not a member - log and return 404-friendly error
			log.Debug().
				Str("user_id", userID.String()).
				Str("org_id", orgID.String()).
				Msg("RBAC: User is not a member of organization")
			return "", ErrNotMember
		}
		return "", fmt.Errorf("failed to check org membership: %w", err)
	}

	// Check if the user's role is sufficient
	if !hasPermission(role, requiredRole) {
		log.Warn().
			Str("user_id", userID.String()).
			Str("org_id", orgID.String()).
			Str("user_role", string(role)).
			Str("required_role", string(requiredRole)).
			Msg("RBAC: Insufficient permissions")
		return role, ErrInsufficientPermissions
	}

	return role, nil
}

// GetUserOrgRole retrieves a user's role in an organization
// Returns ErrNotMember if the user is not a member
func (s *Service) GetUserOrgRole(ctx context.Context, userID, orgID uuid.UUID) (OrgRole, error) {
	var role OrgRole

	query := `
		SELECT role FROM org_memberships
		WHERE org_id = $1 AND user_id = $2
	`

	err := s.pool.QueryRow(ctx, query, orgID, userID).Scan(&role)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return "", ErrNotMember
		}
		return "", fmt.Errorf("failed to get org role: %w", err)
	}

	return role, nil
}

// hasPermission checks if a user's role satisfies the required role
func hasPermission(userRole, requiredRole OrgRole) bool {
	// Define role hierarchy (higher number = more permissions)
	roleLevel := map[OrgRole]int{
		RoleViewer: 1,
		RoleMember: 2,
		RoleAdmin:  3,
		RoleOwner:  4,
	}

	return roleLevel[userRole] >= roleLevel[requiredRole]
}

// RequireOrgMember checks if a user is a member of an organization
// Returns the user's role if they are a member
func (s *Service) RequireOrgMember(ctx context.Context, userID, orgID uuid.UUID) (OrgRole, error) {
	return s.GetUserOrgRole(ctx, userID, orgID)
}

// RequireOrgMutatePermission checks if a user can mutate organization resources (OWNER or ADMIN)
func (s *Service) RequireOrgMutatePermission(ctx context.Context, userID, orgID uuid.UUID) (OrgRole, error) {
	return s.CheckOrgRole(ctx, userID, orgID, RoleAdmin)
}
