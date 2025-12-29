package orgs

import (
	"encoding/json"
	"errors"
	"net/http"
	"net/mail"
	"net/url"
	"strings"
	"time"

	"github.com/aliuyar1234/flakeguard/internal/apperrors"
	"github.com/aliuyar1234/flakeguard/internal/audit"
	"github.com/aliuyar1234/flakeguard/internal/auth"
	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/rs/zerolog/log"
)

type InviteCreateRequest struct {
	Email string  `json:"email"`
	Role  OrgRole `json:"role"`
}

type InviteCreateResponse struct {
	ID        uuid.UUID `json:"id"`
	Email     string    `json:"email"`
	Role      OrgRole   `json:"role"`
	ExpiresAt string    `json:"expires_at"`
	Token     string    `json:"token"`
	AcceptURL string    `json:"accept_url"`
}

type InviteAcceptRequest struct {
	Token string `json:"token"`
}

// HandleCreateInvite handles POST /api/v1/orgs/{org_id}/invites
func HandleCreateInvite(pool *pgxpool.Pool, auditor *audit.Writer) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		ctx := r.Context()
		userID := auth.GetUserID(ctx)

		orgIDStr := chi.URLParam(r, "org_id")
		orgID, err := uuid.Parse(orgIDStr)
		if err != nil {
			apperrors.WriteBadRequest(w, r, "Invalid organization ID")
			return
		}

		var req InviteCreateRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			apperrors.WriteBadRequest(w, r, "Invalid request body")
			return
		}

		req.Email = strings.TrimSpace(req.Email)
		if req.Email == "" {
			apperrors.WriteBadRequest(w, r, "Email is required")
			return
		}
		if len(req.Email) > 320 {
			apperrors.WriteBadRequest(w, r, "Email is too long")
			return
		}
		if _, err := mail.ParseAddress(req.Email); err != nil {
			apperrors.WriteBadRequest(w, r, "Invalid email address")
			return
		}

		if req.Role == "" {
			req.Role = RoleMember
		}
		if !req.Role.IsValid() {
			apperrors.WriteBadRequest(w, r, "Invalid role")
			return
		}

		service := NewService(pool)
		invite, token, err := service.CreateInvite(ctx, orgID, userID, req.Email, req.Role)
		if err != nil {
			if errors.Is(err, ErrNotMember) {
				apperrors.WriteNotFound(w, r, "Organization not found")
				return
			}
			if errors.Is(err, ErrInsufficientPermissions) {
				apperrors.WriteForbidden(w, r, "Insufficient permissions")
				return
			}
			if errors.Is(err, ErrInvalidOrgRole) || errors.Is(err, ErrCannotInviteOwner) {
				apperrors.WriteBadRequest(w, r, err.Error())
				return
			}

			log.Error().Err(err).Msg("Failed to create invite")
			apperrors.WriteInternalError(w, r, "Failed to create invite")
			return
		}

		if err := auditor.LogOrgInviteCreated(ctx, orgID, userID, invite.ID, invite.Email, string(invite.Role)); err != nil {
			log.Error().Err(err).Msg("Failed to log audit event")
		}

		acceptURL := "/invites/accept?token=" + url.QueryEscape(token)
		resp := InviteCreateResponse{
			ID:        invite.ID,
			Email:     invite.Email,
			Role:      invite.Role,
			ExpiresAt: invite.ExpiresAt.Format(time.RFC3339),
			Token:     token,
			AcceptURL: acceptURL,
		}

		apperrors.WriteSuccess(w, r, http.StatusCreated, map[string]any{
			"invite": resp,
		})
	}
}

// HandleListInvites handles GET /api/v1/orgs/{org_id}/invites
func HandleListInvites(pool *pgxpool.Pool) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		ctx := r.Context()
		userID := auth.GetUserID(ctx)

		orgIDStr := chi.URLParam(r, "org_id")
		orgID, err := uuid.Parse(orgIDStr)
		if err != nil {
			apperrors.WriteBadRequest(w, r, "Invalid organization ID")
			return
		}

		service := NewService(pool)
		invites, err := service.ListInvites(ctx, orgID, userID)
		if err != nil {
			if errors.Is(err, ErrNotMember) {
				apperrors.WriteNotFound(w, r, "Organization not found")
				return
			}
			if errors.Is(err, ErrInsufficientPermissions) {
				apperrors.WriteForbidden(w, r, "Insufficient permissions")
				return
			}
			log.Error().Err(err).Msg("Failed to list invites")
			apperrors.WriteInternalError(w, r, "Failed to list invites")
			return
		}

		apperrors.WriteSuccess(w, r, http.StatusOK, map[string]any{
			"invites": invites,
		})
	}
}

// HandleRevokeInvite handles DELETE /api/v1/orgs/{org_id}/invites/{invite_id}
func HandleRevokeInvite(pool *pgxpool.Pool, auditor *audit.Writer) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		ctx := r.Context()
		userID := auth.GetUserID(ctx)

		orgIDStr := chi.URLParam(r, "org_id")
		orgID, err := uuid.Parse(orgIDStr)
		if err != nil {
			apperrors.WriteBadRequest(w, r, "Invalid organization ID")
			return
		}

		inviteIDStr := chi.URLParam(r, "invite_id")
		inviteID, err := uuid.Parse(inviteIDStr)
		if err != nil {
			apperrors.WriteBadRequest(w, r, "Invalid invite ID")
			return
		}

		service := NewService(pool)
		if err := service.RevokeInvite(ctx, orgID, inviteID, userID); err != nil {
			if errors.Is(err, ErrNotMember) {
				apperrors.WriteNotFound(w, r, "Organization not found")
				return
			}
			if errors.Is(err, ErrInsufficientPermissions) {
				apperrors.WriteForbidden(w, r, "Insufficient permissions")
				return
			}
			if errors.Is(err, ErrInviteNotFound) {
				apperrors.WriteNotFound(w, r, "Invite not found")
				return
			}
			log.Error().Err(err).Msg("Failed to revoke invite")
			apperrors.WriteInternalError(w, r, "Failed to revoke invite")
			return
		}

		if err := auditor.LogOrgInviteRevoked(ctx, orgID, userID, inviteID); err != nil {
			log.Error().Err(err).Msg("Failed to log audit event")
		}

		apperrors.WriteSuccess(w, r, http.StatusOK, map[string]any{
			"revoked": true,
		})
	}
}

// HandleAcceptInvite handles POST /api/v1/orgs/invites/accept
func HandleAcceptInvite(pool *pgxpool.Pool, auditor *audit.Writer) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		ctx := r.Context()
		userID := auth.GetUserID(ctx)

		var req InviteAcceptRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			apperrors.WriteBadRequest(w, r, "Invalid request body")
			return
		}

		req.Token = strings.TrimSpace(req.Token)
		if req.Token == "" {
			apperrors.WriteBadRequest(w, r, "Invite token is required")
			return
		}

		service := NewService(pool)
		inviteID, orgID, role, err := service.AcceptInvite(ctx, req.Token, userID)
		if err != nil {
			if errors.Is(err, ErrInviteNotFound) {
				apperrors.WriteNotFound(w, r, "Invite not found")
				return
			}
			if errors.Is(err, ErrInviteNotActive) {
				apperrors.WriteConflict(w, r, "Invite already used or revoked")
				return
			}
			if errors.Is(err, ErrInviteExpired) {
				apperrors.WriteError(w, r, http.StatusGone, "gone", "Invite expired")
				return
			}
			if errors.Is(err, ErrInviteEmailMismatch) {
				apperrors.WriteForbidden(w, r, "Invite email does not match your account")
				return
			}

			log.Error().Err(err).Msg("Failed to accept invite")
			apperrors.WriteInternalError(w, r, "Failed to accept invite")
			return
		}

		if err := auditor.LogOrgInviteAccepted(ctx, orgID, userID, inviteID); err != nil {
			log.Error().Err(err).Msg("Failed to log audit event")
		}

		apperrors.WriteSuccess(w, r, http.StatusOK, map[string]any{
			"accepted":  true,
			"invite_id": inviteID,
			"org_id":    orgID,
			"role":      role,
		})
	}
}
