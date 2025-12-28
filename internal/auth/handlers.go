package auth

import (
	"encoding/json"
	"errors"
	"net"
	"net/http"
	"net/mail"
	"strings"
	"sync"
	"time"

	"github.com/flakeguard/flakeguard/internal/apperrors"
	"github.com/flakeguard/flakeguard/internal/audit"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/rs/zerolog/log"
)

type signupRequest struct {
	Email    string `json:"email"`
	Password string `json:"password"`
}

type loginRequest struct {
	Email    string `json:"email"`
	Password string `json:"password"`
}

type userCreated struct {
	ID        uuid.UUID `json:"id"`
	Email     string    `json:"email"`
	CreatedAt string    `json:"created_at"`
}

type userSummary struct {
	ID    uuid.UUID `json:"id"`
	Email string    `json:"email"`
}

type signupResponse struct {
	User userCreated `json:"user"`
}

type loginResponse struct {
	User userSummary `json:"user"`
}

type logoutResponse struct {
	LoggedOut bool `json:"logged_out"`
}

type loginFailureLimiter struct {
	mu   sync.Mutex
	last map[string]time.Time
}

func (l *loginFailureLimiter) Allow(key string, now time.Time) bool {
	l.mu.Lock()
	defer l.mu.Unlock()
	if l.last == nil {
		l.last = make(map[string]time.Time)
	}

	const window = time.Minute
	if last, ok := l.last[key]; ok && now.Sub(last) < window {
		return false
	}
	l.last[key] = now
	return true
}

var loginFailedAuditLimiter loginFailureLimiter

// HandleSignup processes POST /api/v1/auth/signup.
func HandleSignup(pool *pgxpool.Pool, auditor *audit.Writer) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var req signupRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			apperrors.WriteError(w, r, http.StatusBadRequest, "bad_request", "Invalid request body")
			return
		}

		email := strings.TrimSpace(req.Email)
		password := req.Password

		if !isValidEmail(email) {
			apperrors.WriteError(w, r, http.StatusBadRequest, "invalid_email", "Email format is invalid")
			return
		}
		if len(password) < 8 {
			apperrors.WriteError(w, r, http.StatusBadRequest, "invalid_password", "Password must be at least 8 characters")
			return
		}

		passwordHash, err := HashPassword(password)
		if err != nil {
			log.Error().Err(err).Msg("Failed to hash password")
			apperrors.WriteInternalError(w, r, "Failed to create account")
			return
		}

		var userID uuid.UUID
		var createdAt time.Time

		query := `
			INSERT INTO users (email, password_hash)
			VALUES ($1, $2)
			RETURNING id, created_at
		`
		err = pool.QueryRow(r.Context(), query, email, passwordHash).Scan(&userID, &createdAt)
		if err != nil {
			var pgErr *pgconn.PgError
			if errors.As(err, &pgErr) && pgErr.Code == "23505" {
				apperrors.WriteError(w, r, http.StatusConflict, "email_exists", "Email already registered")
				return
			}

			log.Error().Err(err).Str("email", email).Msg("Failed to insert user")
			apperrors.WriteInternalError(w, r, "Failed to create account")
			return
		}

		if auditor != nil {
			if err := auditor.LogUserSignup(r.Context(), userID, email); err != nil {
				log.Error().Err(err).Str("user_id", userID.String()).Msg("Failed to log audit event")
			}
		}

		apperrors.WriteSuccess(w, r, http.StatusCreated, signupResponse{
			User: userCreated{
				ID:        userID,
				Email:     email,
				CreatedAt: createdAt.UTC().Format(time.RFC3339),
			},
		})
	}
}

// HandleLogin processes POST /api/v1/auth/login.
func HandleLogin(pool *pgxpool.Pool, auditor *audit.Writer, jwtSecret string, sessionDays int, isProduction bool) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var req loginRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			apperrors.WriteError(w, r, http.StatusBadRequest, "bad_request", "Invalid request body")
			return
		}

		email := strings.TrimSpace(req.Email)
		password := req.Password
		if email == "" || password == "" {
			writeInvalidCredentials(w, r)
			return
		}

		var userID uuid.UUID
		var dbEmail string
		var passwordHash string

		query := `SELECT id, email, password_hash FROM users WHERE email = $1`
		err := pool.QueryRow(r.Context(), query, email).Scan(&userID, &dbEmail, &passwordHash)
		if err != nil {
			if errors.Is(err, pgx.ErrNoRows) {
				maybeAuditLoginFailed(r, auditor, email)
				writeInvalidCredentials(w, r)
				return
			}

			log.Error().Err(err).Str("email", email).Msg("Failed to query user")
			apperrors.WriteInternalError(w, r, "Login failed")
			return
		}

		if err := VerifyPassword(passwordHash, password); err != nil {
			maybeAuditLoginFailed(r, auditor, email)
			writeInvalidCredentials(w, r)
			return
		}

		token, err := CreateToken(userID, jwtSecret, sessionDays)
		if err != nil {
			log.Error().Err(err).Msg("Failed to create token")
			apperrors.WriteInternalError(w, r, "Failed to create session")
			return
		}

		SetSessionCookie(w, token, sessionDays, isProduction)

		apperrors.WriteSuccess(w, r, http.StatusOK, loginResponse{
			User: userSummary{
				ID:    userID,
				Email: dbEmail,
			},
		})
	}
}

// HandleLogout processes POST /api/v1/auth/logout.
func HandleLogout(isProduction bool) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		ClearSessionCookie(w, isProduction)
		apperrors.WriteSuccess(w, r, http.StatusOK, logoutResponse{LoggedOut: true})
	}
}

func writeInvalidCredentials(w http.ResponseWriter, r *http.Request) {
	apperrors.WriteError(w, r, http.StatusUnauthorized, "invalid_credentials", "Invalid credentials")
}

func maybeAuditLoginFailed(r *http.Request, auditor *audit.Writer, email string) {
	if auditor == nil {
		return
	}

	ip := remoteIP(r.RemoteAddr)
	if !loginFailedAuditLimiter.Allow(ip, time.Now()) {
		return
	}

	if err := auditor.LogLoginFailed(r.Context(), email, ip); err != nil {
		log.Error().Err(err).Str("email", email).Msg("Failed to log login_failed audit event")
	}
}

func remoteIP(remoteAddr string) string {
	host, _, err := net.SplitHostPort(remoteAddr)
	if err == nil && host != "" {
		return host
	}
	return remoteAddr
}

func isValidEmail(email string) bool {
	_, err := mail.ParseAddress(email)
	return err == nil
}
