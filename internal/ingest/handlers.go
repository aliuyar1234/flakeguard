package ingest

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"mime/multipart"
	"net/http"

	"github.com/flakeguard/flakeguard/internal/apikey"
	"github.com/flakeguard/flakeguard/internal/apperrors"
	"github.com/flakeguard/flakeguard/internal/config"
	"github.com/flakeguard/flakeguard/internal/projects"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/rs/zerolog/log"
)

// HandleJUnitUpload handles POST /api/v1/ingest/junit
func HandleJUnitUpload(pool *pgxpool.Pool, cfg *config.Config, limits UploadLimits) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		ctx := r.Context()

		// Get API key from context (set by middleware)
		key := apikey.GetAPIKey(ctx)
		if key == nil {
			apperrors.WriteUnauthorized(w, r, "API key required")
			return
		}

		// Parse multipart form with size limit
		if err := r.ParseMultipartForm(limits.MaxTotalBytes); err != nil {
			if err == multipart.ErrMessageTooLarge {
				apperrors.WritePayloadTooLarge(w, r, fmt.Sprintf("Upload exceeds maximum size of %d bytes", limits.MaxTotalBytes))
				return
			}
			apperrors.WriteBadRequest(w, r, "Failed to parse multipart form")
			return
		}
		defer r.MultipartForm.RemoveAll()

		// Parse metadata from "meta" field
		metaField := r.FormValue("meta")
		if metaField == "" {
			apperrors.WriteBadRequest(w, r, "Missing 'meta' field in form data")
			return
		}

		var metadata IngestionMetadata
		if err := json.Unmarshal([]byte(metaField), &metadata); err != nil {
			apperrors.WriteBadRequest(w, r, "Invalid JSON in 'meta' field")
			return
		}

		// Validate metadata
		if err := metadata.Validate(); err != nil {
			apperrors.WriteBadRequest(w, r, fmt.Sprintf("Invalid metadata: %v", err))
			return
		}

		// Get JUnit files from form
		files := r.MultipartForm.File["junit"]
		if len(files) == 0 {
			apperrors.WriteBadRequest(w, r, "No JUnit files provided")
			return
		}

		// Validate file count
		if err := limits.ValidateFileCount(len(files)); err != nil {
			apperrors.WriteBadRequest(w, r, err.Error())
			return
		}

		// Validate total size and individual file sizes
		var totalSize int64
		for _, fileHeader := range files {
			totalSize += fileHeader.Size

			// Validate individual file size
			if err := limits.ValidateFileSize(fileHeader.Size, fileHeader.Filename); err != nil {
				apperrors.WritePayloadTooLarge(w, r, err.Error())
				return
			}
		}

		// Validate total size
		if err := limits.ValidateTotalSize(totalSize); err != nil {
			apperrors.WritePayloadTooLarge(w, r, err.Error())
			return
		}

		// Lookup project by slug
		projectService := projects.NewService(pool)
		project, err := projectService.GetBySlug(ctx, metadata.ProjectSlug)
		if err != nil {
			if errors.Is(err, projects.ErrProjectNotFound) {
				apperrors.WriteNotFound(w, r, "Project not found")
				return
			}
			log.Error().Err(err).Str("project_slug", metadata.ProjectSlug).Msg("Failed to lookup project")
			apperrors.WriteInternalError(w, r, "Failed to lookup project")
			return
		}

		// Verify API key belongs to this project
		if key.ProjectID != project.ID {
			apperrors.WriteForbidden(w, r, "API key does not have access to this project")
			return
		}

		// Parse all JUnit files and collect results
		var allTestResults []TestResult
		var junitFiles []JUnitFile

		for _, fileHeader := range files {
			file, err := fileHeader.Open()
			if err != nil {
				log.Error().Err(err).Str("filename", fileHeader.Filename).Msg("Failed to open uploaded file")
				apperrors.WriteInternalError(w, r, "Failed to process uploaded files")
				return
			}

			// Read file content into buffer
			buf := new(bytes.Buffer)
			size, err := io.Copy(buf, file)
			file.Close()

			if err != nil {
				log.Error().Err(err).Str("filename", fileHeader.Filename).Msg("Failed to read uploaded file")
				apperrors.WriteInternalError(w, r, "Failed to read uploaded files")
				return
			}

			// Parse JUnit XML
			results, err := ParseAndExtract(bytes.NewReader(buf.Bytes()))
			if err != nil {
				log.Error().Err(err).Str("filename", fileHeader.Filename).Msg("Failed to parse JUnit XML")
				apperrors.WriteBadRequest(w, r, fmt.Sprintf("Failed to parse JUnit XML in file '%s': %v", fileHeader.Filename, err))
				return
			}

			allTestResults = append(allTestResults, results...)
			junitFiles = append(junitFiles, JUnitFile{
				Filename:     fileHeader.Filename,
				ContentBytes: int(size),
			})
		}

		// Persist ingestion
		persistence := NewPersistenceService(pool, cfg)
		result, err := persistence.PersistIngestion(ctx, project.ID, key.ID, &metadata, junitFiles, allTestResults)
		if err != nil {
			log.Error().Err(err).Msg("Failed to persist ingestion")
			apperrors.WriteInternalError(w, r, "Failed to store ingestion data")
			return
		}

		// Log successful ingestion
		log.Info().
			Str("ingestion_id", result.IngestionID.String()).
			Str("project_slug", metadata.ProjectSlug).
			Str("repo", metadata.RepoFullName).
			Int("run_number", metadata.RunNumber).
			Int("run_attempt", metadata.RunAttempt).
			Str("job_name", metadata.JobName).
			Int("test_results", result.TestResultsCount).
			Int("junit_files", result.JUnitFilesCount).
			Msg("Ingestion successful")

		// Return 202 Accepted with ingestion details
		response := IngestionResponse{
			IngestionID:      result.IngestionID,
			TestResultsCount: result.TestResultsCount,
			JUnitFilesCount:  result.JUnitFilesCount,
			FlakeEventsCount: result.FlakeEventsCount,
		}

		apperrors.WriteSuccess(w, r, http.StatusAccepted, response)
	}
}

// IngestionResponse is the response returned after successful ingestion
type IngestionResponse struct {
	IngestionID      uuid.UUID `json:"ingestion_id"`
	TestResultsCount int       `json:"test_results_count"`
	JUnitFilesCount  int       `json:"junit_files_count"`
	FlakeEventsCount int       `json:"flake_events_created"`
}
