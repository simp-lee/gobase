package app

import (
	"encoding/json"
	"fmt"
	"html/template"
	"io/fs"
	"net/http"
	"strings"
	"time"

	"github.com/gin-gonic/gin/render"
)

// TemplateRenderer is a custom Gin HTML renderer that supports layout + partial
// template inheritance and dual-mode operation (debug / release).
//
// In debug mode, templates are re-parsed from the filesystem on every request,
// enabling instant hot-reload during development. In release mode, templates are
// parsed once at startup and served from memory for maximum performance.
//
// Template loading strategy:
//  1. Load all layout templates   (templates/layouts/*.html)
//  2. Load all partial templates  (templates/partials/*.html)
//  3. For each page template, clone the base set (layouts + partials) and parse
//     the page template on top, allowing the page to override blocks defined in layouts.
//
// Page templates use {{ template "base" . }} to invoke the layout, and define
// blocks ({{ define "title" }}, {{ define "content" }}, etc.) to inject content
// into the layout's block slots.
type TemplateRenderer struct {
	templates map[string]*template.Template // page name -> compiled template set (release mode only)
	fs        fs.FS                         // filesystem containing templates/ directory
	funcMap   template.FuncMap
	debug     bool
}

// Compile-time check: TemplateRenderer implements render.HTMLRender.
var _ render.HTMLRender = (*TemplateRenderer)(nil)

// NewTemplateRenderer creates a TemplateRenderer backed by the given filesystem.
//
// Parameters:
//   - fsys: filesystem to read templates from. Use os.DirFS("web") for debug mode
//     (hot reload from disk) or web.EmbeddedFS for release mode (embedded binary).
//   - debug: when true, templates are re-parsed on every request for hot reload.
//
// The filesystem must contain a templates/ directory structured as:
//
//	templates/
//	  layouts/   – layout templates defining the page skeleton (e.g., base.html)
//	  partials/  – reusable partial templates (e.g., nav, footer)
//	  <module>/  – page templates organized by module (e.g., user/, errors/)
func NewTemplateRenderer(fsys fs.FS, debug bool) (*TemplateRenderer, error) {
	r := &TemplateRenderer{
		fs:      fsys,
		funcMap: templateFuncMap(),
		debug:   debug,
	}

	if !debug {
		templates, err := r.parseAllTemplates()
		if err != nil {
			return nil, fmt.Errorf("parse templates: %w", err)
		}
		r.templates = templates
	}

	return r, nil
}

// Instance returns a render.Render that executes the named page template with data.
// The name should be the page template path relative to templates/, for example
// "user/list.html" or "errors/404.html".
//
// This implements the render.HTMLRender interface required by Gin.
func (r *TemplateRenderer) Instance(name string, data any) render.Render {
	if r.debug {
		// Re-parse all templates on every request for hot reload.
		templates, err := r.parseAllTemplates()
		if err != nil {
			return &HTMLInstance{err: err}
		}
		return &HTMLInstance{
			Template: templates[name],
			Name:     name,
			Data:     data,
		}
	}

	return &HTMLInstance{
		Template: r.templates[name],
		Name:     name,
		Data:     data,
	}
}

// parseAllTemplates walks the templates directory, builds a base template set from
// layouts and partials, then creates a separate compiled template for each page by
// cloning the base and parsing the page on top.
//
// Returns a map from page name (e.g., "user/list.html") to its compiled template.
func (r *TemplateRenderer) parseAllTemplates() (map[string]*template.Template, error) {
	// Step 1: Collect layout and partial file paths.
	layoutFiles, err := fs.Glob(r.fs, "templates/layouts/*.html")
	if err != nil {
		return nil, fmt.Errorf("glob layouts: %w", err)
	}
	partialFiles, err := fs.Glob(r.fs, "templates/partials/*.html")
	if err != nil {
		return nil, fmt.Errorf("glob partials: %w", err)
	}

	// Step 2: Build the base template set from layouts + partials.
	base := template.New("").Funcs(r.funcMap)
	baseFiles := append(layoutFiles, partialFiles...)
	for _, f := range baseFiles {
		content, err := fs.ReadFile(r.fs, f)
		if err != nil {
			return nil, fmt.Errorf("read %s: %w", f, err)
		}
		if _, err := base.New(f).Parse(string(content)); err != nil {
			return nil, fmt.Errorf("parse %s: %w", f, err)
		}
	}

	// Step 3: Discover page templates (everything not in layouts/ or partials/).
	pageFiles, err := r.discoverPageTemplates()
	if err != nil {
		return nil, fmt.Errorf("discover pages: %w", err)
	}

	// Step 4: For each page, clone base and parse the page template on top.
	templates := make(map[string]*template.Template, len(pageFiles))
	for _, pf := range pageFiles {
		clone, err := base.Clone()
		if err != nil {
			return nil, fmt.Errorf("clone base for %s: %w", pf, err)
		}
		content, err := fs.ReadFile(r.fs, pf)
		if err != nil {
			return nil, fmt.Errorf("read %s: %w", pf, err)
		}
		// Page name is relative to templates/, e.g., "user/list.html".
		name := strings.TrimPrefix(pf, "templates/")
		if _, err := clone.New(name).Parse(string(content)); err != nil {
			return nil, fmt.Errorf("parse %s: %w", pf, err)
		}
		templates[name] = clone
	}

	return templates, nil
}

// discoverPageTemplates finds all .html files under templates/ that are not in
// the layouts/ or partials/ subdirectories.
func (r *TemplateRenderer) discoverPageTemplates() ([]string, error) {
	var pages []string
	err := fs.WalkDir(r.fs, "templates", func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() || !strings.HasSuffix(path, ".html") {
			return nil
		}
		// Skip layouts and partials; they form the base template set.
		rel := strings.TrimPrefix(path, "templates/")
		if strings.HasPrefix(rel, "layouts/") || strings.HasPrefix(rel, "partials/") {
			return nil
		}
		pages = append(pages, path)
		return nil
	})
	return pages, err
}

// templateFuncMap returns the default set of template helper functions.
func templateFuncMap() template.FuncMap {
	return template.FuncMap{
		// json marshals v to a JSON string and returns it as template.JS so it
		// can be safely embedded in JavaScript contexts (e.g., Alpine.js x-init)
		// without html/template re-escaping the output.
		"json": func(v any) template.JS {
			b, err := json.Marshal(v)
			if err != nil {
				return template.JS("null")
			}
			return template.JS(b)
		},

		// formatDate formats a time.Time value as "YYYY-MM-DD HH:MM:SS".
		"formatDate": func(t time.Time) string {
			return t.Format("2006-01-02 15:04:05")
		},

		// dangerouslySetInnerHTML marks a string as safe HTML, bypassing
		// html/template's auto-escaping. WARNING: This function MUST NEVER be
		// used with user-supplied or untrusted data — doing so creates an XSS
		// vulnerability. Only use it for trusted, developer-controlled HTML
		// fragments (e.g., pre-rendered markdown from a build step).
		"dangerouslySetInnerHTML": func(s string) template.HTML {
			return template.HTML(s)
		},

		// add returns the sum of two integers (useful for pagination: page + 1).
		"add": func(a, b int) int {
			return a + b
		},

		// sub returns the difference of two integers (useful for pagination: page - 1).
		"sub": func(a, b int) int {
			return a - b
		},

		// seq generates a slice of integers from start to end inclusive
		// (useful for pagination page number links).
		"seq": func(start, end int) []int {
			if start > end {
				return nil
			}
			s := make([]int, 0, end-start+1)
			for i := start; i <= end; i++ {
				s = append(s, i)
			}
			return s
		},
	}
}

// HTMLInstance implements gin's render.Render interface for a single template
// execution. It is returned by TemplateRenderer.Instance.
type HTMLInstance struct {
	Template *template.Template
	Name     string
	Data     any
	err      error // set when template parsing failed (debug mode)
}

const htmlContentType = "text/html; charset=utf-8"

// Render writes the template output to the HTTP response writer.
func (h *HTMLInstance) Render(w http.ResponseWriter) error {
	h.WriteContentType(w)
	if h.err != nil {
		return h.err
	}
	if h.Template == nil {
		return fmt.Errorf("template %q not found", h.Name)
	}
	return h.Template.ExecuteTemplate(w, h.Name, h.Data)
}

// WriteContentType sets the Content-Type header to text/html; charset=utf-8
// if it has not already been set.
func (h *HTMLInstance) WriteContentType(w http.ResponseWriter) {
	header := w.Header()
	if val := header["Content-Type"]; len(val) == 0 {
		header["Content-Type"] = []string{htmlContentType}
	}
}
