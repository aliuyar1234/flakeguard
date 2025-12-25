package apikeys

import (
	"database/sql"
	"time"

	"github.com/google/uuid"
	"github.com/lib/pq"
)

// ApiKeyScope represents a permission scope for an API key
type ApiKeyScope string

const (
	ScopeIngestWrite ApiKeyScope = "ingest:write"
	ScopeReadProject ApiKeyScope = "read:project"
)

// ApiKey represents an API key for project access
type ApiKey struct {
	ID              uuid.UUID      `db:"id"`
	ProjectID       uuid.UUID      `db:"project_id"`
	Name            string         `db:"name"`
	TokenHash       []byte         `db:"token_hash"`
	Scopes          pq.StringArray `db:"scopes"`
	RevokedAt       sql.NullTime   `db:"revoked_at"`
	LastUsedAt      sql.NullTime   `db:"last_used_at"`
	CreatedByUserID uuid.UUID      `db:"created_by_user_id"`
	CreatedAt       time.Time      `db:"created_at"`
	UpdatedAt       time.Time      `db:"updated_at"`
}

// IsRevoked returns true if the API key has been revoked
func (k *ApiKey) IsRevoked() bool {
	return k.RevokedAt.Valid
}

// IsActive returns true if the API key is active (not revoked)
func (k *ApiKey) IsActive() bool {
	return !k.IsRevoked()
}

// ApiKeyResponse represents an API key in API responses (without token hash)
type ApiKeyResponse struct {
	ID        uuid.UUID  `json:"id"`
	ProjectID uuid.UUID  `json:"project_id"`
	Name      string     `json:"name"`
	Scopes    []string   `json:"scopes"`
	RevokedAt *time.Time `json:"revoked_at,omitempty"`
	CreatedAt time.Time  `json:"created_at"`
}

// ApiKeyWithToken represents a newly created API key with the plaintext token
// This is only returned once when the key is created
type ApiKeyWithToken struct {
	ApiKeyResponse
	Token string `json:"token"`
}

// ToResponse converts an ApiKey to an ApiKeyResponse (without token)
func (k *ApiKey) ToResponse() ApiKeyResponse {
	resp := ApiKeyResponse{
		ID:        k.ID,
		ProjectID: k.ProjectID,
		Name:      k.Name,
		Scopes:    []string(k.Scopes),
		CreatedAt: k.CreatedAt,
	}
	if k.RevokedAt.Valid {
		resp.RevokedAt = &k.RevokedAt.Time
	}
	return resp
}
