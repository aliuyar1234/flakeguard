package integration

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"
	"time"

	"github.com/flakeguard/flakeguard/internal/apikeys"
	"github.com/flakeguard/flakeguard/internal/app"
	"github.com/flakeguard/flakeguard/internal/config"
	"github.com/flakeguard/flakeguard/internal/ingest"
	"github.com/flakeguard/flakeguard/internal/orgs"
	"github.com/flakeguard/flakeguard/internal/projects"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/stretchr/testify/require"
)

type successEnvelope struct {
	RequestID string          `json:"request_id"`
	Data      json.RawMessage `json:"data"`
}

func TestIntegration_IngestEndpointWritesRowsAndUpdatesFlakeStats(t *testing.T) {
	pool, cleanup := newTestDB(t)
	t.Cleanup(cleanup)

	ctx := context.Background()

	userID := insertUser(t, pool, "integration@example.com")
	orgService := orgs.NewService(pool)
	org, err := orgService.CreateWithOwner(ctx, "Acme", "acme", userID)
	require.NoError(t, err)

	projectService := projects.NewService(pool)
	project, err := projectService.Create(ctx, org.ID, "Project", "my-project", "main", userID)
	require.NoError(t, err)

	apiKeyService := apikeys.NewService(pool)
	_, token, err := apiKeyService.Create(ctx, project.ID, "CI", []apikeys.ApiKeyScope{apikeys.ScopeIngestWrite}, userID)
	require.NoError(t, err)

	cfg := &config.Config{
		Env:            "dev",
		HTTPAddr:       ":0",
		BaseURL:        "http://localhost",
		DBDSN:          "unused",
		JWTSecret:      "test-secret",
		LogLevel:       "error",
		RateLimitRPM:   120,
		MaxUploadBytes: 5 * 1024 * 1024,
		MaxUploadFiles: 20,
		MaxFileBytes:   1 * 1024 * 1024,
		SlackTimeoutMS: 2000,
		SessionDays:    7,
	}

	srv := httptest.NewServer(app.NewRouter(pool, cfg))
	t.Cleanup(srv.Close)

	metaBase := ingest.IngestionMetadata{
		ProjectSlug:     project.Slug,
		RepoFullName:    "acme/repo",
		WorkflowName:    "CI",
		WorkflowRef:     "refs/heads/main",
		GitHubRunID:     123,
		GitHubRunNumber: 456,
		RunURL:          "https://github.example/runs/123",
		SHA:             "deadbeef",
		Branch:          "main",
		Event:           "push",
		JobName:         "unit",
		JobVariant:      "",
		StartedAt:       time.Now().Add(-2 * time.Minute).UTC().Format(time.RFC3339),
		CompletedAt:     time.Now().Add(-1 * time.Minute).UTC().Format(time.RFC3339),
	}

	meta1 := metaBase
	meta1.GitHubRunAttempt = 1
	accepted1 := ingestJUnit(t, srv.URL, token, meta1, "flaky_attempt1.xml")
	require.Equal(t, 0, accepted1.FlakeEventsCreated)
	require.Equal(t, 1, accepted1.Stored.TestResults)

	meta2 := metaBase
	meta2.GitHubRunAttempt = 2
	accepted2 := ingestJUnit(t, srv.URL, token, meta2, "flaky_attempt2.xml")
	require.Equal(t, 1, accepted2.FlakeEventsCreated)
	require.Equal(t, 1, accepted2.Stored.TestResults)

	accepted2Repeat := ingestJUnit(t, srv.URL, token, meta2, "flaky_attempt2.xml")
	require.Equal(t, 0, accepted2Repeat.FlakeEventsCreated)
	require.Equal(t, 0, accepted2Repeat.Stored.TestResults)

	assertDBCounts(t, pool, map[string]int{
		"test_cases":   1,
		"test_results": 2,
		"flake_events": 1,
		"flake_stats":  1,
	})

	var flakeScore float64
	err = pool.QueryRow(ctx, `
		SELECT fs.flake_score
		FROM flake_stats fs
		JOIN test_cases tc ON tc.id = fs.test_case_id
		WHERE tc.test_identifier = $1
	`, "com.example.FlakyTest#testFlaky").Scan(&flakeScore)
	require.NoError(t, err)
	require.Equal(t, 1.0, flakeScore)
}

type ingestAcceptedData struct {
	IngestionID string `json:"ingestion_id"`
	Stored      struct {
		JUnitFiles  int `json:"junit_files"`
		TestResults int `json:"test_results"`
	} `json:"stored"`
	FlakeEventsCreated int `json:"flake_events_created"`
}

func ingestJUnit(t *testing.T, baseURL, token string, meta ingest.IngestionMetadata, fixtureName string) ingestAcceptedData {
	t.Helper()

	metaBytes, err := json.Marshal(meta)
	require.NoError(t, err)

	body := &bytes.Buffer{}
	writer := multipart.NewWriter(body)
	require.NoError(t, writer.WriteField("meta", string(metaBytes)))

	part, err := writer.CreateFormFile("junit", fixtureName)
	require.NoError(t, err)

	fileBytes, err := os.ReadFile(junitFixturePath(t, fixtureName))
	require.NoError(t, err)
	_, err = part.Write(fileBytes)
	require.NoError(t, err)
	require.NoError(t, writer.Close())

	req, err := http.NewRequest(http.MethodPost, baseURL+"/api/v1/ingest/junit", body)
	require.NoError(t, err)
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("Content-Type", writer.FormDataContentType())

	resp, err := http.DefaultClient.Do(req)
	require.NoError(t, err)
	defer resp.Body.Close()

	respBody, err := io.ReadAll(resp.Body)
	require.NoError(t, err)
	require.Equal(t, http.StatusAccepted, resp.StatusCode, "body: %s", string(respBody))

	var env successEnvelope
	require.NoError(t, json.Unmarshal(respBody, &env))
	require.NotEmpty(t, env.RequestID)

	var data ingestAcceptedData
	require.NoError(t, json.Unmarshal(env.Data, &data))
	require.NotEmpty(t, data.IngestionID)

	return data
}

func insertUser(t *testing.T, pool *pgxpool.Pool, email string) uuid.UUID {
	t.Helper()

	email = fmt.Sprintf("%s-%s", email, uuid.NewString())

	var userID uuid.UUID
	err := pool.QueryRow(context.Background(), `
		INSERT INTO users (email, password_hash)
		VALUES ($1, $2)
		RETURNING id
	`, email, "x").Scan(&userID)
	require.NoError(t, err)
	require.NotEqual(t, uuid.Nil, userID)
	return userID
}

func assertDBCounts(t *testing.T, pool *pgxpool.Pool, expected map[string]int) {
	t.Helper()

	for table, want := range expected {
		var got int
		err := pool.QueryRow(context.Background(), fmt.Sprintf(`SELECT COUNT(*) FROM %s`, table)).Scan(&got)
		require.NoError(t, err, "table: %s", table)
		require.Equal(t, want, got, "table: %s", table)
	}
}
