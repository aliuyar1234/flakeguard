package web

import (
	"encoding/json"
	"errors"
	"net/http"
	"strings"

	"github.com/flakeguard/flakeguard/internal/audit"
	"github.com/flakeguard/flakeguard/internal/auth"
	"github.com/flakeguard/flakeguard/internal/orgs"
	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/rs/zerolog/log"
)

type auditEventView struct {
	Action    string
	Actor     string
	CreatedAt string
	Meta      string
}

func HandleOrgSettingsPage(pool *pgxpool.Pool, isProduction bool) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		ctx := r.Context()
		userID := auth.GetUserID(ctx)

		orgIDStr := chi.URLParam(r, "org_id")
		orgID, err := uuid.Parse(orgIDStr)
		if err != nil {
			http.Error(w, "Invalid organization ID", http.StatusBadRequest)
			return
		}

		orgService := orgs.NewService(pool)
		role, err := orgService.RequireOrgMember(ctx, userID, orgID)
		if err != nil {
			if errors.Is(err, orgs.ErrNotMember) {
				http.Error(w, "Organization not found", http.StatusNotFound)
				return
			}
			log.Error().Err(err).Msg("Failed to check org membership")
			http.Error(w, "Internal Server Error", http.StatusInternalServerError)
			return
		}

		org, err := orgService.GetByID(ctx, orgID)
		if err != nil {
			log.Error().Err(err).Msg("Failed to get organization")
			http.Error(w, "Internal Server Error", http.StatusInternalServerError)
			return
		}

		members, err := orgService.ListMembers(ctx, orgID)
		if err != nil {
			log.Error().Err(err).Msg("Failed to list members")
			http.Error(w, "Internal Server Error", http.StatusInternalServerError)
			return
		}

		var invites []orgs.InviteListItem
		var auditEvents []auditEventView
		if role.CanMutate() {
			invites, err = orgService.ListInvites(ctx, orgID, userID)
			if err != nil {
				log.Error().Err(err).Msg("Failed to list invites")
			}

			reader := audit.NewReader(pool)
			events, err := reader.ListByOrg(ctx, orgID, 25)
			if err != nil {
				log.Error().Err(err).Msg("Failed to list audit events")
			} else {
				auditEvents = make([]auditEventView, 0, len(events))
				for _, event := range events {
					actor := strings.TrimSpace(event.ActorEmail)
					if actor == "" {
						actor = "System"
					}

					metaJSON := "{}"
					if b, err := json.MarshalIndent(event.Meta, "", "  "); err == nil && len(b) > 0 {
						metaJSON = string(b)
					}

					auditEvents = append(auditEvents, auditEventView{
						Action:    event.Action,
						Actor:     actor,
						CreatedAt: event.CreatedAt.Format("2006-01-02 15:04"),
						Meta:      metaJSON,
					})
				}
			}
		}

		csrfToken, err := auth.GenerateCSRFToken()
		if err != nil {
			http.Error(w, "Internal Server Error", http.StatusInternalServerError)
			return
		}
		auth.SetCSRFCookie(w, csrfToken, isProduction)

		pageError := strings.TrimSpace(r.URL.Query().Get("error"))

		data := &TemplateData{
			Title:           org.Name + " Settings",
			UserID:          userID,
			IsAuthenticated: true,
			CSRFToken:       csrfToken,
			Error:           pageError,
			Data: map[string]interface{}{
				"OrgID":       orgID,
				"OrgName":     org.Name,
				"OrgSlug":     org.Slug,
				"ActorRole":   string(role),
				"CanMutate":   role.CanMutate(),
				"Members":     members,
				"Invites":     invites,
				"AuditEvents": auditEvents,
			},
		}
		RenderTemplate(w, r, "org_settings.html", data)
	}
}

func HandleInviteAcceptPage(isProduction bool) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		userID := auth.GetUserID(r.Context())

		token := strings.TrimSpace(r.URL.Query().Get("token"))
		pageError := ""
		if token == "" {
			pageError = "Missing invite token"
		}

		csrfToken, err := auth.GenerateCSRFToken()
		if err != nil {
			http.Error(w, "Internal Server Error", http.StatusInternalServerError)
			return
		}
		auth.SetCSRFCookie(w, csrfToken, isProduction)

		data := &TemplateData{
			Title:           "Accept Invitation",
			UserID:          userID,
			IsAuthenticated: true,
			CSRFToken:       csrfToken,
			Error:           pageError,
			Data: map[string]interface{}{
				"Token": token,
			},
		}
		RenderTemplate(w, r, "invite_accept.html", data)
	}
}
