package apikeys

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"
)

var (
	// ErrAPIKeyNotFound is returned when an API key is not found
	ErrAPIKeyNotFound = errors.New("API key not found")

	// ErrNameConflict is returned when an API key name already exists in the project
	ErrNameConflict = errors.New("API key name already exists in project")

	// ErrAPIKeyRevoked is returned when attempting an operation on a revoked key.
	ErrAPIKeyRevoked = errors.New("API key is revoked")
)

// Service provides API key-related operations
type Service struct {
	pool *pgxpool.Pool
}

// NewService creates a new API key service
func NewService(pool *pgxpool.Pool) *Service {
	return &Service{pool: pool}
}

// GetByID retrieves an API key by ID
func (s *Service) GetByID(ctx context.Context, apiKeyID uuid.UUID) (*ApiKey, error) {
	var key ApiKey

	query := `
			SELECT id, project_id, name, token_hash, scopes::text[], expires_at, revoked_at, last_used_at,
			       created_by_user_id, created_at, updated_at
			FROM api_keys
			WHERE id = $1
	`

	err := s.pool.QueryRow(ctx, query, apiKeyID).Scan(
		&key.ID,
		&key.ProjectID,
		&key.Name,
		&key.TokenHash,
		&key.Scopes,
		&key.ExpiresAt,
		&key.RevokedAt,
		&key.LastUsedAt,
		&key.CreatedByUserID,
		&key.CreatedAt,
		&key.UpdatedAt,
	)

	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, ErrAPIKeyNotFound
		}
		return nil, fmt.Errorf("failed to get API key: %w", err)
	}

	return &key, nil
}

// ListByProject retrieves all API keys for a project
func (s *Service) ListByProject(ctx context.Context, projectID uuid.UUID) ([]ApiKey, error) {
	query := `
		SELECT id, project_id, name, token_hash, scopes::text[], expires_at, revoked_at, last_used_at,
		       created_by_user_id, created_at, updated_at
		FROM api_keys
		WHERE project_id = $1
		ORDER BY created_at DESC
	`

	rows, err := s.pool.Query(ctx, query, projectID)
	if err != nil {
		return nil, fmt.Errorf("failed to list API keys: %w", err)
	}
	defer rows.Close()

	var keys []ApiKey
	for rows.Next() {
		var key ApiKey
		err := rows.Scan(
			&key.ID,
			&key.ProjectID,
			&key.Name,
			&key.TokenHash,
			&key.Scopes,
			&key.ExpiresAt,
			&key.RevokedAt,
			&key.LastUsedAt,
			&key.CreatedByUserID,
			&key.CreatedAt,
			&key.UpdatedAt,
		)
		if err != nil {
			return nil, fmt.Errorf("failed to scan API key: %w", err)
		}
		keys = append(keys, key)
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("error iterating API key rows: %w", err)
	}

	return keys, nil
}

// Create creates a new API key and returns it with the plaintext token
func (s *Service) Create(ctx context.Context, projectID uuid.UUID, name string, scopes []ApiKeyScope, userID uuid.UUID, expiresAt *time.Time) (*ApiKey, string, error) {
	// Generate token
	token, tokenHash, err := GenerateToken()
	if err != nil {
		return nil, "", fmt.Errorf("failed to generate token: %w", err)
	}

	// Convert scopes to string array
	scopeStrs := make([]string, len(scopes))
	for i, scope := range scopes {
		scopeStrs[i] = string(scope)
	}

	// Insert API key
	var key ApiKey
	query := `
		INSERT INTO api_keys (project_id, name, token_hash, scopes, created_by_user_id, expires_at)
		VALUES ($1, $2, $3, $4::api_key_scope[], $5, $6)
		RETURNING id, project_id, name, token_hash, scopes::text[], expires_at, revoked_at, last_used_at,
		          created_by_user_id, created_at, updated_at
	`

	err = s.pool.QueryRow(ctx, query, projectID, name, tokenHash, scopeStrs, userID, expiresAt).Scan(
		&key.ID,
		&key.ProjectID,
		&key.Name,
		&key.TokenHash,
		&key.Scopes,
		&key.ExpiresAt,
		&key.RevokedAt,
		&key.LastUsedAt,
		&key.CreatedByUserID,
		&key.CreatedAt,
		&key.UpdatedAt,
	)

	if err != nil {
		var pgErr *pgconn.PgError
		if errors.As(err, &pgErr) && pgErr.Code == "23505" { // unique_violation
			return nil, "", ErrNameConflict
		}
		return nil, "", fmt.Errorf("failed to create API key: %w", err)
	}

	return &key, token, nil
}

// Rotate atomically creates a new API key and revokes the old key.
// The new key inherits the old key scopes.
func (s *Service) Rotate(ctx context.Context, apiKeyID uuid.UUID, newName string, userID uuid.UUID, expiresAt *time.Time) (newKey *ApiKey, token string, oldKey *ApiKey, err error) {
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return nil, "", nil, fmt.Errorf("failed to begin transaction: %w", err)
	}
	defer func() {
		_ = tx.Rollback(ctx)
	}()

	var old ApiKey
	err = tx.QueryRow(ctx, `
		SELECT id, project_id, name, scopes::text[], expires_at, revoked_at
		FROM api_keys
		WHERE id = $1
		FOR UPDATE
	`, apiKeyID).Scan(&old.ID, &old.ProjectID, &old.Name, &old.Scopes, &old.ExpiresAt, &old.RevokedAt)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, "", nil, ErrAPIKeyNotFound
		}
		return nil, "", nil, fmt.Errorf("failed to load API key: %w", err)
	}
	if old.RevokedAt.Valid {
		return nil, "", nil, ErrAPIKeyRevoked
	}

	// Generate token for the new key
	token, tokenHash, err := GenerateToken()
	if err != nil {
		return nil, "", nil, fmt.Errorf("failed to generate token: %w", err)
	}

	// Insert new API key (inherits scopes from old key)
	var created ApiKey
	err = tx.QueryRow(ctx, `
		INSERT INTO api_keys (project_id, name, token_hash, scopes, created_by_user_id, expires_at)
		VALUES ($1, $2, $3, $4::api_key_scope[], $5, $6)
		RETURNING id, project_id, name, token_hash, scopes::text[], expires_at, revoked_at, last_used_at,
		          created_by_user_id, created_at, updated_at
	`, old.ProjectID, newName, tokenHash, old.Scopes, userID, expiresAt).Scan(
		&created.ID,
		&created.ProjectID,
		&created.Name,
		&created.TokenHash,
		&created.Scopes,
		&created.ExpiresAt,
		&created.RevokedAt,
		&created.LastUsedAt,
		&created.CreatedByUserID,
		&created.CreatedAt,
		&created.UpdatedAt,
	)
	if err != nil {
		var pgErr *pgconn.PgError
		if errors.As(err, &pgErr) && pgErr.Code == "23505" {
			return nil, "", nil, ErrNameConflict
		}
		return nil, "", nil, fmt.Errorf("failed to create rotated API key: %w", err)
	}

	// Revoke old API key
	tag, err := tx.Exec(ctx, `
		UPDATE api_keys
		SET revoked_at = NOW(), updated_at = NOW()
		WHERE id = $1 AND revoked_at IS NULL
	`, apiKeyID)
	if err != nil {
		return nil, "", nil, fmt.Errorf("failed to revoke old API key: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return nil, "", nil, ErrAPIKeyRevoked
	}

	if err := tx.Commit(ctx); err != nil {
		return nil, "", nil, fmt.Errorf("failed to commit transaction: %w", err)
	}

	return &created, token, &old, nil
}

// Revoke marks an API key as revoked
func (s *Service) Revoke(ctx context.Context, apiKeyID uuid.UUID) error {
	query := `
		UPDATE api_keys
		SET revoked_at = $2, updated_at = NOW()
		WHERE id = $1 AND revoked_at IS NULL
	`

	result, err := s.pool.Exec(ctx, query, apiKeyID, time.Now())
	if err != nil {
		return fmt.Errorf("failed to revoke API key: %w", err)
	}

	if result.RowsAffected() == 0 {
		return ErrAPIKeyNotFound
	}

	return nil
}

// GetByTokenHash retrieves an API key by its token hash
// This is used for authentication
func (s *Service) GetByTokenHash(ctx context.Context, tokenHash []byte) (*ApiKey, error) {
	var key ApiKey

	query := `
			SELECT id, project_id, name, token_hash, scopes::text[], expires_at, revoked_at, last_used_at,
			       created_by_user_id, created_at, updated_at
			FROM api_keys
			WHERE token_hash = $1
	`

	err := s.pool.QueryRow(ctx, query, tokenHash).Scan(
		&key.ID,
		&key.ProjectID,
		&key.Name,
		&key.TokenHash,
		&key.Scopes,
		&key.ExpiresAt,
		&key.RevokedAt,
		&key.LastUsedAt,
		&key.CreatedByUserID,
		&key.CreatedAt,
		&key.UpdatedAt,
	)

	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, ErrAPIKeyNotFound
		}
		return nil, fmt.Errorf("failed to get API key by token: %w", err)
	}

	return &key, nil
}

// UpdateLastUsed updates the last_used_at timestamp for an API key
func (s *Service) UpdateLastUsed(ctx context.Context, apiKeyID uuid.UUID) error {
	query := `
		UPDATE api_keys
		SET last_used_at = NOW(), updated_at = NOW()
		WHERE id = $1
	`

	_, err := s.pool.Exec(ctx, query, apiKeyID)
	if err != nil {
		return fmt.Errorf("failed to update last used: %w", err)
	}

	return nil
}
