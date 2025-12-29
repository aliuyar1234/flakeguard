package app

import (
	"github.com/aliuyar1234/flakeguard/internal/apperrors"
	"net/http"

	"github.com/aliuyar1234/flakeguard/internal/apikey"
	"github.com/aliuyar1234/flakeguard/internal/apikeys"
	"github.com/aliuyar1234/flakeguard/internal/audit"
	"github.com/aliuyar1234/flakeguard/internal/auth"
	"github.com/aliuyar1234/flakeguard/internal/config"
	"github.com/aliuyar1234/flakeguard/internal/flake"
	"github.com/aliuyar1234/flakeguard/internal/ingest"
	"github.com/aliuyar1234/flakeguard/internal/orgs"
	"github.com/aliuyar1234/flakeguard/internal/projects"
	"github.com/aliuyar1234/flakeguard/internal/web"
	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"
	"github.com/go-chi/cors"
	"github.com/jackc/pgx/v5/pgxpool"
)

// NewRouter creates and configures the Chi router with all middleware and routes
func NewRouter(pool *pgxpool.Pool, cfg *config.Config) *chi.Mux {
	r := chi.NewRouter()

	isProduction := !cfg.IsDev()

	// Middleware stack
	r.Use(middleware.RealIP)   // Set RemoteAddr to real IP
	r.Use(RequestIDMiddleware) // Add request ID to context
	r.Use(LoggingMiddleware)   // Structured request logging
	r.Use(RecoveryMiddleware)  // Recover from panics
	r.Use(SecurityHeadersMiddleware(isProduction))
	r.Use(cors.Handler(cors.Options{ // CORS (pinned dep)
		AllowedOrigins:   []string{cfg.BaseURL},
		AllowedMethods:   []string{"GET", "POST", "PUT", "DELETE", "OPTIONS"},
		AllowedHeaders:   []string{"Accept", "Authorization", "Content-Type", "X-CSRF-Token"},
		AllowCredentials: true,
		MaxAge:           300,
	}))
	r.Use(auth.AuthMiddleware(cfg.JWTSecret, isProduction)) // Validate session cookies

	// Audit writer (shared across API routes)
	auditor := audit.NewWriter(pool)

	// Health check routes (no authentication required)
	r.Get("/healthz", handleHealthz)
	r.Get("/readyz", handleReadyz(pool))

	// Static assets
	r.Handle("/static/*", http.StripPrefix("/static/", http.FileServer(http.Dir("web/static"))))

	// Public routes - HTML pages
	r.Group(func(r chi.Router) {
		r.Use(NoCacheMiddleware) // Prevent caching of auth pages
		r.Get("/signup", web.HandleSignupPage(isProduction))
		r.Get("/login", web.HandleLoginPage(isProduction))
	})

	// API routes - Authentication
	r.Route("/api/v1/auth", func(r chi.Router) {
		r.Use(ContentTypeJSON)              // Set Content-Type to application/json
		r.Use(CSRFMiddleware(isProduction)) // Validate CSRF tokens

		// Signup (no rate limit for now)
		r.Post("/signup", auth.HandleSignup(pool, auditor))

		// Login with rate limiting (10 requests per minute)
		r.With(LoginRateLimitMiddleware()).Post("/login", auth.HandleLogin(pool, auditor, cfg.JWTSecret, cfg.SessionDays, isProduction))

		// Logout (requires authentication)
		r.With(auth.RequireAuth).Post("/logout", auth.HandleLogout(isProduction))
	})

	// API routes - Organizations (require authentication)
	r.Route("/api/v1/orgs", func(r chi.Router) {
		r.Use(ContentTypeJSON)
		r.Use(CSRFMiddleware(isProduction))
		r.Use(auth.RequireAuth)

		// Organization CRUD
		r.Post("/", orgs.HandleCreate(pool, auditor))
		r.Get("/", orgs.HandleList(pool))

		// Organization members
		r.Get("/{org_id}/members", orgs.HandleListMembers(pool))
		r.Put("/{org_id}/members/{user_id}", orgs.HandleUpdateMemberRole(pool, auditor))
		r.Delete("/{org_id}/members/{user_id}", orgs.HandleRemoveMember(pool, auditor))

		// Organization audit log (OWNER/ADMIN)
		r.Get("/{org_id}/audit", orgs.HandleListAudit(pool))

		// Organization invites
		r.Post("/{org_id}/invites", orgs.HandleCreateInvite(pool, auditor))
		r.Get("/{org_id}/invites", orgs.HandleListInvites(pool))
		r.Delete("/{org_id}/invites/{invite_id}", orgs.HandleRevokeInvite(pool, auditor))
		r.Post("/invites/accept", orgs.HandleAcceptInvite(pool, auditor))

		// Projects under organization
		r.Post("/{org_id}/projects", projects.HandleCreate(pool, auditor))
		r.Get("/{org_id}/projects", projects.HandleList(pool))
	})

	// API routes - Projects (require authentication)
	r.Route("/api/v1/projects", func(r chi.Router) {
		r.Use(ContentTypeJSON)
		r.Use(CSRFMiddleware(isProduction))
		r.Use(auth.RequireAuth)

		// Slack configuration
		r.Put("/{project_id}/slack", projects.HandleConfigureSlack(pool, auditor))
		r.Delete("/{project_id}/slack", projects.HandleRemoveSlack(pool, auditor))

		// API keys
		r.Post("/{project_id}/api-keys", apikeys.HandleCreate(pool, auditor))
		r.Get("/{project_id}/api-keys", apikeys.HandleList(pool))
		r.Delete("/{project_id}/api-keys/{api_key_id}", apikeys.HandleRevoke(pool, auditor))
		r.Post("/{project_id}/api-keys/{api_key_id}/rotate", apikeys.HandleRotate(pool, auditor))

		// Flakes
		r.Get("/{project_id}/flakes", flake.HandleListFlakes(pool))
		r.Get("/{project_id}/flakes/{test_case_id}", flake.HandleGetFlakeDetail(pool))
	})

	// API routes - Ingestion (require API key authentication)
	r.Route("/api/v1/ingest", func(r chi.Router) {
		// Upload limits from config
		uploadLimits := ingest.NewUploadLimits(cfg.MaxUploadFiles, cfg.MaxFileBytes, cfg.MaxUploadBytes)

		// JUnit upload endpoint with API key auth and rate limiting
		r.With(
			apikey.RequireAPIKey(pool, apikeys.ScopeIngestWrite),
			apikey.RateLimitByAPIKey(cfg.RateLimitRPM),
		).Post("/junit", ingest.HandleJUnitUpload(pool, cfg, uploadLimits))
	})

	// Protected routes - require authentication
	r.Group(func(r chi.Router) {
		r.Use(auth.RequireAuthPage)
		r.Use(NoCacheMiddleware)

		// Organizations
		r.Get("/orgs", web.HandleOrgsPage(pool, isProduction))
		r.Get("/orgs/new", web.HandleOrgCreatePage(isProduction))
		r.Get("/orgs/{org_id}/settings", web.HandleOrgSettingsPage(pool, isProduction))

		// Projects
		r.Get("/orgs/{org_id}/projects", web.HandleProjectsPage(pool, isProduction))
		r.Get("/orgs/{org_id}/projects/new", web.HandleProjectCreatePage(pool, isProduction))
		r.Get("/orgs/{org_id}/projects/{project_id}/settings", web.HandleProjectSettingsPage(pool, isProduction))

		// Org invites
		r.Get("/invites/accept", web.HandleInviteAcceptPage(isProduction))

		// Flakes Dashboard (using slug-based URLs)
		r.Get("/orgs/{org_slug}/projects/{project_slug}/flakes", web.HandleFlakesListPage(pool, isProduction))
		r.Get("/orgs/{org_slug}/projects/{project_slug}/flakes/{test_case_id}", web.HandleFlakeDetailPage(pool, isProduction))
	})

	return r
}

// handleHealthz returns a simple liveness check
// Always returns 200 OK if the service is running
func handleHealthz(w http.ResponseWriter, r *http.Request) {
	apperrors.WriteSuccess(w, r, http.StatusOK, map[string]string{
		"status": "ok",
	})
}

// handleReadyz returns a readiness check that includes database connectivity
// Returns 200 OK if service is ready to accept traffic, 503 if not
func handleReadyz(pool *pgxpool.Pool) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		// Check database connectivity
		if err := pool.Ping(r.Context()); err != nil {
			apperrors.WriteServiceUnavailable(w, r, "Database connection failed")
			return
		}

		apperrors.WriteSuccess(w, r, http.StatusOK, map[string]string{
			"status": "ready",
			"db":     "ok",
		})
	}
}
