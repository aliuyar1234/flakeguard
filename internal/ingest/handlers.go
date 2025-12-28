package ingest

import (
	"bytes"
	"crypto/sha256"
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

const maxStoredJUnitContentBytes = 64 * 1024

// HandleJUnitUpload handles POST /api/v1/ingest/junit.
func HandleJUnitUpload(pool *pgxpool.Pool, cfg *config.Config, limits UploadLimits) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		ctx := r.Context()

		key := apikey.GetAPIKey(ctx)
		if key == nil {
			apperrors.WriteUnauthorized(w, r, "API key required")
			return
		}

		if err := r.ParseMultipartForm(limits.MaxTotalBytes); err != nil {
			if err == multipart.ErrMessageTooLarge {
				apperrors.WritePayloadTooLarge(w, r, fmt.Sprintf("Upload exceeds maximum size of %d bytes", limits.MaxTotalBytes))
				return
			}
			apperrors.WriteBadRequest(w, r, "Failed to parse multipart form")
			return
		}
		defer r.MultipartForm.RemoveAll()

		metaBytes, err := readMetaBytes(r)
		if err != nil {
			apperrors.WriteError(w, r, http.StatusBadRequest, "invalid_meta", err.Error())
			return
		}

		var meta IngestionMetadata
		if err := json.Unmarshal(metaBytes, &meta); err != nil {
			apperrors.WriteError(w, r, http.StatusBadRequest, "invalid_meta", "meta must be valid JSON")
			return
		}
		if err := meta.Validate(); err != nil {
			apperrors.WriteError(w, r, http.StatusBadRequest, "invalid_meta", err.Error())
			return
		}

		projectService := projects.NewService(pool)
		project, err := projectService.GetByID(ctx, key.ProjectID)
		if err != nil {
			if errors.Is(err, projects.ErrProjectNotFound) {
				apperrors.WriteNotFound(w, r, "Project not found")
				return
			}
			log.Error().Err(err).Str("project_id", key.ProjectID.String()).Msg("Failed to lookup project")
			apperrors.WriteInternalError(w, r, "Failed to lookup project")
			return
		}

		if meta.ProjectSlug != project.Slug {
			apperrors.WriteError(w, r, http.StatusBadRequest, "invalid_meta", "meta.project_slug does not match API key project")
			return
		}

		files := r.MultipartForm.File["junit"]
		if len(files) == 0 {
			apperrors.WriteBadRequest(w, r, "No JUnit files provided")
			return
		}
		if err := limits.ValidateFileCount(len(files)); err != nil {
			if errors.Is(err, ErrTooManyFiles) {
				apperrors.WriteError(w, r, http.StatusBadRequest, "too_many_files", fmt.Sprintf("Maximum %d files allowed", limits.MaxFiles))
				return
			}
			apperrors.WriteBadRequest(w, r, err.Error())
			return
		}

		var totalSize int64
		for _, fileHeader := range files {
			totalSize += fileHeader.Size
			if err := limits.ValidateFileSize(fileHeader.Size, fileHeader.Filename); err != nil {
				apperrors.WritePayloadTooLarge(w, r, err.Error())
				return
			}
		}
		if err := limits.ValidateTotalSize(totalSize); err != nil {
			apperrors.WritePayloadTooLarge(w, r, err.Error())
			return
		}

		var allTestResults []TestResult
		junitFiles := make([]JUnitFile, 0, len(files))

		for _, fileHeader := range files {
			file, err := fileHeader.Open()
			if err != nil {
				log.Error().Err(err).Str("filename", fileHeader.Filename).Msg("Failed to open uploaded file")
				apperrors.WriteInternalError(w, r, "Failed to process uploaded files")
				return
			}

			buf := new(bytes.Buffer)
			size, err := io.Copy(buf, file)
			file.Close()

			if err != nil {
				log.Error().Err(err).Str("filename", fileHeader.Filename).Msg("Failed to read uploaded file")
				apperrors.WriteInternalError(w, r, "Failed to read uploaded files")
				return
			}

			results, err := ParseAndExtract(bytes.NewReader(buf.Bytes()))
			if err != nil {
				log.Error().Err(err).Str("filename", fileHeader.Filename).Msg("Failed to parse JUnit XML")
				apperrors.WriteError(w, r, http.StatusBadRequest, "invalid_junit_xml", fmt.Sprintf("Failed to parse JUnit XML in file '%s': %v", fileHeader.Filename, err))
				return
			}

			allTestResults = append(allTestResults, results...)

			sum := sha256.Sum256(buf.Bytes())

			storedContent := buf.Bytes()
			contentTruncated := false
			if len(storedContent) > maxStoredJUnitContentBytes {
				storedContent = storedContent[:maxStoredJUnitContentBytes]
				contentTruncated = true
			}

			junitFiles = append(junitFiles, JUnitFile{
				Filename:         fileHeader.Filename,
				SHA256:           fmt.Sprintf("%x", sum),
				SizeBytes:        int(size),
				ContentTruncated: contentTruncated,
				Content:          append([]byte(nil), storedContent...),
			})
		}

		persistence := NewPersistenceService(pool, cfg)
		result, err := persistence.PersistIngestion(ctx, project.ID, key.ID, &meta, junitFiles, allTestResults)
		if err != nil {
			log.Error().Err(err).Msg("Failed to persist ingestion")
			apperrors.WriteInternalError(w, r, "Failed to store ingestion data")
			return
		}

		log.Info().
			Str("ingestion_id", result.IngestionID.String()).
			Str("project_id", project.ID.String()).
			Str("project_slug", meta.ProjectSlug).
			Str("repo_full_name", meta.RepoFullName).
			Int64("github_run_id", meta.GitHubRunID).
			Int("github_run_attempt", meta.GitHubRunAttempt).
			Str("job_name", meta.JobName).
			Str("job_variant", meta.JobVariant).
			Int("stored_test_results", result.TestResultsCount).
			Int("stored_junit_files", result.JUnitFilesCount).
			Int("flake_events_created", result.FlakeEventsCount).
			Msg("Ingestion successful")

		apperrors.WriteSuccess(w, r, http.StatusAccepted, IngestionResponse{
			IngestionID: result.IngestionID,
			Stored: IngestionStoredCounts{
				JUnitFiles:  result.JUnitFilesCount,
				TestResults: result.TestResultsCount,
			},
			FlakeEventsCreated: result.FlakeEventsCount,
		})
	}
}

type IngestionStoredCounts struct {
	JUnitFiles  int `json:"junit_files"`
	TestResults int `json:"test_results"`
}

type IngestionResponse struct {
	IngestionID        uuid.UUID             `json:"ingestion_id"`
	Stored             IngestionStoredCounts `json:"stored"`
	FlakeEventsCreated int                   `json:"flake_events_created"`
}

func readMetaBytes(r *http.Request) ([]byte, error) {
	if meta := r.FormValue("meta"); meta != "" {
		return []byte(meta), nil
	}
	if r.MultipartForm != nil && r.MultipartForm.File != nil {
		if metaFiles := r.MultipartForm.File["meta"]; len(metaFiles) > 0 {
			f, err := metaFiles[0].Open()
			if err != nil {
				return nil, errors.New("meta is required")
			}
			defer f.Close()

			b, err := io.ReadAll(f)
			if err != nil {
				return nil, errors.New("meta must be valid JSON")
			}
			return b, nil
		}
	}
	return nil, errors.New("meta is required")
}
