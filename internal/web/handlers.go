package web

import (
	"net/http"
	"net/url"
	"strings"

	"github.com/aliuyar1234/flakeguard/internal/auth"
	"github.com/aliuyar1234/flakeguard/internal/orgs"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/rs/zerolog/log"
)

func sanitizeNext(next string) string {
	next = strings.TrimSpace(next)
	if next == "" {
		return ""
	}
	if !strings.HasPrefix(next, "/") || strings.HasPrefix(next, "//") {
		return ""
	}
	if strings.ContainsAny(next, "\r\n") {
		return ""
	}
	return next
}

// HandleSignupPage renders the signup page
func HandleSignupPage(isProduction bool) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		next := sanitizeNext(r.URL.Query().Get("next"))

		// Check if user is already logged in
		userID := auth.GetUserID(r.Context())
		if userID != uuid.Nil {
			// Already logged in - redirect to organizations
			redirect := "/orgs"
			if next != "" {
				redirect = next
			}
			http.Redirect(w, r, redirect, http.StatusSeeOther)
			return
		}

		// Generate CSRF token
		csrfToken, err := auth.GenerateCSRFToken()
		if err != nil {
			http.Error(w, "Internal Server Error", http.StatusInternalServerError)
			return
		}

		// Set CSRF cookie
		auth.SetCSRFCookie(w, csrfToken, isProduction)

		redirect := "/login"
		if next != "" {
			redirect = "/login?next=" + url.QueryEscape(next)
		}

		// Render signup page
		data := &TemplateData{
			Title:           "Sign Up",
			IsAuthenticated: false,
			CSRFToken:       csrfToken,
			Next:            next,
			Redirect:        redirect,
		}
		RenderTemplate(w, r, "signup.html", data)
	}
}

// HandleLoginPage renders the login page
func HandleLoginPage(isProduction bool) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		next := sanitizeNext(r.URL.Query().Get("next"))

		redirect := "/orgs"
		if next != "" {
			redirect = next
		}

		// Check if user is already logged in
		userID := auth.GetUserID(r.Context())
		if userID != uuid.Nil {
			// Already logged in - redirect to organizations
			http.Redirect(w, r, redirect, http.StatusSeeOther)
			return
		}

		// Generate CSRF token
		csrfToken, err := auth.GenerateCSRFToken()
		if err != nil {
			http.Error(w, "Internal Server Error", http.StatusInternalServerError)
			return
		}

		// Set CSRF cookie
		auth.SetCSRFCookie(w, csrfToken, isProduction)

		// Render login page
		data := &TemplateData{
			Title:           "Log In",
			IsAuthenticated: false,
			CSRFToken:       csrfToken,
			Next:            next,
			Redirect:        redirect,
		}
		RenderTemplate(w, r, "login.html", data)
	}
}

// HandleOrgsPage renders the organizations list page
func HandleOrgsPage(pool *pgxpool.Pool, isProduction bool) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		ctx := r.Context()
		userID := auth.GetUserID(ctx)

		// Generate CSRF token
		csrfToken, err := auth.GenerateCSRFToken()
		if err != nil {
			http.Error(w, "Internal Server Error", http.StatusInternalServerError)
			return
		}

		// Set CSRF cookie
		auth.SetCSRFCookie(w, csrfToken, isProduction)

		// Get user's organizations
		service := orgs.NewService(pool)
		orgsList, err := service.ListUserOrgs(ctx, userID)
		if err != nil {
			log.Error().Err(err).Msg("Failed to list organizations")
			data := &TemplateData{
				Title:           "Organizations",
				UserID:          userID,
				IsAuthenticated: true,
				CSRFToken:       csrfToken,
				Error:           "Failed to load organizations",
			}
			RenderTemplate(w, r, "org_list.html", data)
			return
		}

		// Render organizations page
		data := &TemplateData{
			Title:           "Organizations",
			UserID:          userID,
			IsAuthenticated: true,
			CSRFToken:       csrfToken,
			Data: map[string]interface{}{
				"Orgs": orgsList,
			},
		}
		RenderTemplate(w, r, "org_list.html", data)
	}
}

// HandleOrgCreatePage renders the organization creation page
func HandleOrgCreatePage(isProduction bool) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		userID := auth.GetUserID(r.Context())

		// Generate CSRF token
		csrfToken, err := auth.GenerateCSRFToken()
		if err != nil {
			http.Error(w, "Internal Server Error", http.StatusInternalServerError)
			return
		}

		// Set CSRF cookie
		auth.SetCSRFCookie(w, csrfToken, isProduction)

		// Render org create page
		data := &TemplateData{
			Title:           "Create Organization",
			UserID:          userID,
			IsAuthenticated: true,
			CSRFToken:       csrfToken,
		}
		RenderTemplate(w, r, "org_create.html", data)
	}
}
