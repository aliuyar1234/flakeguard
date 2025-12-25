package slack

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"time"

	"github.com/rs/zerolog/log"
)

// FlakeMessage contains all information needed to send a flake notification to Slack
type FlakeMessage struct {
	Repo          string
	Workflow      string
	Job           string
	TestID        string
	FailedAttempt int
	PassedAttempt int
	DashboardURL  string
}

// Client handles Slack webhook notifications
type Client struct {
	httpClient *http.Client
	timeout    time.Duration
}

// NewClient creates a new Slack client with the specified timeout
func NewClient(timeoutMS int) *Client {
	return &Client{
		httpClient: &http.Client{
			Timeout: time.Duration(timeoutMS) * time.Millisecond,
		},
		timeout: time.Duration(timeoutMS) * time.Millisecond,
	}
}

// slackPayload represents the JSON payload sent to Slack
type slackPayload struct {
	Text string `json:"text"`
}

// PostFlakeNotification sends a flake notification to Slack
// This method NEVER returns errors to the caller - all failures are logged at WARN level
// This ensures Slack failures do not impact the calling code (e.g., ingestion pipeline)
func (c *Client) PostFlakeNotification(ctx context.Context, webhookURL string, msg FlakeMessage) {
	// Build the message text
	text := c.buildMessageText(msg)

	// Create payload
	payload := slackPayload{
		Text: text,
	}

	// Marshal to JSON
	jsonData, err := json.Marshal(payload)
	if err != nil {
		log.Warn().
			Err(err).
			Str("repo", msg.Repo).
			Str("test_id", msg.TestID).
			Msg("Failed to marshal Slack payload")
		return
	}

	// Create request
	req, err := http.NewRequestWithContext(ctx, "POST", webhookURL, bytes.NewBuffer(jsonData))
	if err != nil {
		log.Warn().
			Err(err).
			Str("webhook_url", "<set>").
			Msg("Failed to create Slack request")
		return
	}

	req.Header.Set("Content-Type", "application/json")

	// Send request
	resp, err := c.httpClient.Do(req)
	if err != nil {
		// Check if this is a timeout error
		if ctx.Err() == context.DeadlineExceeded || isTimeoutError(err) {
			log.Warn().
				Err(err).
				Dur("timeout_ms", c.timeout).
				Str("repo", msg.Repo).
				Str("test_id", msg.TestID).
				Msg("Slack notification timed out")
		} else {
			log.Warn().
				Err(err).
				Str("repo", msg.Repo).
				Str("test_id", msg.TestID).
				Msg("Failed to send Slack notification")
		}
		return
	}
	defer resp.Body.Close()

	// Check response status
	if resp.StatusCode >= 400 && resp.StatusCode < 500 {
		// 4xx errors (client errors)
		log.Warn().
			Int("status_code", resp.StatusCode).
			Str("repo", msg.Repo).
			Str("test_id", msg.TestID).
			Msg("Slack webhook returned client error (4xx)")
		return
	}

	if resp.StatusCode >= 500 {
		// 5xx errors (server errors)
		log.Warn().
			Int("status_code", resp.StatusCode).
			Str("repo", msg.Repo).
			Str("test_id", msg.TestID).
			Msg("Slack webhook returned server error (5xx)")
		return
	}

	if resp.StatusCode != http.StatusOK {
		log.Warn().
			Int("status_code", resp.StatusCode).
			Str("repo", msg.Repo).
			Str("test_id", msg.TestID).
			Msg("Slack webhook returned unexpected status code")
		return
	}

	// Success - notification delivered
	log.Info().
		Str("repo", msg.Repo).
		Str("job", msg.Job).
		Str("test_id", msg.TestID).
		Int("failed_attempt", msg.FailedAttempt).
		Int("passed_attempt", msg.PassedAttempt).
		Msg("Slack notification sent successfully")
}

// buildMessageText constructs the Slack message text with all flake details
func (c *Client) buildMessageText(msg FlakeMessage) string {
	return fmt.Sprintf(
		"🔄 *Flaky Test Detected*\n\n"+
			"*Repository:* %s\n"+
			"*Workflow:* %s\n"+
			"*Job:* %s\n"+
			"*Test:* `%s`\n\n"+
			"*Evidence:* Failed on attempt %d, passed on attempt %d\n\n"+
			"<%s|View Details>",
		msg.Repo,
		msg.Workflow,
		msg.Job,
		msg.TestID,
		msg.FailedAttempt,
		msg.PassedAttempt,
		msg.DashboardURL,
	)
}

// isTimeoutError checks if an error is a timeout error
func isTimeoutError(err error) bool {
	if err == nil {
		return false
	}
	// Check for common timeout error patterns
	return err.Error() == "context deadline exceeded" ||
		err.Error() == "Client.Timeout exceeded"
}
