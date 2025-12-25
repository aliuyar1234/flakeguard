package web

import (
	"errors"
	"net/http"
	"strconv"

	"github.com/flakeguard/flakeguard/internal/auth"
	"github.com/flakeguard/flakeguard/internal/flake"
	"github.com/flakeguard/flakeguard/internal/orgs"
	"github.com/flakeguard/flakeguard/internal/projects"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/rs/zerolog/log"
)

// HandleFlakesListPage renders the flakes list page for a project
func HandleFlakesListPage(pool *pgxpool.Pool, isProduction bool) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		ctx := r.Context()
		userID := auth.GetUserID(ctx)

		// Get org_slug and project_slug from path
		orgSlug := r.PathValue("org_slug")
		projectSlug := r.PathValue("project_slug")

		// Get organization by slug
		orgService := orgs.NewService(pool)
		org, err := orgService.GetBySlug(ctx, orgSlug)
		if err != nil {
			if errors.Is(err, orgs.ErrOrgNotFound) {
				http.Error(w, "Organization not found", http.StatusNotFound)
				return
			}
			log.Error().Err(err).Str("org_slug", orgSlug).Msg("Failed to get organization")
			http.Error(w, "Internal Server Error", http.StatusInternalServerError)
			return
		}

		// Verify user has access to org
		_, err = orgService.RequireOrgMember(ctx, userID, org.ID)
		if err != nil {
			if errors.Is(err, orgs.ErrNotMember) {
				http.Error(w, "Forbidden", http.StatusForbidden)
				return
			}
			log.Error().Err(err).Msg("Failed to check org membership")
			http.Error(w, "Internal Server Error", http.StatusInternalServerError)
			return
		}

		// Get project by slug
		projectService := projects.NewService(pool)
		project, err := projectService.GetByOrgAndSlug(ctx, org.ID, projectSlug)
		if err != nil {
			if errors.Is(err, projects.ErrProjectNotFound) {
				http.Error(w, "Project not found", http.StatusNotFound)
				return
			}
			log.Error().Err(err).Str("project_slug", projectSlug).Msg("Failed to get project")
			http.Error(w, "Internal Server Error", http.StatusInternalServerError)
			return
		}

		// Parse filter parameters
		days := 30 // Default to 30 days
		if daysStr := r.URL.Query().Get("days"); daysStr != "" {
			if parsed, err := strconv.Atoi(daysStr); err == nil && parsed > 0 {
				days = parsed
			}
		}

		repo := r.URL.Query().Get("repo")
		jobName := r.URL.Query().Get("job_name")

		// Build request for service
		req := flake.ListFlakesRequest{
			Days:    days,
			Repo:    repo,
			JobName: jobName,
			Limit:   100, // MVP limit
			Offset:  0,
		}

		// Query flake stats
		flakeService := flake.NewService(pool)
		flakes, total, err := flakeService.ListFlakes(ctx, project.ID, req)
		if err != nil {
			log.Error().Err(err).Str("project_id", project.ID.String()).Msg("Failed to list flakes")
			http.Error(w, "Internal Server Error", http.StatusInternalServerError)
			return
		}

		// Generate CSRF token
		csrfToken, err := auth.GenerateCSRFToken()
		if err != nil {
			http.Error(w, "Internal Server Error", http.StatusInternalServerError)
			return
		}

		// Set CSRF cookie
		auth.SetCSRFCookie(w, csrfToken, isProduction)

		// Render flakes list page
		data := &TemplateData{
			Title:     "Flaky Tests - " + project.Name,
			UserID:    userID,
			CSRFToken: csrfToken,
			Data: map[string]interface{}{
				"OrgSlug":     orgSlug,
				"ProjectSlug": projectSlug,
				"ProjectName": project.Name,
				"Flakes":      flakes,
				"Total":       total,
				"Days":        days,
				"Repo":        repo,
				"JobName":     jobName,
			},
		}
		RenderTemplate(w, r, "flakes_list.html", data)
	}
}

// HandleFlakeDetailPage renders the flake detail page
func HandleFlakeDetailPage(pool *pgxpool.Pool, isProduction bool) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		ctx := r.Context()
		userID := auth.GetUserID(ctx)

		// Get org_slug, project_slug, and test_case_id from path
		orgSlug := r.PathValue("org_slug")
		projectSlug := r.PathValue("project_slug")
		testCaseIDStr := r.PathValue("test_case_id")

		// Parse test_case_id
		testCaseID, err := uuid.Parse(testCaseIDStr)
		if err != nil {
			http.Error(w, "Invalid test case ID", http.StatusBadRequest)
			return
		}

		// Get organization by slug
		orgService := orgs.NewService(pool)
		org, err := orgService.GetBySlug(ctx, orgSlug)
		if err != nil {
			if errors.Is(err, orgs.ErrOrgNotFound) {
				http.Error(w, "Organization not found", http.StatusNotFound)
				return
			}
			log.Error().Err(err).Str("org_slug", orgSlug).Msg("Failed to get organization")
			http.Error(w, "Internal Server Error", http.StatusInternalServerError)
			return
		}

		// Verify user has access to org
		_, err = orgService.RequireOrgMember(ctx, userID, org.ID)
		if err != nil {
			if errors.Is(err, orgs.ErrNotMember) {
				http.Error(w, "Forbidden", http.StatusForbidden)
				return
			}
			log.Error().Err(err).Msg("Failed to check org membership")
			http.Error(w, "Internal Server Error", http.StatusInternalServerError)
			return
		}

		// Get project by slug
		projectService := projects.NewService(pool)
		project, err := projectService.GetByOrgAndSlug(ctx, org.ID, projectSlug)
		if err != nil {
			if errors.Is(err, projects.ErrProjectNotFound) {
				http.Error(w, "Project not found", http.StatusNotFound)
				return
			}
			log.Error().Err(err).Str("project_slug", projectSlug).Msg("Failed to get project")
			http.Error(w, "Internal Server Error", http.StatusInternalServerError)
			return
		}

		// Query flake detail
		flakeService := flake.NewService(pool)
		detail, evidenceTotal, err := flakeService.GetFlakeDetail(ctx, project.ID, testCaseID, 100, 0)
		if err != nil {
			if err.Error() == "flake not found" {
				http.Error(w, "Flake not found", http.StatusNotFound)
				return
			}
			log.Error().Err(err).
				Str("project_id", project.ID.String()).
				Str("test_case_id", testCaseID.String()).
				Msg("Failed to get flake detail")
			http.Error(w, "Internal Server Error", http.StatusInternalServerError)
			return
		}

		// Generate CSRF token
		csrfToken, err := auth.GenerateCSRFToken()
		if err != nil {
			http.Error(w, "Internal Server Error", http.StatusInternalServerError)
			return
		}

		// Set CSRF cookie
		auth.SetCSRFCookie(w, csrfToken, isProduction)

		// Render flake detail page
		data := &TemplateData{
			Title:     "Flake Detail - " + detail.TestIdentifier,
			UserID:    userID,
			CSRFToken: csrfToken,
			Data: map[string]interface{}{
				"OrgSlug":       orgSlug,
				"ProjectSlug":   projectSlug,
				"ProjectName":   project.Name,
				"Detail":        detail,
				"EvidenceTotal": evidenceTotal,
			},
		}
		RenderTemplate(w, r, "flake_detail.html", data)
	}
}
