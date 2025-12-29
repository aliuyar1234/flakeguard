package flake

import (
	"errors"
	"net/http"
	"strconv"

	"github.com/aliuyar1234/flakeguard/internal/apperrors"
	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/rs/zerolog/log"
)

// ListFlakesRequest represents query parameters for listing flakes (internal pagination used by UI).
type ListFlakesRequest struct {
	Days    int
	Repo    string
	JobName string
	Limit   int
	Offset  int
}

// HandleListFlakes handles GET /api/v1/projects/{project_id}/flakes.
func HandleListFlakes(pool *pgxpool.Pool) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		ctx := r.Context()

		projectIDStr := chi.URLParam(r, "project_id")
		projectID, err := uuid.Parse(projectIDStr)
		if err != nil {
			apperrors.WriteBadRequest(w, r, "Invalid project_id")
			return
		}

		req := parseListFlakesRequest(r)

		service := NewService(pool)
		flakes, _, err := service.ListFlakes(ctx, projectID, req)
		if err != nil {
			log.Error().Err(err).Str("project_id", projectID.String()).Msg("Failed to list flakes")
			apperrors.WriteInternalError(w, r, "Failed to retrieve flakes")
			return
		}

		apperrors.WriteSuccess(w, r, http.StatusOK, map[string]any{
			"flakes": flakes,
		})
	}
}

// HandleGetFlakeDetail handles GET /api/v1/projects/{project_id}/flakes/{test_case_id}?days=30.
func HandleGetFlakeDetail(pool *pgxpool.Pool) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		ctx := r.Context()

		projectIDStr := chi.URLParam(r, "project_id")
		projectID, err := uuid.Parse(projectIDStr)
		if err != nil {
			apperrors.WriteBadRequest(w, r, "Invalid project_id")
			return
		}

		testCaseIDStr := chi.URLParam(r, "test_case_id")
		testCaseID, err := uuid.Parse(testCaseIDStr)
		if err != nil {
			apperrors.WriteBadRequest(w, r, "Invalid test_case_id")
			return
		}

		days := 30
		if daysStr := r.URL.Query().Get("days"); daysStr != "" {
			if parsed, err := strconv.Atoi(daysStr); err == nil && parsed > 0 {
				days = parsed
			}
		}

		service := NewService(pool)
		detail, _, err := service.GetFlakeDetail(ctx, projectID, testCaseID, days, 100, 0)
		if err != nil {
			if errors.Is(err, ErrFlakeNotFound) {
				apperrors.WriteNotFound(w, r, "Flake not found")
				return
			}
			log.Error().Err(err).
				Str("project_id", projectID.String()).
				Str("test_case_id", testCaseID.String()).
				Msg("Failed to get flake detail")
			apperrors.WriteInternalError(w, r, "Failed to retrieve flake detail")
			return
		}

		apperrors.WriteSuccess(w, r, http.StatusOK, map[string]any{
			"flake": detail,
		})
	}
}

func parseListFlakesRequest(r *http.Request) ListFlakesRequest {
	req := ListFlakesRequest{
		Days:   30,
		Limit:  100,
		Offset: 0,
	}

	if daysStr := r.URL.Query().Get("days"); daysStr != "" {
		if days, err := strconv.Atoi(daysStr); err == nil && days > 0 {
			req.Days = days
		}
	}

	if repo := r.URL.Query().Get("repo"); repo != "" {
		req.Repo = repo
	}

	if jobName := r.URL.Query().Get("job_name"); jobName != "" {
		req.JobName = jobName
	}

	return req
}
