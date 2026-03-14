package auth

import (
	"errors"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/gin-gonic/gin"

	"github.com/simp-lee/gobase/internal/domain"
	"github.com/simp-lee/gobase/internal/middleware"
	"github.com/simp-lee/gobase/internal/pkg"
)

// AuthHandler handles REST API requests for authentication.
type AuthHandler struct {
	svc               Service
	forceSecureCookie bool
}

// NewHandler creates a new AuthHandler with the given service.
func NewHandler(svc Service) *AuthHandler {
	return &AuthHandler{svc: svc}
}

// NewHandlerWithCookieSecure creates a new AuthHandler with explicit cookie
// secure behavior. When forceSecureCookie is true, auth cookies are always
// marked Secure regardless of request transport/proxy headers.
func NewHandlerWithCookieSecure(svc Service, forceSecureCookie bool) *AuthHandler {
	return &AuthHandler{svc: svc, forceSecureCookie: forceSecureCookie}
}

// Login handles POST /api/v1/auth/login.
func (h *AuthHandler) Login(c *gin.Context) {
	if requiresSameOriginBrowserCheck(c) && !isSameOriginBrowserRequest(c) {
		pkg.Error(c, domain.ErrForbidden)
		return
	}

	var req LoginRequest
	if !pkg.BindAndValidate(c, &req) {
		return
	}

	tokenResp, err := h.svc.Login(c.Request.Context(), req.Email, req.Password)
	if err != nil {
		pkg.Error(c, err)
		return
	}

	h.issueAuthCookie(c, tokenResp.Token, tokenResp.ExpiresAt)

	pkg.Success(c, tokenResp)
}

// Register handles POST /api/v1/auth/register.
func (h *AuthHandler) Register(c *gin.Context) {
	if requiresSameOriginBrowserCheck(c) && !isSameOriginBrowserRequest(c) {
		pkg.Error(c, domain.ErrForbidden)
		return
	}

	var req RegisterRequest
	if !pkg.BindAndValidate(c, &req) {
		return
	}
	if strings.TrimSpace(req.ConfirmPassword) == "" {
		pkg.FieldValidationError(c, map[string]string{"confirm_password": "This field is required"})
		return
	}
	if req.ConfirmPassword != req.Password {
		pkg.FieldValidationError(c, map[string]string{"confirm_password": "confirm_password does not match password"})
		return
	}

	user, err := h.svc.Register(c.Request.Context(), req.Username, req.Email, req.Password)
	if err != nil {
		pkg.Error(c, err)
		return
	}

	c.JSON(http.StatusCreated, pkg.Response{
		Code:    http.StatusCreated,
		Message: "user registered successfully",
		Data: RegisterResponse{
			ID:        user.ID,
			Username:  user.Username,
			Email:     user.Email,
			CreatedAt: user.CreatedAt,
		},
	})
}

// LoginPage handles GET /login — renders the login page.
func (h *AuthHandler) LoginPage(c *gin.Context) {
	c.HTML(http.StatusOK, "auth/login.html", gin.H{
		"CSRFToken": middleware.GetCSRFToken(c),
	})
}

// RegisterPage handles GET /register — renders the register page.
func (h *AuthHandler) RegisterPage(c *gin.Context) {
	c.HTML(http.StatusOK, "auth/register.html", gin.H{
		"CSRFToken": middleware.GetCSRFToken(c),
	})
}

func requiresSameOriginBrowserCheck(c *gin.Context) bool {
	return strings.TrimSpace(c.GetHeader("Origin")) != "" ||
		strings.TrimSpace(c.GetHeader("Referer")) != ""
}

// isSameOriginBrowserRequest validates that browser-sent Origin or Referer
// headers match the request scheme and host. At least one of Origin/Referer
// must be present for browser endpoints.
func isSameOriginBrowserRequest(c *gin.Context) bool {
	origin := strings.TrimSpace(c.GetHeader("Origin"))
	originURL, hasOrigin := parseSameOriginHeaderURL(origin)
	if hasOrigin && originURL == nil {
		return false
	}

	refererURL, hasReferer := parseSameOriginHeaderURL(strings.TrimSpace(c.GetHeader("Referer")))
	if hasReferer && refererURL == nil {
		return false
	}

	// Require Origin or Referer for browser endpoints.
	if !hasOrigin && !hasReferer {
		return false
	}

	clientURL := originURL
	if clientURL == nil {
		clientURL = refererURL
	}

	requestHost := ""
	if c.Request != nil {
		requestHost = strings.TrimSpace(c.Request.Host)
	}
	if requestHost == "" {
		return false
	}

	requestScheme := "http"
	if isHTTPSRequest(c) {
		requestScheme = "https"
	}

	return strings.EqualFold(clientURL.Scheme, requestScheme) &&
		strings.EqualFold(clientURL.Host, requestHost)
}

// parseSameOriginHeaderURL parses a header value (Origin or Referer) into a
// *url.URL. Returns (nil, false) when the value is empty (header absent),
// (nil, true) when the value is present but invalid, and (*url.URL, true)
// when successfully parsed.
func parseSameOriginHeaderURL(value string) (*url.URL, bool) {
	if value == "" {
		return nil, false
	}
	if strings.EqualFold(value, "null") {
		return nil, true
	}

	parsed, err := url.Parse(value)
	if err != nil {
		return nil, true
	}

	if strings.TrimSpace(parsed.Scheme) == "" || strings.TrimSpace(parsed.Host) == "" {
		return nil, true
	}

	return parsed, true
}

// issueAuthCookie sets the access_token cookie on the response.
func (h *AuthHandler) issueAuthCookie(c *gin.Context, token string, expiresAt int64) {
	maxAge := 0
	if expiresAt > 0 {
		seconds := int(time.Until(time.Unix(expiresAt, 0)).Seconds())
		if seconds > 0 {
			maxAge = seconds
		}
	}

	http.SetCookie(c.Writer, &http.Cookie{
		Name:     AccessTokenCookieName,
		Value:    token,
		Path:     "/",
		MaxAge:   maxAge,
		HttpOnly: true,
		Secure:   h.shouldSetSecureCookie(c),
		SameSite: http.SameSiteStrictMode,
	})
}

// shouldSetSecureCookie returns true when the auth cookie must have the Secure
// flag. It honours the explicit forceSecureCookie override first, then falls
// back to detecting the request transport.
func (h *AuthHandler) shouldSetSecureCookie(c *gin.Context) bool {
	if h.forceSecureCookie {
		return true
	}
	return isHTTPSRequest(c)
}

// isHTTPSRequest returns true when the inbound request arrived over TLS,
// either natively or via trusted reverse-proxy headers.
func isHTTPSRequest(c *gin.Context) bool {
	if c.Request != nil && c.Request.TLS != nil {
		return true
	}
	if forwardedProto := c.GetHeader("X-Forwarded-Proto"); forwardedProto != "" {
		for _, part := range strings.Split(forwardedProto, ",") {
			proto := strings.TrimSpace(part)
			if proto == "" {
				continue
			}
			return strings.EqualFold(proto, "https")
		}
	}
	return strings.EqualFold(c.GetHeader("X-Forwarded-Ssl"), "on")
}

// extractBearerToken extracts the raw JWT string from the Authorization header.
func extractBearerToken(c *gin.Context) (string, bool) {
	header := c.GetHeader("Authorization")
	if header == "" {
		return "", false
	}
	parts := strings.SplitN(strings.TrimSpace(header), " ", 2)
	if len(parts) != 2 || !strings.EqualFold(parts[0], "Bearer") {
		return "", false
	}
	token := strings.TrimSpace(parts[1])
	if token == "" {
		return "", false
	}
	return token, true
}

// extractTokenFromCookie extracts the access token from the cookie.
func extractTokenFromCookie(c *gin.Context) (string, bool) {
	token, err := c.Cookie(AccessTokenCookieName)
	if err != nil {
		if errors.Is(err, http.ErrNoCookie) {
			return "", false
		}
		return "", false
	}
	token = strings.TrimSpace(token)
	if token == "" {
		return "", false
	}
	return token, true
}

// extractAccessToken extracts the access token from the Authorization header
// first, then falls back to the auth cookie. Returns the token, whether it
// came from a Bearer header, and whether a token was found at all.
func extractAccessToken(c *gin.Context) (token string, fromBearer bool, ok bool) {
	if bearerToken, hasBearerToken := extractBearerToken(c); hasBearerToken {
		return bearerToken, true, true
	}

	cookieToken, hasCookieToken := extractTokenFromCookie(c)
	if hasCookieToken {
		return cookieToken, false, true
	}

	return "", false, false
}

// clearAuthCookie removes the access_token cookie from the response.
func (h *AuthHandler) clearAuthCookie(c *gin.Context) {
	http.SetCookie(c.Writer, &http.Cookie{
		Name:     AccessTokenCookieName,
		Value:    "",
		Path:     "/",
		MaxAge:   -1,
		Expires:  time.Unix(0, 0),
		HttpOnly: true,
		Secure:   h.shouldSetSecureCookie(c),
		SameSite: http.SameSiteStrictMode,
	})
}

// Logout handles POST /api/v1/auth/logout.
func (h *AuthHandler) Logout(c *gin.Context) {
	token, fromBearer, ok := extractAccessToken(c)
	if !ok {
		pkg.Error(c, domain.ErrUnauthorized)
		return
	}

	if !fromBearer && !isSameOriginBrowserRequest(c) {
		pkg.Error(c, domain.ErrForbidden)
		return
	}

	if err := h.svc.Logout(c.Request.Context(), token); err != nil {
		pkg.Error(c, err)
		return
	}

	h.clearAuthCookie(c)

	pkg.Success(c, nil)
}

// RefreshToken handles POST /api/v1/auth/refresh.
func (h *AuthHandler) RefreshToken(c *gin.Context) {
	token, fromBearer, ok := extractAccessToken(c)
	if !ok {
		pkg.Error(c, domain.ErrUnauthorized)
		return
	}

	if !fromBearer && !isSameOriginBrowserRequest(c) {
		pkg.Error(c, domain.ErrForbidden)
		return
	}

	tokenResp, err := h.svc.RefreshToken(c.Request.Context(), token)
	if err != nil {
		pkg.Error(c, err)
		return
	}

	h.issueAuthCookie(c, tokenResp.Token, tokenResp.ExpiresAt)

	pkg.Success(c, tokenResp)
}
