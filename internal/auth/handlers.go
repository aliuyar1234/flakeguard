package auth

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/mail"
	"strings"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/rs/zerolog/log"
)

// ErrorResponse represents the error envelope
type ErrorResponse struct {
	Error ErrorDetail `json:"error"`
}

// ErrorDetail contains error information
type ErrorDetail struct {
	Code    string `json:"code"`
	Message string `json:"message"`
}

// SuccessResponse represents the success envelope
type SuccessResponse struct {
	Data interface{} `json:"data"`
}

// writeError writes a JSON error response
func writeError(w http.ResponseWriter, statusCode int, code, message string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(statusCode)
	json.NewEncoder(w).Encode(ErrorResponse{
		Error: ErrorDetail{
			Code:    code,
			Message: message,
		},
	})
}

// writeSuccess writes a JSON success response
func writeSuccess(w http.ResponseWriter, statusCode int, data interface{}) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(statusCode)
	json.NewEncoder(w).Encode(SuccessResponse{
		Data: data,
	})
}

// SignupRequest represents the signup request payload
type SignupRequest struct {
	Email    string `json:"email"`
	Password string `json:"password"`
}

// SignupResponse represents the signup response
type SignupResponse struct {
	UserID uuid.UUID `json:"user_id"`
	Email  string    `json:"email"`
}

// HandleSignup processes user registration
func HandleSignup(pool *pgxpool.Pool, jwtSecret string, sessionDays int, isProduction bool) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		// Parse form data
		if err := r.ParseForm(); err != nil {
			writeError(w, http.StatusBadRequest, "bad_request", "Invalid form data")
			return
		}

		email := strings.TrimSpace(r.FormValue("email"))
		password := r.FormValue("password")

		// Validate email format (RFC 5322 simplified)
		if !isValidEmail(email) {
			writeError(w, http.StatusBadRequest, "bad_request", "Invalid email address")
			return
		}

		// Validate password length (minimum 8 characters)
		if len(password) < 8 {
			writeError(w, http.StatusBadRequest, "bad_request", "Password must be at least 8 characters")
			return
		}

		// Hash password
		passwordHash, err := HashPassword(password)
		if err != nil {
			log.Error().Err(err).Msg("Failed to hash password")
			writeError(w, http.StatusInternalServerError, "internal_error", "Failed to create account")
			return
		}

		// Insert user into database
		userID := uuid.New()
		query := `
			INSERT INTO users (id, email, password_hash)
			VALUES ($1, $2, $3)
		`

		_, err = pool.Exec(r.Context(), query, userID, email, passwordHash)
		if err != nil {
			// Check for unique constraint violation
			var pgErr *pgconn.PgError
			if errors.As(err, &pgErr) && pgErr.Code == "23505" { // unique_violation
				writeError(w, http.StatusConflict, "conflict", "Email address already registered")
				return
			}

			log.Error().Err(err).Str("email", email).Msg("Failed to insert user")
			writeError(w, http.StatusInternalServerError, "internal_error", "Failed to create account")
			return
		}

		// Create audit log entry
		if err := createAuditLog(r.Context(), pool, userID, "user.signup", email); err != nil {
			log.Error().Err(err).Str("user_id", userID.String()).Msg("Failed to create audit log")
			// Don't fail the signup if audit log fails
		}

		// Create JWT token
		token, err := CreateToken(userID, jwtSecret, sessionDays)
		if err != nil {
			log.Error().Err(err).Msg("Failed to create token")
			writeError(w, http.StatusInternalServerError, "internal_error", "Failed to create session")
			return
		}

		// Set session cookie
		SetSessionCookie(w, token, sessionDays, isProduction)

		log.Info().
			Str("user_id", userID.String()).
			Str("email", email).
			Msg("User signed up successfully")

		// Return success response
		writeSuccess(w, http.StatusCreated, SignupResponse{
			UserID: userID,
			Email:  email,
		})
	}
}

// LoginRequest represents the login request payload
type LoginRequest struct {
	Email    string `json:"email"`
	Password string `json:"password"`
}

// LoginResponse represents the login response
type LoginResponse struct {
	UserID uuid.UUID `json:"user_id"`
	Email  string    `json:"email"`
}

// HandleLogin processes user authentication
func HandleLogin(pool *pgxpool.Pool, jwtSecret string, sessionDays int, isProduction bool) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		// Parse form data
		if err := r.ParseForm(); err != nil {
			writeError(w, http.StatusBadRequest, "bad_request", "Invalid form data")
			return
		}

		email := strings.TrimSpace(r.FormValue("email"))
		password := r.FormValue("password")

		// Validate input
		if email == "" || password == "" {
			writeError(w, http.StatusUnauthorized, "unauthorized", "Invalid credentials")
			return
		}

		// Look up user by email
		var userID uuid.UUID
		var passwordHash string
		query := `SELECT id, password_hash FROM users WHERE email = $1`

		err := pool.QueryRow(r.Context(), query, email).Scan(&userID, &passwordHash)
		if err != nil {
			if err == pgx.ErrNoRows {
				// User not found - return generic error
				log.Debug().Str("email", email).Msg("Login failed: user not found")
				writeError(w, http.StatusUnauthorized, "unauthorized", "Invalid credentials")
				return
			}
			log.Error().Err(err).Str("email", email).Msg("Failed to query user")
			writeError(w, http.StatusInternalServerError, "internal_error", "Login failed")
			return
		}

		// Verify password
		if err := VerifyPassword(passwordHash, password); err != nil {
			// Wrong password - return generic error
			log.Debug().Str("email", email).Msg("Login failed: wrong password")
			writeError(w, http.StatusUnauthorized, "unauthorized", "Invalid credentials")
			return
		}

		// Create JWT token
		token, err := CreateToken(userID, jwtSecret, sessionDays)
		if err != nil {
			log.Error().Err(err).Msg("Failed to create token")
			writeError(w, http.StatusInternalServerError, "internal_error", "Failed to create session")
			return
		}

		// Set session cookie
		SetSessionCookie(w, token, sessionDays, isProduction)

		log.Info().
			Str("user_id", userID.String()).
			Str("email", email).
			Msg("User logged in successfully")

		// Return success response
		writeSuccess(w, http.StatusOK, LoginResponse{
			UserID: userID,
			Email:  email,
		})
	}
}

// HandleLogout processes user logout
func HandleLogout(w http.ResponseWriter, r *http.Request) {
	// Clear session cookie
	ClearSessionCookie(w)

	// Get user ID from context for logging
	userID := GetUserID(r.Context())
	if userID != uuid.Nil {
		log.Info().Str("user_id", userID.String()).Msg("User logged out")
	}

	// Redirect to login page
	http.Redirect(w, r, "/login", http.StatusSeeOther)
}

// isValidEmail validates email format using net/mail (RFC 5322 simplified)
func isValidEmail(email string) bool {
	_, err := mail.ParseAddress(email)
	return err == nil
}

// createAuditLog creates an audit log entry
func createAuditLog(ctx context.Context, pool *pgxpool.Pool, userID uuid.UUID, event, details string) error {
	query := `
		INSERT INTO audit_logs (id, user_id, event, details)
		VALUES ($1, $2, $3, $4)
	`
	_, err := pool.Exec(ctx, query, uuid.New(), userID, event, details)
	return err
}
