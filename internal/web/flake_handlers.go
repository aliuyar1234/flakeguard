package web

import (
	"errors"
	"net/http"
	"strconv"
	"strings"

	"github.com/flakeguard/flakeguard/internal/auth"
	"github.com/flakeguard/flakeguard/internal/flake"
	"github.com/flakeguard/flakeguard/internal/orgs"
	"github.com/flakeguard/flakeguard/internal/projects"
	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/rs/zerolog/log"
)

func truncateRunes(s string, maxRunes int) (string, bool) {
	runes := []rune(s)
	if len(runes) <= maxRunes {
		return s, false
	}
	return string(runes[:maxRunes]) + "...", true
}

// HandleFlakesListPage renders the flakes list page for a project.
func HandleFlakesListPage(pool *pgxpool.Pool, isProduction bool) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		ctx := r.Context()
		userID := auth.GetUserID(ctx)

		orgSlug := chi.URLParam(r, "org_slug")
		projectSlug := chi.URLParam(r, "project_slug")

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

		days := 30
		if daysStr := r.URL.Query().Get("days"); daysStr != "" {
			if parsed, err := strconv.Atoi(daysStr); err == nil && parsed > 0 {
				days = parsed
			}
		}

		repo := r.URL.Query().Get("repo")
		jobName := r.URL.Query().Get("job_name")

		req := flake.ListFlakesRequest{
			Days:    days,
			Repo:    repo,
			JobName: jobName,
			Limit:   100,
			Offset:  0,
		}

		flakeService := flake.NewService(pool)
		flakes, total, err := flakeService.ListFlakes(ctx, project.ID, req)
		if err != nil {
			log.Error().Err(err).Str("project_id", project.ID.String()).Msg("Failed to list flakes")
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
			Title:           "Flaky Tests - " + project.Name,
			UserID:          userID,
			IsAuthenticated: true,
			CSRFToken:       csrfToken,
			Data: map[string]interface{}{
				"OrgID":       org.ID,
				"ProjectID":   project.ID,
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

// HandleFlakeDetailPage renders the flake detail page.
func HandleFlakeDetailPage(pool *pgxpool.Pool, isProduction bool) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		ctx := r.Context()
		userID := auth.GetUserID(ctx)

		orgSlug := chi.URLParam(r, "org_slug")
		projectSlug := chi.URLParam(r, "project_slug")
		testCaseIDStr := chi.URLParam(r, "test_case_id")

		testCaseID, err := uuid.Parse(testCaseIDStr)
		if err != nil {
			http.Error(w, "Invalid test case ID", http.StatusBadRequest)
			return
		}

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

		days := 30
		if daysStr := r.URL.Query().Get("days"); daysStr != "" {
			if parsed, err := strconv.Atoi(daysStr); err == nil && parsed > 0 {
				days = parsed
			}
		}

		flakeService := flake.NewService(pool)
		detail, evidenceTotal, err := flakeService.GetFlakeDetail(ctx, project.ID, testCaseID, days, 100, 0)
		if err != nil {
			if errors.Is(err, flake.ErrFlakeNotFound) {
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

		lastFailureDisplay := ""
		lastFailureTruncated := false
		lastFailureIngestionTruncated := false
		if detail.LastFailureMessage != nil {
			msg := *detail.LastFailureMessage
			lastFailureIngestionTruncated = strings.Contains(msg, "[truncated]") || strings.Contains(msg, "(truncated)")
			lastFailureDisplay, lastFailureTruncated = truncateRunes(msg, 500)
		}

		csrfToken, err := auth.GenerateCSRFToken()
		if err != nil {
			http.Error(w, "Internal Server Error", http.StatusInternalServerError)
			return
		}
		auth.SetCSRFCookie(w, csrfToken, isProduction)

		data := &TemplateData{
			Title:           "Flake Detail - " + detail.TestIdentifier,
			UserID:          userID,
			IsAuthenticated: true,
			CSRFToken:       csrfToken,
			Data: map[string]interface{}{
				"OrgID":                                org.ID,
				"ProjectID":                            project.ID,
				"OrgSlug":                              orgSlug,
				"ProjectSlug":                          projectSlug,
				"ProjectName":                          project.Name,
				"Days":                                 days,
				"Detail":                               detail,
				"EvidenceTotal":                        evidenceTotal,
				"LastFailureMessageDisplay":            lastFailureDisplay,
				"LastFailureMessageTruncated":          lastFailureTruncated,
				"LastFailureMessageIngestionTruncated": lastFailureIngestionTruncated,
			},
		}
		RenderTemplate(w, r, "flake_detail.html", data)
	}
}
