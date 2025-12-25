package validation

import (
	"errors"
	"regexp"
	"strings"
)

var (
	// ErrInvalidSlug is returned when a slug doesn't match the required format
	ErrInvalidSlug = errors.New("invalid slug format")

	// ErrSlugTooShort is returned when a slug is too short
	ErrSlugTooShort = errors.New("slug must be at least 3 characters")

	// ErrSlugTooLong is returned when a slug is too long
	ErrSlugTooLong = errors.New("slug must be at most 64 characters")

	// slugRegex validates slug format: starts and ends with alphanumeric, can contain hyphens
	// Format: ^[a-z0-9][a-z0-9-]{1,62}[a-z0-9]$
	slugRegex = regexp.MustCompile(`^[a-z0-9][a-z0-9-]{1,62}[a-z0-9]$`)
)

// ValidateSlug validates a slug according to FlakeGuard rules:
// - Must be 3-64 characters long
// - Must start and end with lowercase alphanumeric (a-z, 0-9)
// - Can contain hyphens in the middle
// - No uppercase, no underscores, no other special characters
func ValidateSlug(slug string) error {
	// Normalize to lowercase
	slug = strings.ToLower(strings.TrimSpace(slug))

	// Check length
	if len(slug) < 3 {
		return ErrSlugTooShort
	}
	if len(slug) > 64 {
		return ErrSlugTooLong
	}

	// Check format
	if !slugRegex.MatchString(slug) {
		return ErrInvalidSlug
	}

	return nil
}

// NormalizeSlug normalizes a slug by converting to lowercase and trimming whitespace
func NormalizeSlug(slug string) string {
	return strings.ToLower(strings.TrimSpace(slug))
}

// ValidateWebhookURL validates a Slack webhook URL
func ValidateWebhookURL(url string) error {
	if url == "" {
		return errors.New("webhook URL is required")
	}

	if len(url) > 500 {
		return errors.New("webhook URL must be at most 500 characters")
	}

	if !strings.HasPrefix(url, "https://hooks.slack.com/services/") {
		return errors.New("webhook URL must start with https://hooks.slack.com/services/")
	}

	return nil
}
