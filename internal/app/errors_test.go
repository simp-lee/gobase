package app

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"

	"github.com/simp-lee/gobase/internal/pkg"
)

func init() {
	gin.SetMode(gin.TestMode)
}

func TestAcceptsHTML(t *testing.T) {
	tests := []struct {
		name   string
		accept string
		want   bool
	}{
		{"text/html", "text/html", true},
		{"text/html with charset", "text/html; charset=utf-8", true},
		{"mixed with html", "application/json, text/html", true},
		{"application/json only", "application/json", false},
		{"empty accept", "", true},
		{"wildcard accept", "*/*", true},
		{"case insensitive", "Text/HTML", true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			w := httptest.NewRecorder()
			c, _ := gin.CreateTestContext(w)
			c.Request = httptest.NewRequest(http.MethodGet, "/", nil)
			if tt.accept != "" {
				c.Request.Header.Set("Accept", tt.accept)
			}

			got := acceptsHTML(c)
			if got != tt.want {
				t.Fatalf("acceptsHTML() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestRenderError_JSON(t *testing.T) {
	tests := []struct {
		name    string
		accept  string
		code    int
		message string
	}{
		{"500 internal", "application/json", 500, "internal server error"},
		{"400 bad request", "application/json", 400, "bad request"},
		{"404 not found", "application/json", 404, "not found"},
		{"408 request timeout", "application/json", 408, "request timeout"},
		{"429 rate limited", "application/json", 429, "too many requests"},
		{"json with wildcard", "application/json, */*", 500, "internal server error"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			w := httptest.NewRecorder()
			c, _ := gin.CreateTestContext(w)
			c.Request = httptest.NewRequest(http.MethodGet, "/api/test", nil)
			c.Request.Header.Set("Accept", tt.accept)

			renderError(c, tt.code, tt.message)

			if w.Code != tt.code {
				t.Fatalf("status = %d, want %d", w.Code, tt.code)
			}

			var resp pkg.Response
			if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
				t.Fatalf("json decode error: %v", err)
			}
			if resp.Code != tt.code {
				t.Fatalf("resp.Code = %d, want %d", resp.Code, tt.code)
			}
			if resp.Message != tt.message {
				t.Fatalf("resp.Message = %q, want %q", resp.Message, tt.message)
			}
			if resp.Data != nil {
				t.Fatalf("resp.Data = %v, want nil", resp.Data)
			}
		})
	}
}

func TestRenderError_HTML_FallsBackToPlainText(t *testing.T) {
	// Without an HTML renderer configured on the engine, c.HTML will panic.
	// renderError should recover and fall back to plain text.
	tests := []struct {
		name     string
		code     int
		wantBody string
	}{
		{"500 fallback", 500, "500 Internal Server Error"},
		{"400 fallback", 400, "400 Bad Request"},
		{"404 fallback", 404, "404 Not Found"},
		{"429 fallback", 429, "429 Too Many Requests"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			w := httptest.NewRecorder()
			c, _ := gin.CreateTestContext(w)
			c.Request = httptest.NewRequest(http.MethodGet, "/", nil)
			c.Request.Header.Set("Accept", "text/html")

			renderError(c, tt.code, "ignored for html")

			if w.Code != tt.code {
				t.Fatalf("status = %d, want %d", w.Code, tt.code)
			}

			body := w.Body.String()
			if body != tt.wantBody {
				t.Fatalf("body = %q, want %q", body, tt.wantBody)
			}

			ct := w.Header().Get("Content-Type")
			if ct != "text/plain; charset=utf-8" {
				t.Fatalf("Content-Type = %q, want %q", ct, "text/plain; charset=utf-8")
			}
		})
	}
}

func TestDefaultStatusText(t *testing.T) {
	tests := []struct {
		code int
		want string
	}{
		{400, "Bad Request"},
		{404, "Not Found"},
		{408, "Request Timeout"},
		{429, "Too Many Requests"},
		{500, "Internal Server Error"},
		{503, "Error"},
	}

	for _, tt := range tests {
		t.Run(tt.want, func(t *testing.T) {
			got := defaultStatusText(tt.code)
			if got != tt.want {
				t.Fatalf("defaultStatusText(%d) = %q, want %q", tt.code, got, tt.want)
			}
		})
	}
}

func TestErrorTemplates(t *testing.T) {
	// Verify the template map contains expected entries.
	expected := map[int]string{
		400: "errors/400.html",
		404: "errors/404.html",
		500: "errors/500.html",
	}

	for code, tmpl := range expected {
		got, ok := errorTemplates[code]
		if !ok {
			t.Fatalf("errorTemplates[%d] missing", code)
		}
		if got != tmpl {
			t.Fatalf("errorTemplates[%d] = %q, want %q", code, got, tmpl)
		}
	}
}
