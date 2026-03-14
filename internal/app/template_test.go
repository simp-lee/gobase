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

	"github.com/simp-lee/pagination"

	"github.com/simp-lee/gobase/web"
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
				`subtract:{{ subtract 10 3 }}|` +
				`percent:{{ percent 50 200 }}|` +
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

	t.Run("dangerouslySetInnerHTML_safe_tag", func(t *testing.T) {
		fn := fm["dangerouslySetInnerHTML"].(func(string) template.HTML)
		got := fn("<b>bold</b>")
		want := template.HTML("<b>bold</b>")
		if got != want {
			t.Errorf("dangerouslySetInnerHTML() = %q; want %q", got, want)
		}
	})

	t.Run("dangerouslySetInnerHTML_strips_script", func(t *testing.T) {
		fn := fm["dangerouslySetInnerHTML"].(func(string) template.HTML)
		got := fn(`<p>hello</p><script>alert("xss")</script>`)
		if strings.Contains(string(got), "<script") {
			t.Errorf("dangerouslySetInnerHTML should strip <script> tags, got %q", got)
		}
		if !strings.Contains(string(got), "<p>hello</p>") {
			t.Errorf("dangerouslySetInnerHTML should preserve safe tags, got %q", got)
		}
	})

	t.Run("urlWithQuery", func(t *testing.T) {
		fn := fm["urlWithQuery"].(func(string, string) template.URL)
		got := fn("/users", "page=2&page_size=1&sort=status%3Aasc&status=active")
		want := template.URL("/users?page=2&page_size=1&sort=status%3Aasc&status=active")
		if got != want {
			t.Errorf("urlWithQuery() = %q; want %q", got, want)
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

	t.Run("subtract_alias", func(t *testing.T) {
		fn := fm["subtract"].(func(int, int) int)
		if got := fn(10, 3); got != 7 {
			t.Errorf("subtract(10,3) = %d; want 7", got)
		}
	})

	t.Run("percent", func(t *testing.T) {
		fn := fm["percent"].(func(int, int) int)
		if got := fn(50, 200); got != 25 {
			t.Errorf("percent(50,200) = %d; want 25", got)
		}
	})

	t.Run("percent_zero_total", func(t *testing.T) {
		fn := fm["percent"].(func(int, int) int)
		if got := fn(10, 0); got != 0 {
			t.Errorf("percent(10,0) = %d; want 0", got)
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
		"subtract:7",
		"percent:25",
		"seq:123",
	} {
		if !strings.Contains(body, want) {
			t.Errorf("body missing %q:\n%s", want, body)
		}
	}
}

// TestTemplateRenderer_RenderSanitizesScript verifies that <script> tags are
// stripped when HTML content passes through dangerouslySetInnerHTML during
// actual template rendering (end-to-end, not just the function).
func TestTemplateRenderer_RenderSanitizesScript(t *testing.T) {
	mfs := testFS()
	mfs["templates/sanitize/page.html"] = &fstest.MapFile{
		Data: []byte(
			`{{ template "base" . }}` +
				`{{ define "content" }}{{ dangerouslySetInnerHTML .HTML }}{{ end }}`),
	}
	r, err := NewTemplateRenderer(mfs, false)
	if err != nil {
		t.Fatalf("NewTemplateRenderer() error: %v", err)
	}

	data := map[string]any{
		"HTML": `<p>Safe paragraph</p><script>alert("xss")</script>`,
	}
	inst := r.Instance("sanitize/page.html", data)
	w := httptest.NewRecorder()
	if err := inst.Render(w); err != nil {
		t.Fatalf("Render() error: %v", err)
	}

	body := w.Body.String()
	if !strings.Contains(body, "<p>Safe paragraph</p>") {
		t.Errorf("rendered output should preserve safe HTML content:\n%s", body)
	}
	if strings.Contains(body, "<script") || strings.Contains(body, "alert") {
		t.Errorf("rendered output should not contain <script> tags or script content:\n%s", body)
	}
}

func TestPaginationTemplate_UsesPagesRange(t *testing.T) {
	r, err := NewTemplateRenderer(web.EmbeddedFS, false)
	if err != nil {
		t.Fatalf("NewTemplateRenderer() error: %v", err)
	}

	data := map[string]any{
		"Users":   []any{},
		"BaseURL": "/users",
		"Pagination": &pagination.Pagination[any]{
			CurrentPage:      5,
			ItemsPerPage:     10,
			TotalPages:       10,
			PreviousPage:     intPtr(4),
			NextPage:         intPtr(6),
			FirstPage:        1,
			LastPage:         10,
			FirstPageInRange: 4,
			LastPageInRange:  6,
			Pages:            []int{4, 5, 6},
		},
	}

	inst := r.Instance("user/list.html", data)
	w := httptest.NewRecorder()
	if err := inst.Render(w); err != nil {
		t.Fatalf("Render() error: %v", err)
	}

	body := w.Body.String()
	for _, want := range []string{
		"href=\"/users?page=4&page_size=10\"",
		"href=\"/users?page=6&page_size=10\"",
		"href=\"/users?page=1&page_size=10\"",
		"href=\"/users?page=10&page_size=10\"",
		"&hellip;",
	} {
		if !strings.Contains(body, want) {
			t.Errorf("body missing %q", want)
		}
	}

	if strings.Contains(body, "href=\"/users?page=5&page_size=10\"") {
		t.Error("current page should be rendered as active state, not link")
	}
}

func TestPaginationTemplate_DoesNotDoubleEncodeFallbackLinks(t *testing.T) {
	r, err := NewTemplateRenderer(web.EmbeddedFS, false)
	if err != nil {
		t.Fatalf("NewTemplateRenderer() error: %v", err)
	}

	data := map[string]any{
		"Users":           []any{},
		"BaseURL":         "/users",
		"PageSize":        1,
		"FilterQuery":     "&sort=status%3Aasc&status=active",
		"StatusSortQuery": "page=1&page_size=1&sort=status%3Adesc&status=active",
		"StatusSortDirection": "asc",
		"Pagination": &pagination.Pagination[any]{
			CurrentPage:      1,
			ItemsPerPage:     1,
			TotalPages:       2,
			NextPage:         intPtr(2),
			FirstPage:        1,
			LastPage:         2,
			FirstPageInRange: 1,
			LastPageInRange:  2,
			Pages:            []int{1, 2},
		},
	}

	inst := r.Instance("user/list.html", data)
	w := httptest.NewRecorder()
	if err := inst.Render(w); err != nil {
		t.Fatalf("Render() error: %v", err)
	}

	body := w.Body.String()
	if strings.Contains(body, "page%3d1") || strings.Contains(body, "page_size%3d1") {
		t.Fatalf("expected fallback href links to avoid double-encoded query separators, got %q", body)
	}
	for _, want := range []string{
		"href=\"/users?page=1&amp;page_size=1&amp;sort=status%3Adesc&amp;status=active\"",
		"href=\"/users?page=2&amp;page_size=1&amp;sort=status%3Aasc&amp;status=active\"",
	} {
		if !strings.Contains(body, want) {
			t.Fatalf("body missing %q", want)
		}
	}
}

func TestPaginationTemplate_HidesNavigationWhenSinglePage(t *testing.T) {
	r, err := NewTemplateRenderer(web.EmbeddedFS, false)
	if err != nil {
		t.Fatalf("NewTemplateRenderer() error: %v", err)
	}

	data := map[string]any{
		"Users":   []any{},
		"BaseURL": "/users",
		"Pagination": &pagination.Pagination[any]{
			CurrentPage:  1,
			ItemsPerPage: 10,
			TotalPages:   1,
			FirstPage:    1,
			LastPage:     1,
			Pages:        []int{1},
		},
	}

	inst := r.Instance("user/list.html", data)
	w := httptest.NewRecorder()
	if err := inst.Render(w); err != nil {
		t.Fatalf("Render() error: %v", err)
	}

	body := w.Body.String()
	if strings.Contains(body, `aria-label="分页导航"`) {
		t.Error("pagination navigation should not render when total pages is 1")
	}
}

func TestTemplateRenderer_Regression_BaseBlocksAndCSRFScript(t *testing.T) {
	r, err := NewTemplateRenderer(web.EmbeddedFS, false)
	if err != nil {
		t.Fatalf("NewTemplateRenderer() error: %v", err)
	}

	t.Run("home_includes_nav_csrf_meta_and_htmx_csrf_script", func(t *testing.T) {
		inst := r.Instance("home.html", map[string]any{"CSRFToken": "test-csrf-token"})
		w := httptest.NewRecorder()
		if err := inst.Render(w); err != nil {
			t.Fatalf("Render() error: %v", err)
		}

		body := w.Body.String()
		for _, want := range []string{
			"<nav",
			`<meta name="csrf-token"`,
			"resolveCSRFToken",
		} {
			if !strings.Contains(body, want) {
				t.Errorf("body missing %q:\n%s", want, body)
			}
		}
	})

	t.Run("login_overrides_nav_and_hides_nav_html", func(t *testing.T) {
		inst := r.Instance("auth/login.html", map[string]any{"CSRFToken": "test-csrf-token"})
		w := httptest.NewRecorder()
		if err := inst.Render(w); err != nil {
			t.Fatalf("Render() error: %v", err)
		}

		body := w.Body.String()
		if strings.Contains(body, "<nav") {
			t.Errorf("login page should not render nav HTML:\n%s", body)
		}
	})

	t.Run("page_can_override_head_and_scripts_blocks", func(t *testing.T) {
		realBase, err := web.EmbeddedFS.ReadFile("templates/layouts/base.html")
		if err != nil {
			t.Fatalf("read real base template error: %v", err)
		}
		for _, want := range []string{`block "head"`, `block "scripts"`} {
			if !strings.Contains(string(realBase), want) {
				t.Fatalf("real base template missing %q", want)
			}
		}

		realNav, err := web.EmbeddedFS.ReadFile("templates/partials/nav.html")
		if err != nil {
			t.Fatalf("read real nav partial error: %v", err)
		}
		realScriptsCommon, err := web.EmbeddedFS.ReadFile("templates/partials/scripts_common.html")
		if err != nil {
			t.Fatalf("read real scripts common partial error: %v", err)
		}

		overrideFS := fstest.MapFS{
			"templates/layouts/base.html": &fstest.MapFile{
				Data: realBase,
			},
			"templates/partials/nav.html": &fstest.MapFile{
				Data: realNav,
			},
			"templates/partials/scripts_common.html": &fstest.MapFile{
				Data: realScriptsCommon,
			},
			"templates/custom/override.html": &fstest.MapFile{
				Data: []byte(
					`{{ template "base" . }}` +
						`{{ define "head" }}<meta name="x-head" content="override">{{ end }}` +
						`{{ define "content" }}<h1>Override</h1>{{ end }}` +
						`{{ define "scripts" }}<script>window.__scripts_override__=true;</script>{{ end }}`),
			},
		}

		r2, err := NewTemplateRenderer(overrideFS, false)
		if err != nil {
			t.Fatalf("NewTemplateRenderer() error: %v", err)
		}

		inst := r2.Instance("custom/override.html", nil)
		w := httptest.NewRecorder()
		if err := inst.Render(w); err != nil {
			t.Fatalf("Render() error: %v", err)
		}

		body := w.Body.String()
		for _, want := range []string{
			`<meta name="x-head" content="override">`,
			`<script>window.__scripts_override__=true;</script>`,
		} {
			if !strings.Contains(body, want) {
				t.Errorf("body missing %q:\n%s", want, body)
			}
		}
	})
}

func intPtr(v int) *int {
	return &v
}

// ---------------------------------------------------------------------------
// Fragment template tests
// ---------------------------------------------------------------------------

// testFSWithFragments returns a test filesystem that includes a fragment template.
// The fragment can be referenced by page templates AND rendered independently.
func testFSWithFragments() fstest.MapFS {
	base := testFS()
	// A fragment template: named *_fragment.html
	base["templates/user/row_fragment.html"] = &fstest.MapFile{
		Data: []byte(`<tr><td>{{ .Name }}</td></tr>`),
	}
	// A page template that references the fragment via {{ template }}
	base["templates/user/detail.html"] = &fstest.MapFile{
		Data: []byte(
			`{{ template "base" . }}` +
				`{{ define "content" }}<div>{{ template "user/row_fragment.html" . }}</div>{{ end }}`),
	}
	return base
}

func TestFragmentTemplates_StandaloneRender(t *testing.T) {
	r, err := NewTemplateRenderer(testFSWithFragments(), false)
	if err != nil {
		t.Fatalf("NewTemplateRenderer() error: %v", err)
	}

	// Fragment should be renderable as a standalone template.
	inst := r.Instance("user/row_fragment.html", map[string]any{"Name": "Alice"})
	w := httptest.NewRecorder()
	if err := inst.Render(w); err != nil {
		t.Fatalf("Render() error: %v", err)
	}
	body := w.Body.String()
	if !strings.Contains(body, "<tr><td>Alice</td></tr>") {
		t.Errorf("fragment standalone render missing expected content:\n%s", body)
	}
}

func TestFragmentTemplates_ReferencedByPage(t *testing.T) {
	r, err := NewTemplateRenderer(testFSWithFragments(), false)
	if err != nil {
		t.Fatalf("NewTemplateRenderer() error: %v", err)
	}

	// Page template that includes the fragment.
	inst := r.Instance("user/detail.html", map[string]any{"Name": "Bob"})
	w := httptest.NewRecorder()
	if err := inst.Render(w); err != nil {
		t.Fatalf("Render() error: %v", err)
	}
	body := w.Body.String()
	if !strings.Contains(body, "<tr><td>Bob</td></tr>") {
		t.Errorf("page template should include fragment content:\n%s", body)
	}
	if !strings.Contains(body, "<!DOCTYPE html>") {
		t.Errorf("page template should still use base layout:\n%s", body)
	}
}

func TestFragmentTemplates_DebugMode(t *testing.T) {
	r, err := NewTemplateRenderer(testFSWithFragments(), true)
	if err != nil {
		t.Fatalf("NewTemplateRenderer() error: %v", err)
	}

	// Fragment standalone in debug mode.
	inst := r.Instance("user/row_fragment.html", map[string]any{"Name": "Carol"})
	w := httptest.NewRecorder()
	if err := inst.Render(w); err != nil {
		t.Fatalf("Render() error: %v", err)
	}
	body := w.Body.String()
	if !strings.Contains(body, "<tr><td>Carol</td></tr>") {
		t.Errorf("fragment standalone render (debug) missing expected content:\n%s", body)
	}
}

func TestFragmentTemplates_NonSuffixFragmentNameNotClassifiedAsFragment(t *testing.T) {
	mfs := testFS()
	// Contains `_fragment` but does not follow the *_fragment.html convention.
	mfs["templates/user/not_fragment_page.html"] = &fstest.MapFile{
		Data: []byte(`<span>Not a fragment include target</span>`),
	}
	mfs["templates/user/consumer.html"] = &fstest.MapFile{
		Data: []byte(
			`{{ template "base" . }}` +
				`{{ define "content" }}{{ template "user/not_fragment_page.html" . }}{{ end }}`),
	}

	r, err := NewTemplateRenderer(mfs, false)
	if err != nil {
		t.Fatalf("NewTemplateRenderer() error: %v", err)
	}

	inst := r.Instance("user/consumer.html", nil)
	w := httptest.NewRecorder()
	err = inst.Render(w)
	if err == nil {
		t.Fatal("Render() should fail when non-fragment page is referenced as fragment include")
	}
	if !strings.Contains(err.Error(), "user/not_fragment_page.html") {
		t.Fatalf("error = %q, want to mention missing template", err.Error())
	}
}

func TestDiscoverPageTemplates_IncludesFragments(t *testing.T) {
	r := &TemplateRenderer{fs: testFSWithFragments()}
	pages, err := r.discoverPageTemplates()
	if err != nil {
		t.Fatalf("discoverPageTemplates() error: %v", err)
	}

	found := false
	for _, p := range pages {
		if p == "templates/user/row_fragment.html" {
			found = true
			break
		}
	}
	if !found {
		t.Error("discoverPageTemplates should include fragment templates")
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
