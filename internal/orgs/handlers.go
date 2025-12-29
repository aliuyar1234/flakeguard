package orgs

import (
	"encoding/json"
	"errors"
	"net/http"

	"github.com/aliuyar1234/flakeguard/internal/apperrors"
	"github.com/aliuyar1234/flakeguard/internal/audit"
	"github.com/aliuyar1234/flakeguard/internal/auth"
	"github.com/aliuyar1234/flakeguard/internal/validation"
	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/rs/zerolog/log"
)

// CreateRequest represents the request to create an organization
type CreateRequest struct {
	Name string `json:"name"`
	Slug string `json:"slug"`
}

type OrgCreateResponse struct {
	ID        uuid.UUID `json:"id"`
	Name      string    `json:"name"`
	Slug      string    `json:"slug"`
	CreatedAt string    `json:"created_at"`
}

type OrgListItemResponse struct {
	ID   uuid.UUID `json:"id"`
	Name string    `json:"name"`
	Slug string    `json:"slug"`
	Role OrgRole   `json:"role"`
}

// HandleCreate handles POST /api/v1/orgs
func HandleCreate(pool *pgxpool.Pool, auditor *audit.Writer) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		ctx := r.Context()
		userID := auth.GetUserID(ctx)

		// Parse request
		var req CreateRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			apperrors.WriteBadRequest(w, r, "Invalid request body")
			return
		}

		// Validate required fields
		if req.Name == "" {
			apperrors.WriteBadRequest(w, r, "Organization name is required")
			return
		}
		if req.Slug == "" {
			apperrors.WriteBadRequest(w, r, "Organization slug is required")
			return
		}

		// Normalize and validate slug
		req.Slug = validation.NormalizeSlug(req.Slug)
		if err := validation.ValidateSlug(req.Slug); err != nil {
			apperrors.WriteBadRequest(w, r, err.Error())
			return
		}

		// Create organization
		service := NewService(pool)
		org, err := service.CreateWithOwner(ctx, req.Name, req.Slug, userID)
		if err != nil {
			if errors.Is(err, ErrSlugConflict) {
				apperrors.WriteConflict(w, r, "Organization slug already exists")
				return
			}
			log.Error().Err(err).Msg("Failed to create organization")
			apperrors.WriteInternalError(w, r, "Failed to create organization")
			return
		}

		// Log audit event
		if err := auditor.LogOrgCreated(ctx, org.ID, userID, org.Slug); err != nil {
			log.Error().Err(err).Msg("Failed to log audit event")
			// Continue - don't fail the request
		}

		// Return created organization
		resp := OrgCreateResponse{
			ID:        org.ID,
			Name:      org.Name,
			Slug:      org.Slug,
			CreatedAt: org.CreatedAt.Format("2006-01-02T15:04:05Z07:00"),
		}

		apperrors.WriteSuccess(w, r, http.StatusCreated, map[string]any{
			"org": resp,
		})
	}
}

// HandleList handles GET /api/v1/orgs
func HandleList(pool *pgxpool.Pool) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		ctx := r.Context()
		userID := auth.GetUserID(ctx)

		// Get user's organizations
		service := NewService(pool)
		orgs, err := service.ListUserOrgs(ctx, userID)
		if err != nil {
			log.Error().Err(err).Msg("Failed to list organizations")
			apperrors.WriteInternalError(w, r, "Failed to list organizations")
			return
		}

		// Convert to response format
		resp := make([]OrgListItemResponse, len(orgs))
		for i, org := range orgs {
			resp[i] = OrgListItemResponse{
				ID:   org.ID,
				Name: org.Name,
				Slug: org.Slug,
				Role: org.Role,
			}
		}

		apperrors.WriteSuccess(w, r, http.StatusOK, map[string]any{
			"orgs": resp,
		})
	}
}

// HandleListMembers handles GET /api/v1/orgs/{org_id}/members
func HandleListMembers(pool *pgxpool.Pool) http.HandlerFunc {
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
		orgService := NewService(pool)
		_, err = orgService.RequireOrgMember(ctx, userID, orgID)
		if err != nil {
			if errors.Is(err, ErrNotMember) {
				apperrors.WriteNotFound(w, r, "Organization not found")
				return
			}
			log.Error().Err(err).Msg("Failed to check org membership")
			apperrors.WriteInternalError(w, r, "Failed to check permissions")
			return
		}

		// Get organization members
		service := NewService(pool)
		members, err := service.ListMembers(ctx, orgID)
		if err != nil {
			log.Error().Err(err).Msg("Failed to list members")
			apperrors.WriteInternalError(w, r, "Failed to list members")
			return
		}

		apperrors.WriteSuccess(w, r, http.StatusOK, map[string]any{
			"members": members,
		})
	}
}
