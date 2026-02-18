package app

import (
	"encoding/json"
	"fmt"
	"html/template"
	"net/http/httptest"
	"strings"
	"testing"
	"testing/fstest"
	"time"
)

// testFS returns an in-memory filesystem suitable for testing template rendering.
// It mirrors the expected web/templates/ directory layout with a base layout,
// a partial, and two page templates (user and error).
func testFS() fstest.MapFS {
	return fstest.MapFS{
		"templates/layouts/base.html": &fstest.MapFile{
			Data: []byte(
				`{{ define "base" }}<!DOCTYPE html><html>` +
					`<head><title>{{ block "title" . }}Default{{ end }}</title></head>` +
					`<body>{{ block "nav" . }}{{ end }}{{ block "content" . }}{{ end }}</body>` +
					`</html>{{ end }}`),
		},
		"templates/partials/nav.html": &fstest.MapFile{
			Data: []byte(`{{ define "nav" }}<nav>Navigation</nav>{{ end }}`),
		},
		"templates/user/list.html": &fstest.MapFile{
			Data: []byte(
				`{{ template "base" . }}` +
					`{{ define "title" }}Users{{ end }}` +
					`{{ define "content" }}<h1>User List</h1>{{ template "nav" . }}{{ end }}`),
		},
		"templates/errors/404.html": &fstest.MapFile{
			Data: []byte(
				`{{ template "base" . }}` +
					`{{ define "title" }}Not Found{{ end }}` +
					`{{ define "content" }}<h1>404 Not Found</h1>{{ end }}`),
		},
	}
}

// testFSWithFuncs returns a test filesystem that uses template functions
// (formatDate, safeHTML, add, sub, seq) in the page template.
func testFSWithFuncs() fstest.MapFS {
	base := testFS()
	base["templates/functest/page.html"] = &fstest.MapFile{
		Data: []byte(
			`{{ template "base" . }}` +
				`{{ define "content" }}` +
				`date:{{ formatDate .Date }}|` +
				`safe:{{ dangerouslySetInnerHTML .HTML }}|` +
				`add:{{ add 3 4 }}|` +
				`sub:{{ sub 10 3 }}|` +
				`seq:{{ range seq 1 3 }}{{ . }}{{ end }}` +
				`{{ end }}`),
	}
	return base
}

// ---------------------------------------------------------------------------
// Template function tests
// ---------------------------------------------------------------------------

func TestTemplateFuncMap(t *testing.T) {
	fm := templateFuncMap()

	t.Run("json_string", func(t *testing.T) {
		fn := fm["json"].(func(any) template.JS)
		got := fn("操作成功")
		want := template.JS(`"操作成功"`)
		if got != want {
			t.Errorf("json(string) = %q; want %q", got, want)
		}
	})

	t.Run("json_string_with_special_chars", func(t *testing.T) {
		fn := fm["json"].(func(any) template.JS)
		got := fn(`He said "hello" & 'bye'`)
		// json.Marshal produces a valid JS string literal with quotes escaped.
		var roundtrip string
		if err := json.Unmarshal([]byte(got), &roundtrip); err != nil {
			t.Fatalf("json output %q is not valid JSON: %v", got, err)
		}
		if roundtrip != `He said "hello" & 'bye'` {
			t.Errorf("round-tripped value = %q; want original string", roundtrip)
		}
	})

	t.Run("json_nil_returns_null", func(t *testing.T) {
		fn := fm["json"].(func(any) template.JS)
		got := fn(nil)
		if got != "null" {
			t.Errorf("json(nil) = %q; want %q", got, "null")
		}
	})

	t.Run("formatDate", func(t *testing.T) {
		fn := fm["formatDate"].(func(time.Time) string)
		d := time.Date(2024, 3, 15, 14, 30, 0, 0, time.UTC)
		got := fn(d)
		want := "2024-03-15 14:30:00"
		if got != want {
			t.Errorf("formatDate() = %q; want %q", got, want)
		}
	})

	t.Run("dangerouslySetInnerHTML", func(t *testing.T) {
		fn := fm["dangerouslySetInnerHTML"].(func(string) template.HTML)
		got := fn("<b>bold</b>")
		want := template.HTML("<b>bold</b>")
		if got != want {
			t.Errorf("dangerouslySetInnerHTML() = %q; want %q", got, want)
		}
	})

	t.Run("add", func(t *testing.T) {
		fn := fm["add"].(func(int, int) int)
		if got := fn(3, 4); got != 7 {
			t.Errorf("add(3,4) = %d; want 7", got)
		}
	})

	t.Run("sub", func(t *testing.T) {
		fn := fm["sub"].(func(int, int) int)
		if got := fn(10, 3); got != 7 {
			t.Errorf("sub(10,3) = %d; want 7", got)
		}
	})

	t.Run("seq", func(t *testing.T) {
		fn := fm["seq"].(func(int, int) []int)

		got := fn(1, 5)
		want := []int{1, 2, 3, 4, 5}
		if len(got) != len(want) {
			t.Fatalf("seq(1,5) len = %d; want %d", len(got), len(want))
		}
		for i, v := range got {
			if v != want[i] {
				t.Errorf("seq(1,5)[%d] = %d; want %d", i, v, want[i])
			}
		}

		if got := fn(5, 1); got != nil {
			t.Errorf("seq(5,1) = %v; want nil", got)
		}
	})
}

// ---------------------------------------------------------------------------
// NewTemplateRenderer tests
// ---------------------------------------------------------------------------

func TestNewTemplateRenderer_Release(t *testing.T) {
	r, err := NewTemplateRenderer(testFS(), false)
	if err != nil {
		t.Fatalf("NewTemplateRenderer() error: %v", err)
	}
	if r.debug {
		t.Error("expected debug=false")
	}
	if r.templates == nil {
		t.Fatal("templates map should be initialized in release mode")
	}
	if _, ok := r.templates["user/list.html"]; !ok {
		t.Error("expected template 'user/list.html' to be loaded")
	}
	if _, ok := r.templates["errors/404.html"]; !ok {
		t.Error("expected template 'errors/404.html' to be loaded")
	}
}

func TestNewTemplateRenderer_Debug(t *testing.T) {
	r, err := NewTemplateRenderer(testFS(), true)
	if err != nil {
		t.Fatalf("NewTemplateRenderer() error: %v", err)
	}
	if !r.debug {
		t.Error("expected debug=true")
	}
	if r.templates != nil {
		t.Error("templates should be nil in debug mode (parsed on each request)")
	}
}

func TestNewTemplateRenderer_InvalidTemplate(t *testing.T) {
	badFS := fstest.MapFS{
		"templates/layouts/base.html": &fstest.MapFile{
			Data: []byte(`{{ define "base" }}{{ end }}`),
		},
		"templates/bad/page.html": &fstest.MapFile{
			Data: []byte(`{{ invalid_syntax `),
		},
	}
	_, err := NewTemplateRenderer(badFS, false)
	if err == nil {
		t.Fatal("expected error for invalid template syntax")
	}
}

// ---------------------------------------------------------------------------
// Instance + Render tests
// ---------------------------------------------------------------------------

func TestTemplateRenderer_Instance_Release(t *testing.T) {
	r, err := NewTemplateRenderer(testFS(), false)
	if err != nil {
		t.Fatalf("NewTemplateRenderer() error: %v", err)
	}

	inst := r.Instance("user/list.html", nil)
	w := httptest.NewRecorder()
	if err := inst.Render(w); err != nil {
		t.Fatalf("Render() error: %v", err)
	}

	body := w.Body.String()
	for _, want := range []string{
		"<title>Users</title>",
		"<h1>User List</h1>",
		"<nav>Navigation</nav>",
		"<!DOCTYPE html>",
	} {
		if !strings.Contains(body, want) {
			t.Errorf("body missing %q:\n%s", want, body)
		}
	}
}

func TestTemplateRenderer_Instance_Debug(t *testing.T) {
	r, err := NewTemplateRenderer(testFS(), true)
	if err != nil {
		t.Fatalf("NewTemplateRenderer() error: %v", err)
	}

	inst := r.Instance("errors/404.html", nil)
	w := httptest.NewRecorder()
	if err := inst.Render(w); err != nil {
		t.Fatalf("Render() error: %v", err)
	}

	body := w.Body.String()
	for _, want := range []string{
		"<title>Not Found</title>",
		"<h1>404 Not Found</h1>",
	} {
		if !strings.Contains(body, want) {
			t.Errorf("body missing %q:\n%s", want, body)
		}
	}
}

func TestTemplateRenderer_Instance_NotFound(t *testing.T) {
	r, err := NewTemplateRenderer(testFS(), false)
	if err != nil {
		t.Fatalf("NewTemplateRenderer() error: %v", err)
	}

	inst := r.Instance("nonexistent.html", nil)
	w := httptest.NewRecorder()
	if err := inst.Render(w); err == nil {
		t.Error("Render() should return error for nonexistent template")
	}
}

func TestTemplateRenderer_Instance_WithFuncMap(t *testing.T) {
	r, err := NewTemplateRenderer(testFSWithFuncs(), false)
	if err != nil {
		t.Fatalf("NewTemplateRenderer() error: %v", err)
	}

	data := map[string]any{
		"Date": time.Date(2024, 6, 15, 10, 30, 0, 0, time.UTC),
		"HTML": "<em>hello</em>",
	}
	inst := r.Instance("functest/page.html", data)
	w := httptest.NewRecorder()
	if err := inst.Render(w); err != nil {
		t.Fatalf("Render() error: %v", err)
	}

	body := w.Body.String()
	for _, want := range []string{
		"date:2024-06-15 10:30:00",
		"safe:<em>hello</em>",
		"add:7",
		"sub:7",
		"seq:123",
	} {
		if !strings.Contains(body, want) {
			t.Errorf("body missing %q:\n%s", want, body)
		}
	}
}

// ---------------------------------------------------------------------------
// HTMLInstance tests
// ---------------------------------------------------------------------------

func TestHTMLInstance_WriteContentType(t *testing.T) {
	w := httptest.NewRecorder()
	h := &HTMLInstance{}
	h.WriteContentType(w)

	got := w.Header().Get("Content-Type")
	want := "text/html; charset=utf-8"
	if got != want {
		t.Errorf("Content-Type = %q; want %q", got, want)
	}
}

func TestHTMLInstance_WriteContentType_NoOverwrite(t *testing.T) {
	w := httptest.NewRecorder()
	w.Header().Set("Content-Type", "application/json")

	h := &HTMLInstance{}
	h.WriteContentType(w)

	got := w.Header().Get("Content-Type")
	if got != "application/json" {
		t.Errorf("Content-Type should not be overwritten; got %q", got)
	}
}

func TestHTMLInstance_Render_ParseError(t *testing.T) {
	w := httptest.NewRecorder()
	h := &HTMLInstance{err: fmt.Errorf("parse error")}

	err := h.Render(w)
	if err == nil {
		t.Fatal("expected error from Render")
	}
	if !strings.Contains(err.Error(), "parse error") {
		t.Errorf("error = %q; want to contain 'parse error'", err.Error())
	}
}

// ---------------------------------------------------------------------------
// discoverPageTemplates test
// ---------------------------------------------------------------------------

func TestDiscoverPageTemplates(t *testing.T) {
	r := &TemplateRenderer{fs: testFS()}

	pages, err := r.discoverPageTemplates()
	if err != nil {
		t.Fatalf("discoverPageTemplates() error: %v", err)
	}

	// Should find user/list.html and errors/404.html but not layouts or partials.
	wantPaths := map[string]bool{
		"templates/user/list.html":  false,
		"templates/errors/404.html": false,
	}
	for _, p := range pages {
		if _, ok := wantPaths[p]; ok {
			wantPaths[p] = true
		}
	}
	for p, found := range wantPaths {
		if !found {
			t.Errorf("expected page %q to be discovered", p)
		}
	}

	// Layouts and partials must NOT appear.
	for _, p := range pages {
		rel := strings.TrimPrefix(p, "templates/")
		if strings.HasPrefix(rel, "layouts/") || strings.HasPrefix(rel, "partials/") {
			t.Errorf("base template %q should not be in page list", p)
		}
	}
}
