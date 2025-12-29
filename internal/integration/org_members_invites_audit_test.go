package integration

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"net/http/cookiejar"
	"net/http/httptest"
	"net/url"
	"strconv"
	"testing"

	"github.com/aliuyar1234/flakeguard/internal/app"
	"github.com/aliuyar1234/flakeguard/internal/auth"
	"github.com/aliuyar1234/flakeguard/internal/config"
	"github.com/aliuyar1234/flakeguard/internal/orgs"
	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
)

type errorEnvelope struct {
	Error struct {
		Code      string `json:"code"`
		Message   string `json:"message"`
		RequestID string `json:"request_id"`
	} `json:"error"`
}

func TestE2E_OrgInvites_Members_LastOwnerGuardrails_Audit(t *testing.T) {
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

	ownerClient, ownerCSRF := newCSRFClient(t, srv.URL)
	inviteeClient, inviteeCSRF := newCSRFClient(t, srv.URL)

	ownerEmail := "owner@example.com"
	inviteeEmail := "invitee@example.com"
	password := "password123"

	ownerUserID := signupAndLogin(t, ownerClient, srv.URL, ownerCSRF, ownerEmail, password)
	inviteeUserID := signupAndLogin(t, inviteeClient, srv.URL, inviteeCSRF, inviteeEmail, password)

	orgID := createOrg(t, ownerClient, srv.URL, ownerCSRF, "Acme", "acme")

	inviteToken := createInvite(t, ownerClient, srv.URL, ownerCSRF, orgID, inviteeEmail, orgs.RoleMember)

	acceptInvite(t, inviteeClient, srv.URL, inviteeCSRF, inviteToken)

	members := listMembers(t, ownerClient, srv.URL, orgID)
	require.Len(t, members, 2)
	require.Contains(t, []uuid.UUID{members[0].UserID, members[1].UserID}, ownerUserID)
	require.Contains(t, []uuid.UUID{members[0].UserID, members[1].UserID}, inviteeUserID)

	updateRole(t, ownerClient, srv.URL, ownerCSRF, orgID, inviteeUserID, orgs.RoleAdmin)

	{
		errEnv := doJSONExpectError(t, ownerClient, http.MethodPut, srv.URL+"/api/v1/orgs/"+orgID.String()+"/members/"+ownerUserID.String(), ownerCSRF, http.StatusConflict, map[string]any{
			"role": string(orgs.RoleMember),
		})
		require.Equal(t, "conflict", errEnv.Error.Code)
	}

	removeMember(t, ownerClient, srv.URL, ownerCSRF, orgID, inviteeUserID)

	{
		errEnv := doJSONExpectError(t, ownerClient, http.MethodDelete, srv.URL+"/api/v1/orgs/"+orgID.String()+"/members/"+ownerUserID.String(), ownerCSRF, http.StatusConflict, nil)
		require.Equal(t, "conflict", errEnv.Error.Code)
	}

	events := listAudit(t, ownerClient, srv.URL, orgID, 50)
	actions := make(map[string]bool)
	for _, ev := range events {
		actions[ev.Action] = true
	}
	require.True(t, actions["org.invite_created"], "missing org.invite_created audit event")
	require.True(t, actions["org.invite_accepted"], "missing org.invite_accepted audit event")
	require.True(t, actions["org.member_role_updated"], "missing org.member_role_updated audit event")
	require.True(t, actions["org.member_removed"], "missing org.member_removed audit event")
}

func newCSRFClient(t *testing.T, serverURL string) (*http.Client, string) {
	t.Helper()

	jar, err := cookiejar.New(nil)
	require.NoError(t, err)
	client := &http.Client{Jar: jar}

	baseURL, err := url.Parse(serverURL)
	require.NoError(t, err)

	csrfToken, err := auth.GenerateCSRFToken()
	require.NoError(t, err)
	jar.SetCookies(baseURL, []*http.Cookie{{
		Name:  auth.CSRFCookieName,
		Value: csrfToken,
		Path:  "/",
	}})

	return client, csrfToken
}

func signupAndLogin(t *testing.T, client *http.Client, baseURL, csrfToken, email, password string) uuid.UUID {
	t.Helper()

	signupResp := postJSONExpectStatus(t, client, baseURL+"/api/v1/auth/signup", csrfToken, http.StatusCreated, map[string]any{
		"email":    email,
		"password": password,
	})

	var signup struct {
		User struct {
			ID uuid.UUID `json:"id"`
		} `json:"user"`
	}
	require.NoError(t, json.Unmarshal(signupResp.Data, &signup))
	require.NotEqual(t, uuid.Nil, signup.User.ID)

	postJSONExpectStatus(t, client, baseURL+"/api/v1/auth/login", csrfToken, http.StatusOK, map[string]any{
		"email":    email,
		"password": password,
	})

	return signup.User.ID
}

func createOrg(t *testing.T, client *http.Client, baseURL, csrfToken, name, slug string) uuid.UUID {
	t.Helper()

	orgResp := postJSONExpectStatus(t, client, baseURL+"/api/v1/orgs", csrfToken, http.StatusCreated, map[string]any{
		"name": name,
		"slug": slug,
	})

	var parsed struct {
		Org struct {
			ID uuid.UUID `json:"id"`
		} `json:"org"`
	}
	require.NoError(t, json.Unmarshal(orgResp.Data, &parsed))
	require.NotEqual(t, uuid.Nil, parsed.Org.ID)

	return parsed.Org.ID
}

func createInvite(t *testing.T, client *http.Client, baseURL, csrfToken string, orgID uuid.UUID, email string, role orgs.OrgRole) string {
	t.Helper()

	inviteResp := postJSONExpectStatus(t, client, baseURL+"/api/v1/orgs/"+orgID.String()+"/invites", csrfToken, http.StatusCreated, map[string]any{
		"email": email,
		"role":  string(role),
	})

	var parsed struct {
		Invite struct {
			Token string `json:"token"`
		} `json:"invite"`
	}
	require.NoError(t, json.Unmarshal(inviteResp.Data, &parsed))
	require.NotEmpty(t, parsed.Invite.Token)

	return parsed.Invite.Token
}

func acceptInvite(t *testing.T, client *http.Client, baseURL, csrfToken, token string) {
	t.Helper()

	postJSONExpectStatus(t, client, baseURL+"/api/v1/orgs/invites/accept", csrfToken, http.StatusOK, map[string]any{
		"token": token,
	})
}

func listMembers(t *testing.T, client *http.Client, baseURL string, orgID uuid.UUID) []orgs.MemberInfo {
	t.Helper()

	resp, err := client.Get(baseURL + "/api/v1/orgs/" + orgID.String() + "/members")
	require.NoError(t, err)
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	require.NoError(t, err)
	require.Equal(t, http.StatusOK, resp.StatusCode, "body: %s", string(body))

	var env struct {
		RequestID string `json:"request_id"`
		Data      struct {
			Members []orgs.MemberInfo `json:"members"`
		} `json:"data"`
	}
	require.NoError(t, json.Unmarshal(body, &env))
	require.NotEmpty(t, env.RequestID)

	return env.Data.Members
}

func updateRole(t *testing.T, client *http.Client, baseURL, csrfToken string, orgID, userID uuid.UUID, role orgs.OrgRole) {
	t.Helper()

	doJSONExpectSuccess(t, client, http.MethodPut, baseURL+"/api/v1/orgs/"+orgID.String()+"/members/"+userID.String(), csrfToken, http.StatusOK, map[string]any{
		"role": string(role),
	})
}

func removeMember(t *testing.T, client *http.Client, baseURL, csrfToken string, orgID, userID uuid.UUID) {
	t.Helper()

	doJSONExpectSuccess(t, client, http.MethodDelete, baseURL+"/api/v1/orgs/"+orgID.String()+"/members/"+userID.String(), csrfToken, http.StatusOK, nil)
}

func listAudit(t *testing.T, client *http.Client, baseURL string, orgID uuid.UUID, limit int) []struct {
	Action string `json:"action"`
} {
	t.Helper()

	u := baseURL + "/api/v1/orgs/" + orgID.String() + "/audit?limit=" + strconv.Itoa(limit)
	resp, err := client.Get(u)
	require.NoError(t, err)
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	require.NoError(t, err)
	require.Equal(t, http.StatusOK, resp.StatusCode, "body: %s", string(body))

	var env struct {
		RequestID string `json:"request_id"`
		Data      struct {
			Events []struct {
				Action string `json:"action"`
			} `json:"events"`
		} `json:"data"`
	}
	require.NoError(t, json.Unmarshal(body, &env))
	require.NotEmpty(t, env.RequestID)

	return env.Data.Events
}

func doJSONExpectSuccess(t *testing.T, client *http.Client, method, urlStr, csrfToken string, wantStatus int, payload any) envelopeResponse {
	t.Helper()

	respBody := doJSONExpectStatus(t, client, method, urlStr, csrfToken, wantStatus, payload)

	var env envelopeResponse
	require.NoError(t, json.Unmarshal(respBody, &env))
	require.NotEmpty(t, env.RequestID)

	return env
}

func doJSONExpectError(t *testing.T, client *http.Client, method, urlStr, csrfToken string, wantStatus int, payload any) errorEnvelope {
	t.Helper()

	respBody := doJSONExpectStatus(t, client, method, urlStr, csrfToken, wantStatus, payload)

	var env errorEnvelope
	require.NoError(t, json.Unmarshal(respBody, &env))
	require.NotEmpty(t, env.Error.RequestID)

	return env
}

func doJSONExpectStatus(t *testing.T, client *http.Client, method, urlStr, csrfToken string, wantStatus int, payload any) []byte {
	t.Helper()

	var bodyReader io.Reader
	if payload != nil {
		b, err := json.Marshal(payload)
		require.NoError(t, err)
		bodyReader = bytes.NewReader(b)
	}

	req, err := http.NewRequest(method, urlStr, bodyReader)
	require.NoError(t, err)
	if payload != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	if method == http.MethodPost || method == http.MethodPut || method == http.MethodDelete {
		req.Header.Set(auth.CSRFHeaderName, csrfToken)
	}

	resp, err := client.Do(req)
	require.NoError(t, err)
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	require.NoError(t, err)
	require.Equal(t, wantStatus, resp.StatusCode, "body: %s", string(body))

	return body
}
