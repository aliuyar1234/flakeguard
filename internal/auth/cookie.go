package auth

import (
	"net/http"
)

const (
	// SessionCookieName is the name of the session cookie
	SessionCookieName = "fg_session"
)

// SetSessionCookie sets the session cookie with the JWT token
// Cookie is HttpOnly, SameSite=Lax, and Secure in production
func SetSessionCookie(w http.ResponseWriter, token string, sessionDays int, isProduction bool) {
	cookie := &http.Cookie{
		Name:     SessionCookieName,
		Value:    token,
		Path:     "/",
		MaxAge:   sessionDays * 24 * 60 * 60, // Convert days to seconds
		HttpOnly: true,
		Secure:   isProduction, // Only set Secure flag in production
		SameSite: http.SameSiteLaxMode,
	}
	http.SetCookie(w, cookie)
}

// ClearSessionCookie clears the session cookie by setting MaxAge to -1
func ClearSessionCookie(w http.ResponseWriter) {
	cookie := &http.Cookie{
		Name:     SessionCookieName,
		Value:    "",
		Path:     "/",
		MaxAge:   -1,
		HttpOnly: true,
		SameSite: http.SameSiteLaxMode,
	}
	http.SetCookie(w, cookie)
}

// GetSessionCookie reads the session cookie from the request
// Returns the cookie value or empty string if not found
func GetSessionCookie(r *http.Request) string {
	cookie, err := r.Cookie(SessionCookieName)
	if err != nil {
		return ""
	}
	return cookie.Value
}
