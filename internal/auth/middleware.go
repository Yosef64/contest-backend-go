package auth

import (
	"net/http"
	"strings"
	"time"

	"github.com/gin-gonic/gin"

	"victor-contest-go/internal/httpx"
)

const (
	// AdminCookie holds the admin session (httpOnly; never readable by JS).
	AdminCookie = "victory_admin"
	ctxClaims   = "auth.claims"
)

// Require rejects requests without a valid token for the given role.
// Students send "Authorization: Bearer <token>"; admins use the cookie (or
// a bearer token, for tools).
func (t *Tokens) Require(role Role) gin.HandlerFunc {
	return func(c *gin.Context) {
		raw := bearer(c)
		if raw == "" && role == RoleAdmin {
			raw, _ = c.Cookie(AdminCookie)
		}
		if raw == "" {
			httpx.Fail(c, httpx.Unauthorized("Please sign in."))
			return
		}
		claims, err := t.Parse(raw)
		if err != nil || claims.Role != role || claims.Subject == "" {
			httpx.Fail(c, httpx.Unauthorized("Your session has expired. Please sign in again."))
			return
		}
		if role == RoleStudent && claims.Session != "" && t.SessionCheck != nil {
			if err := t.SessionCheck(c, claims); err != nil {
				httpx.Fail(c, err)
				return
			}
		}
		c.Set(ctxClaims, claims)
		c.Next()
	}
}

func bearer(c *gin.Context) string {
	h := c.GetHeader("Authorization")
	if len(h) > 7 && strings.EqualFold(h[:7], "bearer ") {
		return strings.TrimSpace(h[7:])
	}
	return ""
}

// Caller returns the authenticated claims set by Require.
func Caller(c *gin.Context) *Claims {
	v, _ := c.Get(ctxClaims)
	claims, _ := v.(*Claims)
	return claims
}

// SetAdminCookie stores the admin session token.
func SetAdminCookie(c *gin.Context, token string, ttl time.Duration, secure bool) {
	sameSite := http.SameSiteLaxMode
	if secure {
		// The dashboard is served from a different site than the API in
		// production, so the cookie must be allowed cross-site.
		sameSite = http.SameSiteNoneMode
	}
	http.SetCookie(c.Writer, &http.Cookie{
		Name:     AdminCookie,
		Value:    token,
		Path:     "/",
		MaxAge:   int(ttl.Seconds()),
		HttpOnly: true,
		Secure:   secure,
		SameSite: sameSite,
	})
}

func ClearAdminCookie(c *gin.Context, secure bool) {
	SetAdminCookie(c, "", -time.Second, secure)
}
