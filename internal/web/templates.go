package web

import (
	"html/template"
	"net/http"
	"path/filepath"

	"github.com/google/uuid"
	"github.com/rs/zerolog/log"
)

// TemplateData holds common data passed to all templates
type TemplateData struct {
	Title           string
	UserID          uuid.UUID
	IsAuthenticated bool
	CSRFToken       string
	Next            string
	Redirect        string
	Error           string
	Success         string
	Data            interface{}
}

// templates is the global template cache
var templates map[string]*template.Template

// InitTemplates parses and caches all templates
func InitTemplates(templatesDir string) error {
	templates = make(map[string]*template.Template)

	// Parse layout
	layoutPath := filepath.Join(templatesDir, "layout.html")

	// Parse each page template with layout
	pages := []string{
		"signup.html",
		"login.html",
		"org_list.html",
		"org_create.html",
		"org_settings.html",
		"project_list.html",
		"project_create.html",
		"project_settings.html",
		"invite_accept.html",
		"flakes_list.html",
		"flake_detail.html",
	}

	for _, page := range pages {
		pagePath := filepath.Join(templatesDir, page)
		tmpl, err := template.ParseFiles(layoutPath, pagePath)
		if err != nil {
			return err
		}
		templates[page] = tmpl
	}

	log.Info().Int("count", len(templates)).Msg("Templates initialized")
	return nil
}

// RenderTemplate renders a template with the given data
func RenderTemplate(w http.ResponseWriter, r *http.Request, name string, data *TemplateData) {
	tmpl, ok := templates[name]
	if !ok {
		log.Error().Str("template", name).Msg("Template not found")
		http.Error(w, "Internal Server Error", http.StatusInternalServerError)
		return
	}

	// Set cache control headers for HTML pages
	w.Header().Set("Cache-Control", "no-store")
	w.Header().Set("Content-Type", "text/html; charset=utf-8")

	if err := tmpl.Execute(w, data); err != nil {
		log.Error().Err(err).Str("template", name).Msg("Failed to render template")
		http.Error(w, "Internal Server Error", http.StatusInternalServerError)
		return
	}
}
