package integration

import (
	"bytes"
	"encoding/json"
	"io"
	"mime/multipart"
	"net/http"
	"net/http/cookiejar"
	"net/http/httptest"
	"net/url"
	"os"
	"testing"
	"time"

	"github.com/flakeguard/flakeguard/internal/app"
	"github.com/flakeguard/flakeguard/internal/auth"
	"github.com/flakeguard/flakeguard/internal/config"
	"github.com/flakeguard/flakeguard/internal/flake"
	"github.com/flakeguard/flakeguard/internal/ingest"
	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
)

func TestE2E_MinimalFlow_SignupToListFlakes(t *testing.T) {
	pool, cleanup := newTestDB(t)
	t.Cleanup(cleanup)

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

	jar, err := cookiejar.New(nil)
	require.NoError(t, err)
	client := &http.Client{Jar: jar}

	baseURL, err := url.Parse(srv.URL)
	require.NoError(t, err)

	csrfToken, err := auth.GenerateCSRFToken()
	require.NoError(t, err)
	jar.SetCookies(baseURL, []*http.Cookie{{Name: auth.CSRFCookieName, Value: csrfToken, Path: "/"}})

	email := "e2e@example.com"
	password := "password123"

	postJSONExpectStatus(t, client, srv.URL+"/api/v1/auth/signup", csrfToken, http.StatusCreated, map[string]any{
		"email":    email,
		"password": password,
	})

	postJSONExpectStatus(t, client, srv.URL+"/api/v1/auth/login", csrfToken, http.StatusOK, map[string]any{
		"email":    email,
		"password": password,
	})

	var orgID uuid.UUID
	orgResp := postJSONExpectStatus(t, client, srv.URL+"/api/v1/orgs", csrfToken, http.StatusCreated, map[string]any{
		"name": "Acme",
		"slug": "acme",
	})

	{
		var parsed struct {
			Org struct {
				ID uuid.UUID `json:"id"`
			} `json:"org"`
		}
		require.NoError(t, json.Unmarshal(orgResp.Data, &parsed))
		orgID = parsed.Org.ID
	}

	var projectID uuid.UUID
	var projectSlug string
	projectResp := postJSONExpectStatus(t, client, srv.URL+"/api/v1/orgs/"+orgID.String()+"/projects", csrfToken, http.StatusCreated, map[string]any{
		"name":           "Project",
		"slug":           "my-project",
		"default_branch": "main",
	})
	{
		var parsed struct {
			Project struct {
				ID   uuid.UUID `json:"id"`
				Slug string    `json:"slug"`
			} `json:"project"`
		}
		require.NoError(t, json.Unmarshal(projectResp.Data, &parsed))
		projectID = parsed.Project.ID
		projectSlug = parsed.Project.Slug
	}

	apiKeyResp := postJSONExpectStatus(t, client, srv.URL+"/api/v1/projects/"+projectID.String()+"/api-keys", csrfToken, http.StatusCreated, map[string]any{
		"name": "CI",
	})
	var apiKeyToken string
	{
		var parsed struct {
			APIKey struct {
				Token string `json:"token"`
			} `json:"api_key"`
		}
		require.NoError(t, json.Unmarshal(apiKeyResp.Data, &parsed))
		apiKeyToken = parsed.APIKey.Token
	}
	require.NotEmpty(t, apiKeyToken)

	metaBase := ingest.IngestionMetadata{
		ProjectSlug:     projectSlug,
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
	ingestAccepted1 := postIngest(t, client, srv.URL, apiKeyToken, meta1, "flaky_attempt1.xml")
	require.Equal(t, 0, ingestAccepted1.FlakeEventsCreated)

	meta2 := metaBase
	meta2.GitHubRunAttempt = 2
	ingestAccepted2 := postIngest(t, client, srv.URL, apiKeyToken, meta2, "flaky_attempt2.xml")
	require.Equal(t, 1, ingestAccepted2.FlakeEventsCreated)

	resp, err := client.Get(srv.URL + "/api/v1/projects/" + projectID.String() + "/flakes")
	require.NoError(t, err)
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	require.NoError(t, err)
	require.Equal(t, http.StatusOK, resp.StatusCode, "body: %s", string(body))

	var env struct {
		RequestID string `json:"request_id"`
		Data      struct {
			Flakes []flake.FlakeListItem `json:"flakes"`
		} `json:"data"`
	}
	require.NoError(t, json.Unmarshal(body, &env))
	require.NotEmpty(t, env.RequestID)

	require.NotEmpty(t, env.Data.Flakes)
	require.Equal(t, "com.example.FlakyTest#testFlaky", env.Data.Flakes[0].TestIdentifier)
}

type envelopeResponse struct {
	RequestID string          `json:"request_id"`
	Data      json.RawMessage `json:"data"`
}

func postJSONExpectStatus(t *testing.T, client *http.Client, urlStr, csrfToken string, wantStatus int, payload any) envelopeResponse {
	t.Helper()

	b, err := json.Marshal(payload)
	require.NoError(t, err)

	req, err := http.NewRequest(http.MethodPost, urlStr, bytes.NewReader(b))
	require.NoError(t, err)
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set(auth.CSRFHeaderName, csrfToken)

	resp, err := client.Do(req)
	require.NoError(t, err)
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	require.NoError(t, err)
	require.Equal(t, wantStatus, resp.StatusCode, "body: %s", string(body))

	var env envelopeResponse
	require.NoError(t, json.Unmarshal(body, &env))
	require.NotEmpty(t, env.RequestID)

	return env
}

type ingestAccepted struct {
	IngestionID string `json:"ingestion_id"`
	Stored      struct {
		JUnitFiles  int `json:"junit_files"`
		TestResults int `json:"test_results"`
	} `json:"stored"`
	FlakeEventsCreated int `json:"flake_events_created"`
}

func postIngest(t *testing.T, client *http.Client, baseURL, apiKeyToken string, meta ingest.IngestionMetadata, fixtureName string) ingestAccepted {
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
	req.Header.Set("Authorization", "Bearer "+apiKeyToken)
	req.Header.Set("Content-Type", writer.FormDataContentType())

	resp, err := client.Do(req)
	require.NoError(t, err)
	defer resp.Body.Close()

	respBody, err := io.ReadAll(resp.Body)
	require.NoError(t, err)
	require.Equal(t, http.StatusAccepted, resp.StatusCode, "body: %s", string(respBody))

	var env struct {
		RequestID string         `json:"request_id"`
		Data      ingestAccepted `json:"data"`
	}
	require.NoError(t, json.Unmarshal(respBody, &env))
	require.NotEmpty(t, env.RequestID)
	require.NotEmpty(t, env.Data.IngestionID)

	return env.Data
}
