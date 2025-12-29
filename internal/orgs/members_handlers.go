package orgs

import (
	"encoding/json"
	"errors"
	"net/http"

	"github.com/aliuyar1234/flakeguard/internal/apperrors"
	"github.com/aliuyar1234/flakeguard/internal/audit"
	"github.com/aliuyar1234/flakeguard/internal/auth"
	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/rs/zerolog/log"
)

type MemberRoleUpdateRequest struct {
	Role OrgRole `json:"role"`
}

// HandleUpdateMemberRole handles PUT /api/v1/orgs/{org_id}/members/{user_id}
func HandleUpdateMemberRole(pool *pgxpool.Pool, auditor *audit.Writer) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		ctx := r.Context()
		actorUserID := auth.GetUserID(ctx)

		orgIDStr := chi.URLParam(r, "org_id")
		orgID, err := uuid.Parse(orgIDStr)
		if err != nil {
			apperrors.WriteBadRequest(w, r, "Invalid organization ID")
			return
		}

		targetUserIDStr := chi.URLParam(r, "user_id")
		targetUserID, err := uuid.Parse(targetUserIDStr)
		if err != nil {
			apperrors.WriteBadRequest(w, r, "Invalid user ID")
			return
		}

		var req MemberRoleUpdateRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			apperrors.WriteBadRequest(w, r, "Invalid request body")
			return
		}
		if !req.Role.IsValid() {
			apperrors.WriteBadRequest(w, r, "Invalid role")
			return
		}

		service := NewService(pool)
		prevRole, err := service.UpdateMemberRole(ctx, orgID, actorUserID, targetUserID, req.Role)
		if err != nil {
			if errors.Is(err, ErrNotMember) {
				apperrors.WriteNotFound(w, r, "Organization not found")
				return
			}
			if errors.Is(err, ErrInsufficientPermissions) {
				apperrors.WriteForbidden(w, r, "Insufficient permissions")
				return
			}
			if errors.Is(err, ErrMemberNotFound) {
				apperrors.WriteNotFound(w, r, "Member not found")
				return
			}
			if errors.Is(err, ErrCannotDemoteLastOwner) {
				apperrors.WriteConflict(w, r, "Cannot demote the last owner")
				return
			}
			if errors.Is(err, ErrInvalidOrgRole) {
				apperrors.WriteBadRequest(w, r, "Invalid role")
				return
			}

			log.Error().Err(err).Msg("Failed to update member role")
			apperrors.WriteInternalError(w, r, "Failed to update member role")
			return
		}

		if err := auditor.LogOrgMemberRoleUpdated(ctx, orgID, actorUserID, targetUserID, string(prevRole), string(req.Role)); err != nil {
			log.Error().Err(err).Msg("Failed to log audit event")
		}

		apperrors.WriteSuccess(w, r, http.StatusOK, map[string]any{
			"updated": true,
		})
	}
}

// HandleRemoveMember handles DELETE /api/v1/orgs/{org_id}/members/{user_id}
func HandleRemoveMember(pool *pgxpool.Pool, auditor *audit.Writer) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		ctx := r.Context()
		actorUserID := auth.GetUserID(ctx)

		orgIDStr := chi.URLParam(r, "org_id")
		orgID, err := uuid.Parse(orgIDStr)
		if err != nil {
			apperrors.WriteBadRequest(w, r, "Invalid organization ID")
			return
		}

		targetUserIDStr := chi.URLParam(r, "user_id")
		targetUserID, err := uuid.Parse(targetUserIDStr)
		if err != nil {
			apperrors.WriteBadRequest(w, r, "Invalid user ID")
			return
		}

		service := NewService(pool)
		removedRole, err := service.RemoveMember(ctx, orgID, actorUserID, targetUserID)
		if err != nil {
			if errors.Is(err, ErrNotMember) {
				apperrors.WriteNotFound(w, r, "Organization not found")
				return
			}
			if errors.Is(err, ErrInsufficientPermissions) {
				apperrors.WriteForbidden(w, r, "Insufficient permissions")
				return
			}
			if errors.Is(err, ErrMemberNotFound) {
				apperrors.WriteNotFound(w, r, "Member not found")
				return
			}
			if errors.Is(err, ErrCannotRemoveLastOwner) {
				apperrors.WriteConflict(w, r, "Cannot remove the last owner")
				return
			}

			log.Error().Err(err).Msg("Failed to remove member")
			apperrors.WriteInternalError(w, r, "Failed to remove member")
			return
		}

		if err := auditor.LogOrgMemberRemoved(ctx, orgID, actorUserID, targetUserID, string(removedRole)); err != nil {
			log.Error().Err(err).Msg("Failed to log audit event")
		}

		apperrors.WriteSuccess(w, r, http.StatusOK, map[string]any{
			"removed": true,
		})
	}
}
