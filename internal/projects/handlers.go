package projects

import (
	"encoding/json"
	"errors"
	"net/http"

	"github.com/aliuyar1234/flakeguard/internal/apperrors"
	"github.com/aliuyar1234/flakeguard/internal/audit"
	"github.com/aliuyar1234/flakeguard/internal/auth"
	"github.com/aliuyar1234/flakeguard/internal/orgs"
	"github.com/aliuyar1234/flakeguard/internal/validation"
	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/rs/zerolog/log"
)

// CreateRequest represents the request to create a project
type CreateRequest struct {
	Name          string `json:"name"`
	Slug          string `json:"slug"`
	DefaultBranch string `json:"default_branch"`
}

type ProjectCreateResponse struct {
	ID            uuid.UUID `json:"id"`
	OrgID         uuid.UUID `json:"org_id"`
	Name          string    `json:"name"`
	Slug          string    `json:"slug"`
	DefaultBranch string    `json:"default_branch"`
	CreatedAt     string    `json:"created_at"`
}

type ProjectListItemResponse struct {
	ID            uuid.UUID `json:"id"`
	Name          string    `json:"name"`
	Slug          string    `json:"slug"`
	DefaultBranch string    `json:"default_branch"`
}

// HandleCreate handles POST /api/v1/orgs/{org_id}/projects
func HandleCreate(pool *pgxpool.Pool, auditor *audit.Writer) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		ctx := r.Context()
		userID := auth.GetUserID(ctx)

		// Get org ID from path
		orgIDStr := chi.URLParam(r, "org_id")
		orgID, err := uuid.Parse(orgIDStr)
		if err != nil {
			apperrors.WriteBadRequest(w, r, "Invalid organization ID")
			return
		}

		// Check if user can mutate org resources (OWNER or ADMIN)
		orgService := orgs.NewService(pool)
		_, err = orgService.RequireOrgMutatePermission(ctx, userID, orgID)
		if err != nil {
			if errors.Is(err, orgs.ErrNotMember) {
				apperrors.WriteNotFound(w, r, "Organization not found")
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
			apperrors.WriteBadRequest(w, r, "Project name is required")
			return
		}
		if req.Slug == "" {
			apperrors.WriteBadRequest(w, r, "Project slug is required")
			return
		}

		// Normalize and validate slug
		req.Slug = validation.NormalizeSlug(req.Slug)
		if err := validation.ValidateSlug(req.Slug); err != nil {
			apperrors.WriteBadRequest(w, r, err.Error())
			return
		}

		// Set default branch if not provided
		if req.DefaultBranch == "" {
			req.DefaultBranch = "main"
		}

		// Create project
		service := NewService(pool)
		project, err := service.Create(ctx, orgID, req.Name, req.Slug, req.DefaultBranch, userID)
		if err != nil {
			if errors.Is(err, ErrSlugConflict) {
				apperrors.WriteConflict(w, r, "Project slug already exists in organization")
				return
			}
			log.Error().Err(err).Msg("Failed to create project")
			apperrors.WriteInternalError(w, r, "Failed to create project")
			return
		}

		// Log audit event
		if err := auditor.LogProjectCreated(ctx, orgID, project.ID, userID, project.Slug); err != nil {
			log.Error().Err(err).Msg("Failed to log audit event")
			// Continue - don't fail the request
		}

		// Return created project
		resp := ProjectCreateResponse{
			ID:            project.ID,
			OrgID:         project.OrgID,
			Name:          project.Name,
			Slug:          project.Slug,
			DefaultBranch: project.DefaultBranch,
			CreatedAt:     project.CreatedAt.Format("2006-01-02T15:04:05Z07:00"),
		}

		apperrors.WriteSuccess(w, r, http.StatusCreated, map[string]any{
			"project": resp,
		})
	}
}

// HandleList handles GET /api/v1/orgs/{org_id}/projects
func HandleList(pool *pgxpool.Pool) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		ctx := r.Context()
		userID := auth.GetUserID(ctx)

		// Get org ID from path
		orgIDStr := chi.URLParam(r, "org_id")
		orgID, err := uuid.Parse(orgIDStr)
		if err != nil {
			apperrors.WriteBadRequest(w, r, "Invalid organization ID")
			return
		}

		// Check if user is a member of the organization
		orgService := orgs.NewService(pool)
		_, err = orgService.RequireOrgMember(ctx, userID, orgID)
		if err != nil {
			if errors.Is(err, orgs.ErrNotMember) {
				apperrors.WriteNotFound(w, r, "Organization not found")
				return
			}
			log.Error().Err(err).Msg("Failed to check org membership")
			apperrors.WriteInternalError(w, r, "Failed to check permissions")
			return
		}

		// Get projects
		service := NewService(pool)
		projects, err := service.ListByOrg(ctx, orgID)
		if err != nil {
			log.Error().Err(err).Msg("Failed to list projects")
			apperrors.WriteInternalError(w, r, "Failed to list projects")
			return
		}

		// Convert to response format
		resp := make([]ProjectListItemResponse, len(projects))
		for i, project := range projects {
			resp[i] = ProjectListItemResponse{
				ID:            project.ID,
				Name:          project.Name,
				Slug:          project.Slug,
				DefaultBranch: project.DefaultBranch,
			}
		}

		apperrors.WriteSuccess(w, r, http.StatusOK, map[string]any{
			"projects": resp,
		})
	}
}

// SlackConfigRequest represents the request to configure Slack
type SlackConfigRequest struct {
	WebhookURL string `json:"webhook_url"`
	Enabled    *bool  `json:"enabled"`
}

// HandleConfigureSlack handles PUT /api/v1/projects/{project_id}/slack
func HandleConfigureSlack(pool *pgxpool.Pool, auditor *audit.Writer) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		ctx := r.Context()
		userID := auth.GetUserID(ctx)

		// Get project ID from path
		projectIDStr := chi.URLParam(r, "project_id")
		projectID, err := uuid.Parse(projectIDStr)
		if err != nil {
			apperrors.WriteBadRequest(w, r, "Invalid project ID")
			return
		}

		// Get project to check org membership
		service := NewService(pool)
		project, err := service.GetByID(ctx, projectID)
		if err != nil {
			if errors.Is(err, ErrProjectNotFound) {
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
		var req SlackConfigRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			apperrors.WriteBadRequest(w, r, "Invalid request body")
			return
		}

		if req.Enabled == nil {
			apperrors.WriteBadRequest(w, r, "enabled is required")
			return
		}

		// Validate webhook URL
		if err := validation.ValidateWebhookURL(req.WebhookURL); err != nil {
			apperrors.WriteBadRequest(w, r, err.Error())
			return
		}

		// Configure Slack
		status, err := service.ConfigureSlack(ctx, projectID, req.WebhookURL, *req.Enabled)
		if err != nil {
			log.Error().Err(err).Msg("Failed to configure Slack")
			if errors.Is(err, ErrProjectNotFound) {
				apperrors.WriteNotFound(w, r, "Project not found")
				return
			}
			apperrors.WriteInternalError(w, r, "Failed to configure Slack")
			return
		}

		// Log audit event
		if err := auditor.LogSlackConfigured(ctx, project.OrgID, projectID, userID); err != nil {
			log.Error().Err(err).Msg("Failed to log audit event")
			// Continue - don't fail the request
		}

		// Return status (without webhook URL)
		apperrors.WriteSuccess(w, r, http.StatusOK, map[string]any{
			"slack": status,
		})
	}
}

// HandleRemoveSlack handles DELETE /api/v1/projects/{project_id}/slack
func HandleRemoveSlack(pool *pgxpool.Pool, auditor *audit.Writer) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		ctx := r.Context()
		userID := auth.GetUserID(ctx)

		// Get project ID from path
		projectIDStr := chi.URLParam(r, "project_id")
		projectID, err := uuid.Parse(projectIDStr)
		if err != nil {
			apperrors.WriteBadRequest(w, r, "Invalid project ID")
			return
		}

		// Get project to check org membership
		service := NewService(pool)
		project, err := service.GetByID(ctx, projectID)
		if err != nil {
			if errors.Is(err, ErrProjectNotFound) {
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

		// Remove Slack configuration
		if err := service.RemoveSlack(ctx, projectID); err != nil {
			log.Error().Err(err).Msg("Failed to remove Slack")
			apperrors.WriteInternalError(w, r, "Failed to remove Slack")
			return
		}

		// Log audit event
		if err := auditor.LogSlackRemoved(ctx, project.OrgID, projectID, userID); err != nil {
			log.Error().Err(err).Msg("Failed to log audit event")
			// Continue - don't fail the request
		}

		// Return status
		apperrors.WriteSuccess(w, r, http.StatusOK, map[string]any{
			"slack": SlackStatus{
				Enabled:       false,
				WebhookURLSet: false,
			},
		})
	}
}
