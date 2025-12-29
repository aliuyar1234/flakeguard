package orgs

import (
	"errors"
	"net/http"
	"strconv"
	"strings"

	"github.com/aliuyar1234/flakeguard/internal/apperrors"
	"github.com/aliuyar1234/flakeguard/internal/audit"
	"github.com/aliuyar1234/flakeguard/internal/auth"
	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/rs/zerolog/log"
)

// HandleListAudit handles GET /api/v1/orgs/{org_id}/audit
func HandleListAudit(pool *pgxpool.Pool) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		ctx := r.Context()
		userID := auth.GetUserID(ctx)

		orgIDStr := chi.URLParam(r, "org_id")
		orgID, err := uuid.Parse(orgIDStr)
		if err != nil {
			apperrors.WriteBadRequest(w, r, "Invalid organization ID")
			return
		}

		orgService := NewService(pool)
		if _, err := orgService.RequireOrgMutatePermission(ctx, userID, orgID); err != nil {
			if errors.Is(err, ErrNotMember) {
				apperrors.WriteNotFound(w, r, "Organization not found")
				return
			}
			if errors.Is(err, ErrInsufficientPermissions) {
				apperrors.WriteForbidden(w, r, "Insufficient permissions")
				return
			}
			log.Error().Err(err).Msg("Failed to check org permission")
			apperrors.WriteInternalError(w, r, "Failed to check permissions")
			return
		}

		limit := 50
		if raw := r.URL.Query().Get("limit"); raw != "" {
			if v, err := strconv.Atoi(raw); err == nil {
				limit = v
			}
		}

		reader := audit.NewReader(pool)
		offset := 0
		if raw := r.URL.Query().Get("offset"); raw != "" {
			if v, err := strconv.Atoi(raw); err == nil {
				offset = v
			}
		}

		action := strings.TrimSpace(r.URL.Query().Get("action"))
		actor := strings.TrimSpace(r.URL.Query().Get("actor"))

		var actorUserID *uuid.UUID
		if raw := strings.TrimSpace(r.URL.Query().Get("actor_user_id")); raw != "" {
			if parsed, err := uuid.Parse(raw); err == nil {
				actorUserID = &parsed
			}
		}

		events, total, err := reader.ListByOrgPage(ctx, orgID, audit.ListByOrgOptions{
			Limit:       limit,
			Offset:      offset,
			Action:      action,
			ActorEmail:  actor,
			ActorUserID: actorUserID,
		})
		if err != nil {
			log.Error().Err(err).Msg("Failed to list audit log")
			apperrors.WriteInternalError(w, r, "Failed to list audit log")
			return
		}

		apperrors.WriteSuccess(w, r, http.StatusOK, map[string]any{
			"events":  events,
			"total":   total,
			"limit":   limit,
			"offset":  offset,
			"filters": map[string]any{"action": action, "actor": actor, "actor_user_id": actorUserID},
		})
	}
}
