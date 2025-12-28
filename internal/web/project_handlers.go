package web

import (
	"errors"
	"net/http"

	"github.com/flakeguard/flakeguard/internal/apikeys"
	"github.com/flakeguard/flakeguard/internal/auth"
	"github.com/flakeguard/flakeguard/internal/orgs"
	"github.com/flakeguard/flakeguard/internal/projects"
	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/rs/zerolog/log"
)

// HandleProjectsPage renders the projects list page for an organization.
func HandleProjectsPage(pool *pgxpool.Pool, isProduction bool) http.HandlerFunc {
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

		projectService := projects.NewService(pool)
		projectsList, err := projectService.ListByOrg(ctx, orgID)
		if err != nil {
			log.Error().Err(err).Msg("Failed to list projects")
			http.Error(w, "Internal Server Error", http.StatusInternalServerError)
			return
		}

		csrfToken, err := auth.GenerateCSRFToken()
		if err != nil {
			http.Error(w, "Internal Server Error", http.StatusInternalServerError)
			return
		}
		auth.SetCSRFCookie(w, csrfToken, isProduction)

		data := &TemplateData{
			Title:           org.Name + " Projects",
			UserID:          userID,
			IsAuthenticated: true,
			CSRFToken:       csrfToken,
			Data: map[string]interface{}{
				"OrgID":     orgID,
				"OrgName":   org.Name,
				"Projects":  projectsList,
				"CanMutate": role.CanMutate(),
			},
		}
		RenderTemplate(w, r, "project_list.html", data)
	}
}

// HandleProjectCreatePage renders the project creation page.
func HandleProjectCreatePage(pool *pgxpool.Pool, isProduction bool) http.HandlerFunc {
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
		_, err = orgService.RequireOrgMutatePermission(ctx, userID, orgID)
		if err != nil {
			if errors.Is(err, orgs.ErrNotMember) {
				http.Error(w, "Organization not found", http.StatusNotFound)
				return
			}
			if errors.Is(err, orgs.ErrInsufficientPermissions) {
				http.Error(w, "Insufficient permissions", http.StatusForbidden)
				return
			}
			log.Error().Err(err).Msg("Failed to check org permissions")
			http.Error(w, "Internal Server Error", http.StatusInternalServerError)
			return
		}

		org, err := orgService.GetByID(ctx, orgID)
		if err != nil {
			log.Error().Err(err).Msg("Failed to get organization")
			http.Error(w, "Internal Server Error", http.StatusInternalServerError)
			return
		}

		csrfToken, err := auth.GenerateCSRFToken()
		if err != nil {
			http.Error(w, "Internal Server Error", http.StatusInternalServerError)
			return
		}
		auth.SetCSRFCookie(w, csrfToken, isProduction)

		data := &TemplateData{
			Title:           "Create Project",
			UserID:          userID,
			IsAuthenticated: true,
			CSRFToken:       csrfToken,
			Data: map[string]interface{}{
				"OrgID":   orgID,
				"OrgName": org.Name,
			},
		}
		RenderTemplate(w, r, "project_create.html", data)
	}
}

// HandleProjectSettingsPage renders the project settings page.
func HandleProjectSettingsPage(pool *pgxpool.Pool, isProduction bool) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		ctx := r.Context()
		userID := auth.GetUserID(ctx)

		orgIDStr := chi.URLParam(r, "org_id")
		projectIDStr := chi.URLParam(r, "project_id")

		orgID, err := uuid.Parse(orgIDStr)
		if err != nil {
			http.Error(w, "Invalid organization ID", http.StatusBadRequest)
			return
		}

		projectID, err := uuid.Parse(projectIDStr)
		if err != nil {
			http.Error(w, "Invalid project ID", http.StatusBadRequest)
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

		projectService := projects.NewService(pool)
		project, err := projectService.GetByID(ctx, projectID)
		if err != nil {
			if errors.Is(err, projects.ErrProjectNotFound) {
				http.Error(w, "Project not found", http.StatusNotFound)
				return
			}
			log.Error().Err(err).Msg("Failed to get project")
			http.Error(w, "Internal Server Error", http.StatusInternalServerError)
			return
		}

		if project.OrgID != orgID {
			http.Error(w, "Project not found", http.StatusNotFound)
			return
		}

		org, err := orgService.GetByID(ctx, orgID)
		if err != nil {
			log.Error().Err(err).Msg("Failed to get organization")
			http.Error(w, "Internal Server Error", http.StatusInternalServerError)
			return
		}

		csrfToken, err := auth.GenerateCSRFToken()
		if err != nil {
			http.Error(w, "Internal Server Error", http.StatusInternalServerError)
			return
		}
		auth.SetCSRFCookie(w, csrfToken, isProduction)

		pageError := ""
		apiKeyService := apikeys.NewService(pool)
		keys, err := apiKeyService.ListByProject(ctx, projectID)
		if err != nil {
			log.Error().Err(err).Str("project_id", projectID.String()).Msg("Failed to list API keys for project settings page")
			pageError = "Failed to load API keys"
		}

		apiKeyItems := make([]apikeys.ApiKeyListItemResponse, 0, len(keys))
		for _, key := range keys {
			apiKeyItems = append(apiKeyItems, key.ToListItemResponse())
		}

		slackWebhookURLSet := project.SlackWebhookURL.Valid && project.SlackWebhookURL.String != ""

		data := &TemplateData{
			Title:           project.Name + " Settings",
			UserID:          userID,
			IsAuthenticated: true,
			CSRFToken:       csrfToken,
			Error:           pageError,
			Data: map[string]interface{}{
				"OrgID":              orgID,
				"OrgSlug":            org.Slug,
				"ProjectID":          projectID,
				"ProjectName":        project.Name,
				"ProjectSlug":        project.Slug,
				"DefaultBranch":      project.DefaultBranch,
				"SlackEnabled":       project.SlackEnabled,
				"SlackWebhookURLSet": slackWebhookURLSet,
				"APIKeys":            apiKeyItems,
				"CanMutate":          role.CanMutate(),
			},
		}
		RenderTemplate(w, r, "project_settings.html", data)
	}
}
