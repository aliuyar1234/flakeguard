package flake

import (
	"net/http"
	"strconv"

	"github.com/flakeguard/flakeguard/internal/apperrors"
	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/rs/zerolog/log"
)

// ListFlakesRequest represents query parameters for listing flakes
type ListFlakesRequest struct {
	Days     int
	Repo     string
	JobName  string
	Limit    int
	Offset   int
}

// ListFlakesResponse represents the response for listing flakes
type ListFlakesResponse struct {
	Flakes []FlakeListItem `json:"flakes"`
	Total  int             `json:"total"`
	Limit  int             `json:"limit"`
	Offset int             `json:"offset"`
}

// FlakeDetailResponse represents the response for flake detail
type FlakeDetailResponse struct {
	FlakeDetail
	EvidenceTotal int `json:"evidence_total"`
	EvidenceLimit int `json:"evidence_limit"`
}

// HandleListFlakes handles GET /api/v1/projects/{project_id}/flakes
func HandleListFlakes(pool *pgxpool.Pool) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		ctx := r.Context()

		// Parse project_id from URL
		projectIDStr := chi.URLParam(r, "project_id")
		projectID, err := uuid.Parse(projectIDStr)
		if err != nil {
			apperrors.WriteBadRequest(w, r, "Invalid project_id")
			return
		}

		// Parse query parameters
		req := parseListFlakesRequest(r)

		// Query flake stats
		service := NewService(pool)
		flakes, total, err := service.ListFlakes(ctx, projectID, req)
		if err != nil {
			log.Error().Err(err).Str("project_id", projectID.String()).Msg("Failed to list flakes")
			apperrors.WriteInternalError(w, r, "Failed to retrieve flakes")
			return
		}

		response := ListFlakesResponse{
			Flakes: flakes,
			Total:  total,
			Limit:  req.Limit,
			Offset: req.Offset,
		}

		apperrors.WriteSuccess(w, r, http.StatusOK, response)
	}
}

// HandleGetFlakeDetail handles GET /api/v1/projects/{project_id}/flakes/{test_case_id}
func HandleGetFlakeDetail(pool *pgxpool.Pool) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		ctx := r.Context()

		// Parse project_id from URL
		projectIDStr := chi.URLParam(r, "project_id")
		projectID, err := uuid.Parse(projectIDStr)
		if err != nil {
			apperrors.WriteBadRequest(w, r, "Invalid project_id")
			return
		}

		// Parse test_case_id from URL
		testCaseIDStr := chi.URLParam(r, "test_case_id")
		testCaseID, err := uuid.Parse(testCaseIDStr)
		if err != nil {
			apperrors.WriteBadRequest(w, r, "Invalid test_case_id")
			return
		}

		// Parse pagination for evidence
		evidenceLimit := 100
		evidenceOffset := 0
		if limitStr := r.URL.Query().Get("evidence_limit"); limitStr != "" {
			if parsed, err := strconv.Atoi(limitStr); err == nil && parsed > 0 && parsed <= 1000 {
				evidenceLimit = parsed
			}
		}
		if offsetStr := r.URL.Query().Get("evidence_offset"); offsetStr != "" {
			if parsed, err := strconv.Atoi(offsetStr); err == nil && parsed >= 0 {
				evidenceOffset = parsed
			}
		}

		// Query flake detail
		service := NewService(pool)
		detail, evidenceTotal, err := service.GetFlakeDetail(ctx, projectID, testCaseID, evidenceLimit, evidenceOffset)
		if err != nil {
			if err.Error() == "flake not found" {
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

		response := FlakeDetailResponse{
			FlakeDetail:   *detail,
			EvidenceTotal: evidenceTotal,
			EvidenceLimit: evidenceLimit,
		}

		apperrors.WriteSuccess(w, r, http.StatusOK, response)
	}
}

// parseListFlakesRequest parses query parameters for list flakes
func parseListFlakesRequest(r *http.Request) ListFlakesRequest {
	req := ListFlakesRequest{
		Days:   30,   // Default 30 days
		Limit:  100,  // Default 100 items
		Offset: 0,
	}

	// Parse days
	if daysStr := r.URL.Query().Get("days"); daysStr != "" {
		if days, err := strconv.Atoi(daysStr); err == nil && days > 0 {
			req.Days = days
		}
	}

	// Parse repo filter
	if repo := r.URL.Query().Get("repo"); repo != "" {
		req.Repo = repo
	}

	// Parse job_name filter
	if jobName := r.URL.Query().Get("job_name"); jobName != "" {
		req.JobName = jobName
	}

	// Parse limit
	if limitStr := r.URL.Query().Get("limit"); limitStr != "" {
		if limit, err := strconv.Atoi(limitStr); err == nil && limit > 0 && limit <= 1000 {
			req.Limit = limit
		}
	}

	// Parse offset
	if offsetStr := r.URL.Query().Get("offset"); offsetStr != "" {
		if offset, err := strconv.Atoi(offsetStr); err == nil && offset >= 0 {
			req.Offset = offset
		}
	}

	return req
}
