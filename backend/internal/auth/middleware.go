package auth

import (
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"
)

const (
	// CookieName is the name of the auth cookie
	CookieName = "auth_token"

	// ContextKeyUserID is the key for user ID in gin context
	ContextKeyUserID = "user_id"

	// ContextKeyEmail is the key for user email in gin context
	ContextKeyEmail = "user_email"

	// ContextKeyClaims is the key for full claims in gin context
	ContextKeyClaims = "auth_claims"
)

// JWTMiddleware returns a gin middleware that validates JWT tokens from cookies
func JWTMiddleware(authService *Service) gin.HandlerFunc {
	return func(c *gin.Context) {
		// Get token from cookie
		tokenString, err := c.Cookie(CookieName)
		if err != nil || tokenString == "" {
			c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{
				"error": "authentication required",
			})
			return
		}

		// Validate token
		claims, err := authService.ValidateJWT(tokenString)
		if err != nil {
			ClearAuthCookie(c)
			c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{
				"error": "invalid or expired session",
			})
			return
		}

		// Set user info in context
		c.Set(ContextKeyUserID, claims.UserID)
		c.Set(ContextKeyEmail, claims.Email)
		c.Set(ContextKeyClaims, claims)

		// Auto-refresh token if needed
		if authService.ShouldRefreshToken(claims) {
			newToken, expiresAt, err := authService.GenerateJWT(claims.UserID, claims.Email)
			if err == nil {
				SetAuthCookie(c, newToken, expiresAt)
			}
		}

		c.Next()
	}
}

// SetAuthCookie sets the authentication cookie
func SetAuthCookie(c *gin.Context, token string, expiresAt interface{}) {
	// Cookie max age in seconds (24 hours)
	maxAge := int(JWTExpiry.Seconds())

	// Determine if we're in production (HTTPS)
	secure := isSecureContext(c)

	c.SetSameSite(http.SameSiteNoneMode)
	c.SetCookie(
		CookieName,
		token,
		maxAge,
		"/",
		"",   // domain - empty means current domain
		true, // secure flag - required for SameSite=None
		true, // httpOnly - prevents JavaScript access
	)
}

// ClearAuthCookie removes the authentication cookie
func ClearAuthCookie(c *gin.Context) {
	secure := isSecureContext(c)

	c.SetSameSite(http.SameSiteNoneMode)
	c.SetCookie(
		CookieName,
		"",
		-1, // negative max age = delete
		"/",
		"",
		true, // secure flag - required for SameSite=None
		true,
	)
}

// isSecureContext determines if the request is over HTTPS
func isSecureContext(c *gin.Context) bool {
	// Check X-Forwarded-Proto header (common for reverse proxies)
	if proto := c.GetHeader("X-Forwarded-Proto"); proto == "https" {
		return true
	}

	// Check if the request URL scheme is HTTPS
	if c.Request.TLS != nil {
		return true
	}

	// Check for common production indicators
	host := strings.ToLower(c.Request.Host)
	if strings.Contains(host, "render.com") ||
		strings.Contains(host, "riskscore.website") ||
		strings.HasSuffix(host, ".onrender.com") {
		return true
	}

	return false
}

// GetUserID extracts the user ID from the gin context
func GetUserID(c *gin.Context) (uint, bool) {
	id, exists := c.Get(ContextKeyUserID)
	if !exists {
		return 0, false
	}
	userID, ok := id.(uint)
	return userID, ok
}

// GetUserEmail extracts the user email from the gin context
func GetUserEmail(c *gin.Context) (string, bool) {
	email, exists := c.Get(ContextKeyEmail)
	if !exists {
		return "", false
	}
	emailStr, ok := email.(string)
	return emailStr, ok
}

// GetClaims extracts the full claims from the gin context
func GetClaims(c *gin.Context) (*Claims, bool) {
	claims, exists := c.Get(ContextKeyClaims)
	if !exists {
		return nil, false
	}
	claimsPtr, ok := claims.(*Claims)
	return claimsPtr, ok
}
