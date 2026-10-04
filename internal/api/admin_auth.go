package api

import (
	"errors"
	"strings"
	"time"

	"github.com/gin-gonic/gin"

	"victor-contest-go/internal/auth"
	"victor-contest-go/internal/httpx"
	"victor-contest-go/internal/store"
)

// A precomputed hash so login takes the same time whether or not the email
// exists (no account enumeration by timing).
var dummyHash, _ = auth.HashPassword("not-a-real-password")

func (s *Server) adminLogin(c *gin.Context) error {
	var in struct {
		Email    string `json:"email" binding:"required,email"`
		Password string `json:"password" binding:"required"`
		// Remember: keep the session after the browser closes (default).
		Remember *bool `json:"remember"`
	}
	if err := httpx.Bind(c, &in); err != nil {
		return err
	}
	admin, err := s.store.AdminByEmail(c, in.Email)
	if err != nil && !errors.Is(err, store.ErrNotFound) {
		return err
	}
	hash := dummyHash
	if err == nil {
		hash = admin.PasswordHash
	}
	if !auth.CheckPassword(hash, in.Password) || err != nil {
		return httpx.Unauthorized("Email or password is incorrect.")
	}
	if !admin.IsApproved {
		return httpx.Forbidden("Your account is waiting for approval by another admin.")
	}
	return s.startAdminSession(c, admin, in.Remember == nil || *in.Remember)
}

// browserSessionTTL bounds a session that is not remembered: its cookie ends
// with the browser, and the token itself expires after a working day.
const browserSessionTTL = 12 * time.Hour

// startAdminSession signs the admin in. Without remember the cookie has no
// expiry, so the browser drops it when it closes.
func (s *Server) startAdminSession(c *gin.Context, admin store.Admin, remember bool) error {
	ttl := s.cfg.AdminSessionTTL
	if !remember && ttl > browserSessionTTL {
		ttl = browserSessionTTL
	}
	token, err := s.tokens.Issue(auth.RoleAdmin, admin.ID, admin.Name, ttl)
	if err != nil {
		return err
	}
	cookieTTL := ttl
	if !remember {
		cookieTTL = 0
	}
	auth.SetAdminCookie(c, token, cookieTTL, s.cfg.CookieSecure)
	return httpx.OK(c, gin.H{"admin": admin})
}

// adminGoogle signs in an existing, approved admin whose account email is
// the verified email of a Google ID token. It never creates accounts.
func (s *Server) adminGoogle(c *gin.Context) error {
	var in struct {
		Credential string `json:"credential" binding:"required"`
		Remember   *bool  `json:"remember"`
	}
	if err := httpx.Bind(c, &in); err != nil {
		return err
	}
	if len(s.cfg.GoogleClientIDs) == 0 {
		return httpx.Conflict("google_unavailable", "Google sign-in isn't set up for this dashboard yet.")
	}
	user, err := auth.VerifyGoogleIDToken(c, in.Credential, s.cfg.GoogleClientIDs)
	if err != nil || !user.Verified || user.Email == "" {
		return httpx.Unauthorized("Google could not confirm this account. Try again.")
	}
	admin, err := s.store.AdminByEmail(c, user.Email)
	if errors.Is(err, store.ErrNotFound) {
		return httpx.Unauthorized("No admin account uses " + user.Email + ". Request access first.")
	}
	if err != nil {
		return err
	}
	if !admin.IsApproved {
		return httpx.Forbidden("Your account is waiting for approval by another admin.")
	}
	return s.startAdminSession(c, admin, in.Remember == nil || *in.Remember)
}

func (s *Server) adminLogout(c *gin.Context) error {
	auth.ClearAdminCookie(c, s.cfg.CookieSecure)
	return httpx.NoContent(c)
}

// adminRegister creates an unapproved account; an existing admin approves it.
func (s *Server) adminRegister(c *gin.Context) error {
	var in struct {
		Name     string `json:"name" binding:"required,min=2,max=80"`
		Email    string `json:"email" binding:"required,email,max=200"`
		Password string `json:"password" binding:"required,min=8,max=200"`
	}
	if err := httpx.Bind(c, &in); err != nil {
		return err
	}
	hash, err := auth.HashPassword(in.Password)
	if err != nil {
		return err
	}
	_, err = s.store.CreateAdmin(c, store.Admin{Name: strings.TrimSpace(in.Name), Email: in.Email, PasswordHash: hash})
	if errors.Is(err, store.ErrDuplicate) {
		return httpx.Field("email", "An account with this email already exists.")
	}
	if err != nil {
		return err
	}
	return httpx.Created(c, gin.H{})
}

// requireApprovedAdmin re-checks the account on every request, so revoking
// access takes effect immediately rather than when the token expires.
func (s *Server) requireApprovedAdmin(c *gin.Context) {
	admin, err := s.store.AdminByID(c, auth.Caller(c).Subject)
	if err != nil || !admin.IsApproved {
		auth.ClearAdminCookie(c, s.cfg.CookieSecure)
		httpx.Fail(c, httpx.Unauthorized("Your session is no longer valid. Please sign in again."))
		return
	}
	c.Set("admin", admin)
	c.Next()
}

func currentAdmin(c *gin.Context) store.Admin {
	v, _ := c.Get("admin")
	a, _ := v.(store.Admin)
	return a
}

func (s *Server) adminMe(c *gin.Context) error {
	return httpx.OK(c, gin.H{"admin": currentAdmin(c)})
}

func (s *Server) adminOverview(c *gin.Context) error {
	o, err := s.store.Overview(c)
	if err != nil {
		return err
	}
	return httpx.OK(c, o)
}

const lockTTL = 2 * time.Minute
