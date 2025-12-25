package apikeys

import (
	"encoding/json"
	"errors"
	"net/http"

	"github.com/flakeguard/flakeguard/internal/apperrors"
	"github.com/flakeguard/flakeguard/internal/audit"
	"github.com/flakeguard/flakeguard/internal/auth"
	"github.com/flakeguard/flakeguard/internal/orgs"
	"github.com/flakeguard/flakeguard/internal/projects"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/rs/zerolog/log"
)

// CreateRequest represents the request to create an API key
type CreateRequest struct {
	Name   string        `json:"name"`
	Scopes []ApiKeyScope `json:"scopes,omitempty"`
}

// HandleCreate handles POST /api/v1/projects/{project_id}/api-keys
func HandleCreate(pool *pgxpool.Pool, auditor *audit.Writer) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		ctx := r.Context()
		userID := auth.GetUserID(ctx)

		// Get project ID from path
		projectIDStr := r.PathValue("project_id")
		projectID, err := uuid.Parse(projectIDStr)
		if err != nil {
			apperrors.WriteBadRequest(w, r, "Invalid project ID")
			return
		}

		// Get project to check org membership
		projectService := projects.NewService(pool)
		project, err := projectService.GetByID(ctx, projectID)
		if err != nil {
			if errors.Is(err, projects.ErrProjectNotFound) {
				apperrors.WriteNotFound(w, r, "Project not found")
				return
			}
			log.Error().Err(err).Msg("Failed to get project")
			apperrors.WriteInternalError(w, r, "Failed to get project")
			return
		}

		// Check if user can mutate org resources (OWNER or ADMIN)
		orgService := orgs.NewService(pool)
		_, err = orgService.RequireOrgMutatePermission(ctx, userID, project.OrgID)
		if err != nil {
			if errors.Is(err, orgs.ErrNotMember) {
				apperrors.WriteNotFound(w, r, "Project not found")
				return
			}
			if errors.Is(err, orgs.ErrInsufficientPermissions) {
				apperrors.WriteForbidden(w, r, "Insufficient permissions")
				return
			}
			log.Error().Err(err).Msg("Failed to check org permissions")
			apperrors.WriteInternalError(w, r, "Failed to check permissions")
			return
		}

		// Parse request
		var req CreateRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			apperrors.WriteBadRequest(w, r, "Invalid request body")
			return
		}

		// Validate required fields
		if req.Name == "" {
			apperrors.WriteBadRequest(w, r, "API key name is required")
			return
		}

		// Set default scopes if not provided
		if len(req.Scopes) == 0 {
			req.Scopes = []ApiKeyScope{ScopeIngestWrite}
		}

		// Create API key
		service := NewService(pool)
		key, token, err := service.Create(ctx, projectID, req.Name, req.Scopes, userID)
		if err != nil {
			if errors.Is(err, ErrNameConflict) {
				apperrors.WriteConflict(w, r, "API key name already exists in project")
				return
			}
			log.Error().Err(err).Msg("Failed to create API key")
			apperrors.WriteInternalError(w, r, "Failed to create API key")
			return
		}

		// Log audit event
		if err := auditor.LogAPIKeyCreated(ctx, project.OrgID, projectID, key.ID, userID, key.Name); err != nil {
			log.Error().Err(err).Msg("Failed to log audit event")
			// Continue - don't fail the request
		}

		// Return created API key with plaintext token
		resp := ApiKeyWithToken{
			ApiKeyResponse: key.ToResponse(),
			Token:          token,
		}

		apperrors.WriteSuccess(w, r, http.StatusCreated, resp)
	}
}

// HandleList handles GET /api/v1/projects/{project_id}/api-keys
func HandleList(pool *pgxpool.Pool) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		ctx := r.Context()
		userID := auth.GetUserID(ctx)

		// Get project ID from path
		projectIDStr := r.PathValue("project_id")
		projectID, err := uuid.Parse(projectIDStr)
		if err != nil {
			apperrors.WriteBadRequest(w, r, "Invalid project ID")
			return
		}

		// Get project to check org membership
		projectService := projects.NewService(pool)
		project, err := projectService.GetByID(ctx, projectID)
		if err != nil {
			if errors.Is(err, projects.ErrProjectNotFound) {
				apperrors.WriteNotFound(w, r, "Project not found")
				return
			}
			log.Error().Err(err).Msg("Failed to get project")
			apperrors.WriteInternalError(w, r, "Failed to get project")
			return
		}

		// Check if user is a member of the organization
		orgService := orgs.NewService(pool)
		_, err = orgService.RequireOrgMember(ctx, userID, project.OrgID)
		if err != nil {
			if errors.Is(err, orgs.ErrNotMember) {
				apperrors.WriteNotFound(w, r, "Project not found")
				return
			}
			log.Error().Err(err).Msg("Failed to check org membership")
			apperrors.WriteInternalError(w, r, "Failed to check permissions")
			return
		}

		// Get API keys
		service := NewService(pool)
		keys, err := service.ListByProject(ctx, projectID)
		if err != nil {
			log.Error().Err(err).Msg("Failed to list API keys")
			apperrors.WriteInternalError(w, r, "Failed to list API keys")
			return
		}

		// Convert to response format (without token hashes)
		resp := make([]ApiKeyResponse, len(keys))
		for i, key := range keys {
			resp[i] = key.ToResponse()
		}

		apperrors.WriteSuccess(w, r, http.StatusOK, resp)
	}
}

// HandleRevoke handles DELETE /api/v1/projects/{project_id}/api-keys/{api_key_id}
func HandleRevoke(pool *pgxpool.Pool, auditor *audit.Writer) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		ctx := r.Context()
		userID := auth.GetUserID(ctx)

		// Get IDs from path
		projectIDStr := r.PathValue("project_id")
		apiKeyIDStr := r.PathValue("api_key_id")

		projectID, err := uuid.Parse(projectIDStr)
		if err != nil {
			apperrors.WriteBadRequest(w, r, "Invalid project ID")
			return
		}

		apiKeyID, err := uuid.Parse(apiKeyIDStr)
		if err != nil {
			apperrors.WriteBadRequest(w, r, "Invalid API key ID")
			return
		}

		// Get project to check org membership
		projectService := projects.NewService(pool)
		project, err := projectService.GetByID(ctx, projectID)
		if err != nil {
			if errors.Is(err, projects.ErrProjectNotFound) {
				apperrors.WriteNotFound(w, r, "Project not found")
				return
			}
			log.Error().Err(err).Msg("Failed to get project")
			apperrors.WriteInternalError(w, r, "Failed to get project")
			return
		}

		// Check if user can mutate org resources (OWNER or ADMIN)
		orgService := orgs.NewService(pool)
		_, err = orgService.RequireOrgMutatePermission(ctx, userID, project.OrgID)
		if err != nil {
			if errors.Is(err, orgs.ErrNotMember) {
				apperrors.WriteNotFound(w, r, "Project not found")
				return
			}
			if errors.Is(err, orgs.ErrInsufficientPermissions) {
				apperrors.WriteForbidden(w, r, "Insufficient permissions")
				return
			}
			log.Error().Err(err).Msg("Failed to check org permissions")
			apperrors.WriteInternalError(w, r, "Failed to check permissions")
			return
		}

		// Get API key to verify it belongs to this project
		service := NewService(pool)
		key, err := service.GetByID(ctx, apiKeyID)
		if err != nil {
			if errors.Is(err, ErrAPIKeyNotFound) {
				apperrors.WriteNotFound(w, r, "API key not found")
				return
			}
			log.Error().Err(err).Msg("Failed to get API key")
			apperrors.WriteInternalError(w, r, "Failed to get API key")
			return
		}

		// Verify API key belongs to this project
		if key.ProjectID != projectID {
			apperrors.WriteNotFound(w, r, "API key not found")
			return
		}

		// Revoke API key
		if err := service.Revoke(ctx, apiKeyID); err != nil {
			if errors.Is(err, ErrAPIKeyNotFound) {
				apperrors.WriteNotFound(w, r, "API key already revoked or not found")
				return
			}
			log.Error().Err(err).Msg("Failed to revoke API key")
			apperrors.WriteInternalError(w, r, "Failed to revoke API key")
			return
		}

		// Log audit event
		if err := auditor.LogAPIKeyRevoked(ctx, project.OrgID, projectID, apiKeyID, userID, key.Name); err != nil {
			log.Error().Err(err).Msg("Failed to log audit event")
			// Continue - don't fail the request
		}

		apperrors.WriteSuccess(w, r, http.StatusOK, map[string]string{
			"message": "API key revoked successfully",
		})
	}
}
