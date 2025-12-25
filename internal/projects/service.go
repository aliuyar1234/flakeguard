package projects

import (
	"context"
	"database/sql"
	"errors"
	"fmt"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"
)

var (
	// ErrProjectNotFound is returned when a project is not found
	ErrProjectNotFound = errors.New("project not found")

	// ErrSlugConflict is returned when a project slug already exists in the organization
	ErrSlugConflict = errors.New("project slug already exists in organization")
)

// Service provides project-related operations
type Service struct {
	pool *pgxpool.Pool
}

// NewService creates a new project service
func NewService(pool *pgxpool.Pool) *Service {
	return &Service{pool: pool}
}

// GetByID retrieves a project by ID
func (s *Service) GetByID(ctx context.Context, projectID uuid.UUID) (*Project, error) {
	var project Project

	query := `
		SELECT id, org_id, name, slug, default_branch, slack_enabled, slack_webhook_url,
		       created_by_user_id, created_at, updated_at
		FROM projects
		WHERE id = $1
	`

	err := s.pool.QueryRow(ctx, query, projectID).Scan(
		&project.ID,
		&project.OrgID,
		&project.Name,
		&project.Slug,
		&project.DefaultBranch,
		&project.SlackEnabled,
		&project.SlackWebhookURL,
		&project.CreatedByUserID,
		&project.CreatedAt,
		&project.UpdatedAt,
	)

	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, ErrProjectNotFound
		}
		return nil, fmt.Errorf("failed to get project: %w", err)
	}

	return &project, nil
}

// GetBySlug retrieves a project by slug (format: org-slug/project-slug)
func (s *Service) GetBySlug(ctx context.Context, slug string) (*Project, error) {
	var project Project

	query := `
		SELECT p.id, p.org_id, p.name, p.slug, p.default_branch, p.slack_enabled, p.slack_webhook_url,
		       p.created_by_user_id, p.created_at, p.updated_at
		FROM projects p
		JOIN orgs o ON p.org_id = o.id
		WHERE o.slug || '/' || p.slug = $1
	`

	err := s.pool.QueryRow(ctx, query, slug).Scan(
		&project.ID,
		&project.OrgID,
		&project.Name,
		&project.Slug,
		&project.DefaultBranch,
		&project.SlackEnabled,
		&project.SlackWebhookURL,
		&project.CreatedByUserID,
		&project.CreatedAt,
		&project.UpdatedAt,
	)

	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, ErrProjectNotFound
		}
		return nil, fmt.Errorf("failed to get project: %w", err)
	}

	return &project, nil
}

// GetByOrgAndSlug retrieves a project by organization ID and slug
func (s *Service) GetByOrgAndSlug(ctx context.Context, orgID uuid.UUID, slug string) (*Project, error) {
	var project Project

	query := `
		SELECT id, org_id, name, slug, default_branch, slack_enabled, slack_webhook_url,
		       created_by_user_id, created_at, updated_at
		FROM projects
		WHERE org_id = $1 AND slug = $2
	`

	err := s.pool.QueryRow(ctx, query, orgID, slug).Scan(
		&project.ID,
		&project.OrgID,
		&project.Name,
		&project.Slug,
		&project.DefaultBranch,
		&project.SlackEnabled,
		&project.SlackWebhookURL,
		&project.CreatedByUserID,
		&project.CreatedAt,
		&project.UpdatedAt,
	)

	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, ErrProjectNotFound
		}
		return nil, fmt.Errorf("failed to get project: %w", err)
	}

	return &project, nil
}

// ListByOrg retrieves all projects for an organization
func (s *Service) ListByOrg(ctx context.Context, orgID uuid.UUID) ([]Project, error) {
	query := `
		SELECT id, org_id, name, slug, default_branch, slack_enabled, slack_webhook_url,
		       created_by_user_id, created_at, updated_at
		FROM projects
		WHERE org_id = $1
		ORDER BY created_at DESC
	`

	rows, err := s.pool.Query(ctx, query, orgID)
	if err != nil {
		return nil, fmt.Errorf("failed to list projects: %w", err)
	}
	defer rows.Close()

	var projects []Project
	for rows.Next() {
		var project Project
		err := rows.Scan(
			&project.ID,
			&project.OrgID,
			&project.Name,
			&project.Slug,
			&project.DefaultBranch,
			&project.SlackEnabled,
			&project.SlackWebhookURL,
			&project.CreatedByUserID,
			&project.CreatedAt,
			&project.UpdatedAt,
		)
		if err != nil {
			return nil, fmt.Errorf("failed to scan project: %w", err)
		}
		projects = append(projects, project)
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("error iterating project rows: %w", err)
	}

	return projects, nil
}

// Create creates a new project
func (s *Service) Create(ctx context.Context, orgID uuid.UUID, name, slug, defaultBranch string, userID uuid.UUID) (*Project, error) {
	var project Project

	query := `
		INSERT INTO projects (org_id, name, slug, default_branch, created_by_user_id)
		VALUES ($1, $2, $3, $4, $5)
		RETURNING id, org_id, name, slug, default_branch, slack_enabled, slack_webhook_url,
		          created_by_user_id, created_at, updated_at
	`

	err := s.pool.QueryRow(ctx, query, orgID, name, slug, defaultBranch, userID).Scan(
		&project.ID,
		&project.OrgID,
		&project.Name,
		&project.Slug,
		&project.DefaultBranch,
		&project.SlackEnabled,
		&project.SlackWebhookURL,
		&project.CreatedByUserID,
		&project.CreatedAt,
		&project.UpdatedAt,
	)

	if err != nil {
		var pgErr *pgconn.PgError
		if errors.As(err, &pgErr) && pgErr.Code == "23505" { // unique_violation
			return nil, ErrSlugConflict
		}
		return nil, fmt.Errorf("failed to create project: %w", err)
	}

	return &project, nil
}

// ConfigureSlack configures Slack webhook for a project
func (s *Service) ConfigureSlack(ctx context.Context, projectID uuid.UUID, webhookURL string) error {
	query := `
		UPDATE projects
		SET slack_enabled = TRUE, slack_webhook_url = $2, updated_at = NOW()
		WHERE id = $1
	`

	result, err := s.pool.Exec(ctx, query, projectID, webhookURL)
	if err != nil {
		return fmt.Errorf("failed to configure Slack: %w", err)
	}

	if result.RowsAffected() == 0 {
		return ErrProjectNotFound
	}

	return nil
}

// RemoveSlack removes Slack webhook configuration from a project
func (s *Service) RemoveSlack(ctx context.Context, projectID uuid.UUID) error {
	query := `
		UPDATE projects
		SET slack_enabled = FALSE, slack_webhook_url = NULL, updated_at = NOW()
		WHERE id = $1
	`

	result, err := s.pool.Exec(ctx, query, projectID)
	if err != nil {
		return fmt.Errorf("failed to remove Slack: %w", err)
	}

	if result.RowsAffected() == 0 {
		return ErrProjectNotFound
	}

	return nil
}

// GetSlackWebhookURL retrieves the Slack webhook URL for a project
// This should only be used internally for sending notifications
func (s *Service) GetSlackWebhookURL(ctx context.Context, projectID uuid.UUID) (string, error) {
	var webhookURL sql.NullString

	query := `
		SELECT slack_webhook_url
		FROM projects
		WHERE id = $1 AND slack_enabled = TRUE
	`

	err := s.pool.QueryRow(ctx, query, projectID).Scan(&webhookURL)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return "", ErrProjectNotFound
		}
		return "", fmt.Errorf("failed to get Slack webhook URL: %w", err)
	}

	if !webhookURL.Valid || webhookURL.String == "" {
		return "", nil
	}

	return webhookURL.String, nil
}
