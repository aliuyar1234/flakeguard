package apikeys

import (
	"database/sql"
	"time"

	"github.com/google/uuid"
)

// ApiKeyScope represents a permission scope for an API key
type ApiKeyScope string

const (
	ScopeIngestWrite ApiKeyScope = "ingest:write"
	ScopeReadProject ApiKeyScope = "read:project"
)

// ApiKey represents an API key for project access
type ApiKey struct {
	ID              uuid.UUID    `db:"id"`
	ProjectID       uuid.UUID    `db:"project_id"`
	Name            string       `db:"name"`
	TokenHash       []byte       `db:"token_hash"`
	Scopes          []string     `db:"scopes"`
	RevokedAt       sql.NullTime `db:"revoked_at"`
	LastUsedAt      sql.NullTime `db:"last_used_at"`
	CreatedByUserID uuid.UUID    `db:"created_by_user_id"`
	CreatedAt       time.Time    `db:"created_at"`
	UpdatedAt       time.Time    `db:"updated_at"`
}

// IsRevoked returns true if the API key has been revoked
func (k *ApiKey) IsRevoked() bool {
	return k.RevokedAt.Valid
}

// IsActive returns true if the API key is active (not revoked)
func (k *ApiKey) IsActive() bool {
	return !k.IsRevoked()
}

type ApiKeyCreatedResponse struct {
	ID        uuid.UUID `json:"id"`
	Name      string    `json:"name"`
	Scopes    []string  `json:"scopes"`
	Token     string    `json:"token"`
	CreatedAt time.Time `json:"created_at"`
}

type ApiKeyListItemResponse struct {
	ID         uuid.UUID  `json:"id"`
	Name       string     `json:"name"`
	Scopes     []string   `json:"scopes"`
	CreatedAt  time.Time  `json:"created_at"`
	RevokedAt  *time.Time `json:"revoked_at"`
	LastUsedAt *time.Time `json:"last_used_at"`
}

func (k *ApiKey) ToCreatedResponse(token string) ApiKeyCreatedResponse {
	return ApiKeyCreatedResponse{
		ID:        k.ID,
		Name:      k.Name,
		Scopes:    append([]string(nil), k.Scopes...),
		Token:     token,
		CreatedAt: k.CreatedAt,
	}
}

func (k *ApiKey) ToListItemResponse() ApiKeyListItemResponse {
	resp := ApiKeyListItemResponse{
		ID:        k.ID,
		Name:      k.Name,
		Scopes:    append([]string(nil), k.Scopes...),
		CreatedAt: k.CreatedAt,
	}
	if k.RevokedAt.Valid {
		resp.RevokedAt = &k.RevokedAt.Time
	}
	if k.LastUsedAt.Valid {
		resp.LastUsedAt = &k.LastUsedAt.Time
	}
	return resp
}
