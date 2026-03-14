package middleware

import (
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/base64"
	"encoding/hex"
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"
)

const (
	csrfCookieName      = "_csrf_token"
	csrfFormField       = "_csrf_token"
	csrfHeaderName      = "X-CSRF-Token"
	csrfContextKey      = "CSRFToken"
	csrfMinSecretLength = 32
)

// CSRF returns a gin middleware that provides CSRF protection for HTML form submissions.
// The secret is used to sign CSRF tokens with HMAC-SHA256.
//
// Token format: hex(nonce) + "." + base64url(HMAC-SHA256(nonce, secret))
//
// For GET/HEAD/OPTIONS requests, a CSRF token is generated (if not already present as a valid
// cookie) and set as a cookie (HttpOnly=false, SameSite=Strict). The token is also stored
// in gin.Context under the key "CSRFToken" for use in templates.
//
// For POST/PUT/PATCH/DELETE requests, the token is read from the form field "_csrf_token"
// or the header "X-CSRF-Token" and validated against the cookie value using constant-time
// comparison. On failure, a 403 Forbidden JSON response is returned.
//
// API routes should be exempted by not registering this middleware on their route groups.
func CSRF(secret string) gin.HandlerFunc {
	secret = strings.TrimSpace(secret)
	if secret == "" {
		return func(c *gin.Context) {
			c.AbortWithStatusJSON(http.StatusInternalServerError, gin.H{
				"error": "csrf secret is required",
			})
		}
	}
	if len(secret) < csrfMinSecretLength {
		return func(c *gin.Context) {
			c.AbortWithStatusJSON(http.StatusInternalServerError, gin.H{
				"error": "csrf secret must be at least 32 characters",
			})
		}
	}

	return func(c *gin.Context) {
		switch c.Request.Method {
		case http.MethodGet, http.MethodHead, http.MethodOptions:
			token, err := c.Cookie(csrfCookieName)
			if err != nil || token == "" || !validToken(token, secret) {
				token, err = generateToken(secret)
				if err != nil {
					c.AbortWithStatusJSON(http.StatusInternalServerError, gin.H{
						"error": "failed to generate CSRF token",
					})
					return
				}
				setCSRFCookie(c, token, requestIsSecure(c.Request))
			}
			c.Set(csrfContextKey, token)
			c.Next()
		case http.MethodPost, http.MethodPut, http.MethodPatch, http.MethodDelete:
			cookieToken, err := c.Cookie(csrfCookieName)
			if err != nil || cookieToken == "" {
				c.AbortWithStatusJSON(http.StatusForbidden, gin.H{
					"error": "CSRF token missing",
				})
				return
			}

			requestToken := c.PostForm(csrfFormField)
			if requestToken == "" {
				requestToken = c.GetHeader(csrfHeaderName)
			}
			if requestToken == "" {
				c.AbortWithStatusJSON(http.StatusForbidden, gin.H{
					"error": "CSRF token missing",
				})
				return
			}

			if !validToken(cookieToken, secret) || !validToken(requestToken, secret) {
				c.AbortWithStatusJSON(http.StatusForbidden, gin.H{
					"error": "CSRF token invalid",
				})
				return
			}

			if !tokensMatch(cookieToken, requestToken) {
				c.AbortWithStatusJSON(http.StatusForbidden, gin.H{
					"error": "CSRF token invalid",
				})
				return
			}

			c.Set(csrfContextKey, cookieToken)
			c.Next()
		default:
			c.Next()
		}
	}
}

// GetCSRFToken retrieves the CSRF token stored in gin.Context by the CSRF middleware.
// Returns an empty string if no token is available.
func GetCSRFToken(c *gin.Context) string {
	if token, exists := c.Get(csrfContextKey); exists {
		if s, ok := token.(string); ok {
			return s
		}
	}
	return ""
}

// SetCSRFToken reads the CSRF token from the cookie and stores it in gin.Context
// under the key "CSRFToken". This helper does not validate token signatures and
// should only be used on requests that have already passed through CSRF(secret)
// in the same request chain. If the token is already present in gin.Context,
// this is a no-op.
//
// Deprecated: SetCSRFToken does not verify the token signature.
// Use SetCSRFTokenWithSecret instead, which validates the HMAC before storing.
// Removal target: v2.0.
func SetCSRFToken(c *gin.Context) {
	if _, exists := c.Get(csrfContextKey); exists {
		return
	}
	if token, err := c.Cookie(csrfCookieName); err == nil && token != "" {
		c.Set(csrfContextKey, token)
	}
}

// SetCSRFTokenWithSecret reads the CSRF token from the cookie and stores it in
// gin.Context only when the token has a valid signature for the provided secret.
// If the token is already present in gin.Context, or secret/cookie is empty,
// this is a no-op.
func SetCSRFTokenWithSecret(c *gin.Context, secret string) {
	if _, exists := c.Get(csrfContextKey); exists {
		return
	}
	secret = strings.TrimSpace(secret)
	if secret == "" {
		return
	}
	token, err := c.Cookie(csrfCookieName)
	if err != nil || token == "" {
		return
	}
	if !validToken(token, secret) {
		return
	}
	c.Set(csrfContextKey, token)
}

// generateToken creates a new CSRF token: hex(nonce) + "." + base64url(HMAC-SHA256(nonce, secret)).
func generateToken(secret string) (string, error) {
	nonce := make([]byte, 32)
	if _, err := rand.Read(nonce); err != nil {
		return "", err
	}
	nonceHex := hex.EncodeToString(nonce)
	sig := signNonce(nonceHex, secret)
	return nonceHex + "." + sig, nil
}

// signNonce returns the base64url-encoded HMAC-SHA256 signature of the nonce.
func signNonce(nonce, secret string) string {
	mac := hmac.New(sha256.New, []byte(secret))
	mac.Write([]byte(nonce))
	return base64.RawURLEncoding.EncodeToString(mac.Sum(nil))
}

// validToken checks whether the token has a valid format and a correct HMAC signature.
func validToken(token, secret string) bool {
	parts := strings.SplitN(token, ".", 2)
	if len(parts) != 2 || parts[0] == "" || parts[1] == "" {
		return false
	}
	expectedSig := signNonce(parts[0], secret)
	return subtle.ConstantTimeCompare([]byte(parts[1]), []byte(expectedSig)) == 1
}

// ValidateCSRFToken validates a CSRF token format and signature.
func ValidateCSRFToken(token, secret string) bool {
	return validToken(token, secret)
}

// tokensMatch performs a constant-time comparison of two token strings.
func tokensMatch(a, b string) bool {
	return subtle.ConstantTimeCompare([]byte(a), []byte(b)) == 1
}

// ValidateCSRFTokenPair validates both tokens and checks they match.
func ValidateCSRFTokenPair(cookieToken, requestToken, secret string) bool {
	return validToken(cookieToken, secret) && validToken(requestToken, secret) && tokensMatch(cookieToken, requestToken)
}

// requestIsSecure reports whether the incoming request arrived over HTTPS.
// It checks the actual TLS connection first, then falls back to reverse-proxy
// headers X-Forwarded-Proto and X-Forwarded-Ssl.
func requestIsSecure(r *http.Request) bool {
	if r == nil {
		return false
	}
	if r.TLS != nil {
		return true
	}
	if forwardedProto := r.Header.Get("X-Forwarded-Proto"); forwardedProto != "" {
		for _, part := range strings.Split(forwardedProto, ",") {
			proto := strings.TrimSpace(part)
			if proto == "" {
				continue
			}
			return strings.EqualFold(proto, "https")
		}
	}
	if strings.EqualFold(r.Header.Get("X-Forwarded-Ssl"), "on") {
		return true
	}
	return false
}

// setCSRFCookie sets the CSRF token cookie with HttpOnly=false and SameSite=Strict.
// When secure is true, the Secure flag is set so the cookie is only transmitted
// over HTTPS.
func setCSRFCookie(c *gin.Context, token string, secure bool) {
	http.SetCookie(c.Writer, &http.Cookie{
		Name:     csrfCookieName,
		Value:    token,
		Path:     "/",
		HttpOnly: false,
		Secure:   secure,
		SameSite: http.SameSiteStrictMode,
	})
}
