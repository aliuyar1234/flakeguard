package auth

import (
	"crypto/rand"
	"encoding/base64"
	"fmt"
	"net/http"
)

const (
	// CSRFCookieName is the name of the CSRF cookie
	CSRFCookieName = "_csrf"

	// CSRFTokenBytes is the number of random bytes for CSRF tokens
	CSRFTokenBytes = 32
)

// GenerateCSRFToken generates a cryptographically secure CSRF token
// Returns a base64url-encoded 32-byte random token
func GenerateCSRFToken() (string, error) {
	bytes := make([]byte, CSRFTokenBytes)
	if _, err := rand.Read(bytes); err != nil {
		return "", fmt.Errorf("failed to generate random bytes: %w", err)
	}
	return base64.RawURLEncoding.EncodeToString(bytes), nil
}

// SetCSRFCookie sets the CSRF token in a cookie
// Uses double-submit cookie pattern for CSRF protection
func SetCSRFCookie(w http.ResponseWriter, token string, isProduction bool) {
	cookie := &http.Cookie{
		Name:     CSRFCookieName,
		Value:    token,
		Path:     "/",
		HttpOnly: false, // Must be accessible to JavaScript for form submission
		Secure:   isProduction,
		SameSite: http.SameSiteLaxMode,
	}
	http.SetCookie(w, cookie)
}

// GetCSRFCookie reads the CSRF token from the cookie
func GetCSRFCookie(r *http.Request) string {
	cookie, err := r.Cookie(CSRFCookieName)
	if err != nil {
		return ""
	}
	return cookie.Value
}

// ValidateCSRF validates the CSRF token using double-submit pattern
// Compares the token from the form/header with the token from the cookie
func ValidateCSRF(r *http.Request) error {
	// Get token from cookie
	cookieToken := GetCSRFCookie(r)
	if cookieToken == "" {
		return fmt.Errorf("missing CSRF cookie")
	}

	// Get token from form or header
	var formToken string
	if r.Method == http.MethodPost || r.Method == http.MethodPut || r.Method == http.MethodDelete {
		// Try form field first
		formToken = r.FormValue(CSRFCookieName)
		if formToken == "" {
			// Fall back to header
			formToken = r.Header.Get("X-CSRF-Token")
		}
	}

	if formToken == "" {
		return fmt.Errorf("missing CSRF token in request")
	}

	// Compare tokens (constant-time comparison would be ideal, but for now simple comparison)
	if cookieToken != formToken {
		return fmt.Errorf("CSRF token mismatch")
	}

	return nil
}
