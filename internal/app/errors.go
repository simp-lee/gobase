package app

import (
	"fmt"
	"strings"

	"github.com/gin-gonic/gin"

	"github.com/simp-lee/gobase/internal/pkg"
)

// errorTemplates maps HTTP status codes to their error template paths.
var errorTemplates = map[int]string{
	400: "errors/400.html",
	404: "errors/404.html",
	500: "errors/500.html",
}

// renderError sends an error response appropriate for the client.
// For requests that accept HTML, it renders the corresponding error template
// (falling back to errors/500.html for unmapped codes, then plain text if
// template rendering panics). For other requests it returns a JSON envelope.
func renderError(c *gin.Context, code int, message string) {
	accept := strings.ToLower(c.GetHeader("Accept"))
	// Explicit JSON request — check before acceptsHTML because acceptsHTML also matches */*.
	if strings.Contains(accept, "application/json") && !strings.Contains(accept, "text/html") {
		c.JSON(code, pkg.Response{Code: code, Message: message})
		return
	}
	if acceptsHTML(c) {
		renderHTMLErrorPage(c, code)
		return
	}
	c.JSON(code, pkg.Response{
		Code:    code,
		Message: message,
		Data:    nil,
	})
}

// renderHTMLErrorPage renders the error template for the given status code.
// If no template exists for the code, it falls back to errors/500.html.
// If rendering panics, it falls back to a plain text response.
func renderHTMLErrorPage(c *gin.Context, code int) {
	defer func() {
		if r := recover(); r != nil {
			c.Data(code, "text/plain; charset=utf-8",
				[]byte(fmt.Sprintf("%d %s", code, defaultStatusText(code))))
		}
	}()

	tmpl, ok := errorTemplates[code]
	if !ok {
		tmpl = errorTemplates[500]
	}
	c.HTML(code, tmpl, gin.H{})
}

// acceptsHTML returns true if the client accepts an HTML response.
// Matches text/html, */* (browser default), and empty Accept headers.
func acceptsHTML(c *gin.Context) bool {
	accept := strings.ToLower(c.GetHeader("Accept"))
	return strings.Contains(accept, "text/html") ||
		strings.Contains(accept, "*/*") ||
		strings.TrimSpace(accept) == ""
}

// defaultStatusText returns a short human-readable label for common error codes.
func defaultStatusText(code int) string {
	switch code {
	case 400:
		return "Bad Request"
	case 404:
		return "Not Found"
	case 408:
		return "Request Timeout"
	case 429:
		return "Too Many Requests"
	case 500:
		return "Internal Server Error"
	default:
		return "Error"
	}
}
