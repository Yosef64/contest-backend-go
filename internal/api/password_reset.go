package api

import (
	"context"
	"crypto/rand"
	"crypto/tls"
	"errors"
	"fmt"
	"log/slog"
	"math/big"
	"net"
	"net/http"
	"net/mail"
	"net/smtp"
	"sync"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/jackc/pgx/v5"

	"victor-contest-go/internal/auth"
	"victor-contest-go/internal/httpx"
)

// Forgot password for email/phone accounts. A 6-digit code, stored hashed,
// is emailed when SMTP is configured; otherwise an admin issues it from the
// student's page and passes it on (phone accounts have no SMS yet).

const (
	resetCodeTTL      = 15 * time.Minute
	resetCodeAttempts = 5
)

func newResetCode() (string, error) {
	n, err := rand.Int(rand.Reader, big.NewInt(1_000_000))
	if err != nil {
		return "", err
	}
	return fmt.Sprintf("%06d", n.Int64()), nil
}

// issueReset replaces any earlier code for the login.
func (s *Server) issueReset(ctx context.Context, login string, adminID *string) (string, time.Time, error) {
	code, err := newResetCode()
	if err != nil {
		return "", time.Time{}, err
	}
	hash, err := auth.HashPassword(code)
	if err != nil {
		return "", time.Time{}, err
	}
	expires := s.now().Add(resetCodeTTL)
	_, err = s.pool.Exec(ctx, `INSERT INTO password_resets (login, code_hash, expires_at, issued_by) VALUES ($1, $2, $3, $4)
		ON CONFLICT (login) DO UPDATE SET code_hash = $2, expires_at = $3, attempts = 0, issued_by = $4, created_at = now()`,
		login, hash, expires, adminID)
	return code, expires, err
}

// sendResetEmail sends the code and waits for the mail server to accept
// it: on Lambda nothing runs after the response is sent. Port 465 uses TLS
// from the start; other ports (587) upgrade with STARTTLS.
func (s *Server) sendResetEmail(ctx context.Context, d resetEmailData) error {
	cfg := s.cfg
	ctx, cancel := context.WithTimeout(ctx, 15*time.Second)
	defer cancel()
	addr := net.JoinHostPort(cfg.SMTPHost, cfg.SMTPPort)
	tlsConfig := &tls.Config{ServerName: cfg.SMTPHost, MinVersion: tls.VersionTLS12}
	var conn net.Conn
	var err error
	if cfg.SMTPPort == "465" {
		conn, err = (&tls.Dialer{Config: tlsConfig}).DialContext(ctx, "tcp", addr)
	} else {
		conn, err = (&net.Dialer{}).DialContext(ctx, "tcp", addr)
	}
	if err != nil {
		return err
	}
	defer conn.Close()
	if deadline, ok := ctx.Deadline(); ok {
		_ = conn.SetDeadline(deadline)
	}
	client, err := smtp.NewClient(conn, cfg.SMTPHost)
	if err != nil {
		return err
	}
	defer client.Close()
	if ok, _ := client.Extension("STARTTLS"); ok && cfg.SMTPPort != "465" {
		if err := client.StartTLS(tlsConfig); err != nil {
			return err
		}
	}
	if cfg.SMTPUser != "" {
		if err := client.Auth(smtp.PlainAuth("", cfg.SMTPUser, cfg.SMTPPass, cfg.SMTPHost)); err != nil {
			return err
		}
	}
	from, err := mail.ParseAddress(cfg.SMTPFrom)
	if err != nil {
		return fmt.Errorf("SMTP_FROM: %w", err)
	}
	if err := client.Mail(from.Address); err != nil {
		return err
	}
	if err := client.Rcpt(d.Email); err != nil {
		return err
	}
	w, err := client.Data()
	if err != nil {
		return err
	}
	msg, err := resetEmail(cfg.SMTPFrom, from.Address, d, s.now())
	if err != nil {
		return err
	}
	if _, err := w.Write(msg); err != nil {
		return err
	}
	if err := w.Close(); err != nil {
		return err
	}
	return client.Quit()
}

// forgotPassword always answers the same way for a given login format, so
// it does not reveal whether an account exists.
func (s *Server) forgotPassword(c *gin.Context) error {
	var in struct {
		Login string `json:"login" binding:"required,max=254"`
	}
	if err := httpx.Bind(c, &in); err != nil {
		return err
	}
	login, email, _, ok := normalizeLogin(in.Login)
	if !ok {
		return httpx.Field("login", "Enter a valid email address or Ethiopian phone number.")
	}
	ipKey, resetKey := "ip:"+c.ClientIP(), "reset:"+login
	if passwordLimiter.blocked(ipKey, resetKey) {
		return httpx.NewError(http.StatusTooManyRequests, "too_many_attempts", "Too many requests. Wait 15 minutes and try again.")
	}
	passwordLimiter.fail(resetKey)

	delivery := "support"
	if email != "" && s.cfg.MailEnabled() {
		delivery = "email"
	}
	var name string
	err := s.pool.QueryRow(c, `SELECT display_name FROM student_credentials WHERE login = $1`, login).Scan(&name)
	exists := err == nil
	if err != nil && !errors.Is(err, pgx.ErrNoRows) {
		return err
	}
	if exists && (delivery == "email" || s.cfg.IsDev()) {
		code, _, err := s.issueReset(c, login, nil)
		if err != nil {
			return err
		}
		if s.cfg.IsDev() {
			slog.Info("password reset code", "login", login, "code", code)
		}
		if delivery == "email" {
			d := resetEmailData{Name: firstName(name), Email: email, Code: code,
				Minutes: int(resetCodeTTL.Minutes()), Year: s.now().In(addisAbaba).Year()}
			d.support(s.pageContacts(c))
			if err := s.sendResetEmail(c, d); err != nil {
				slog.Error("send password reset email", "err", err)
				return httpx.NewError(http.StatusBadGateway, "email_failed", "We couldn't send the email just now. Try again in a minute.")
			}
		}
	}
	if !exists && delivery == "email" && noAccountNotices.allow(login, s.now()) {
		// Like the big services: tell the address owner there is no
		// password (they likely signed up with Google), rather than leave
		// them waiting for a code. The answer to the app stays the same.
		d := resetEmailData{Email: email, NoAccount: true, Year: s.now().In(addisAbaba).Year()}
		d.support(s.pageContacts(c))
		if err := s.sendResetEmail(c, d); err != nil {
			slog.Error("send no-account reset email", "err", err)
		}
	}
	return httpx.OK(c, gin.H{"delivery": delivery, "expires_in_minutes": int(resetCodeTTL.Minutes())})
}

func (s *Server) resetPassword(c *gin.Context) error {
	var in struct {
		Login    string `json:"login" binding:"required,max=254"`
		Code     string `json:"code" binding:"required,len=6,numeric"`
		Password string `json:"password" binding:"required"`
	}
	if err := httpx.Bind(c, &in); err != nil {
		return err
	}
	login, email, phone, ok := normalizeLogin(in.Login)
	if !ok {
		return httpx.Field("login", "Enter a valid email address or Ethiopian phone number.")
	}
	if !validPassword(in.Password) {
		return httpx.Field("password", passwordRule)
	}
	ipKey, loginKey := "ip:"+c.ClientIP(), "login:"+login
	if passwordLimiter.blocked(ipKey, loginKey) {
		return httpx.NewError(http.StatusTooManyRequests, "too_many_attempts", "Too many attempts. Wait 15 minutes and try again.")
	}
	invalid := httpx.Field("code", "The code is wrong or has expired. Request a new one.")

	var studentID, name, hash string
	var expires time.Time
	var attempts int
	err := s.pool.QueryRow(c, `SELECT sc.student_id, sc.display_name, r.code_hash, r.expires_at, r.attempts
		FROM password_resets r JOIN student_credentials sc ON sc.login = r.login WHERE r.login = $1`, login).
		Scan(&studentID, &name, &hash, &expires, &attempts)
	if errors.Is(err, pgx.ErrNoRows) {
		auth.CheckPassword(dummyHash, in.Code)
		passwordLimiter.fail(ipKey, loginKey)
		return invalid
	}
	if err != nil {
		return err
	}
	if attempts >= resetCodeAttempts || s.now().After(expires) {
		_, _ = s.pool.Exec(c, `DELETE FROM password_resets WHERE login = $1`, login)
		return invalid
	}
	if !auth.CheckPassword(hash, in.Code) {
		passwordLimiter.fail(ipKey, loginKey)
		if _, err := s.pool.Exec(c, `UPDATE password_resets SET attempts = attempts + 1 WHERE login = $1`, login); err != nil {
			return err
		}
		return invalid
	}
	newHash, err := auth.HashPassword(in.Password)
	if err != nil {
		return err
	}
	err = pgx.BeginFunc(c, s.pool, func(tx pgx.Tx) error {
		if _, err := tx.Exec(c, `UPDATE student_credentials SET password_hash = $1, password_changed_at = now() WHERE login = $2`, newHash, login); err != nil {
			return err
		}
		_, err := tx.Exec(c, `DELETE FROM password_resets WHERE login = $1`, login)
		return err
	})
	if err != nil {
		return err
	}
	passwordLimiter.reset(loginKey)
	return s.completeSignIn(c, passwordAccount(name, login, email, phone), studentID)
}

// adminPasswordReset issues a code an admin can pass to a student who can't
// receive email, e.g. one who signed up with a phone number.
func (s *Server) adminPasswordReset(c *gin.Context) error {
	var login string
	err := s.pool.QueryRow(c, `SELECT login FROM student_credentials WHERE student_id = $1`, c.Param("id")).Scan(&login)
	if errors.Is(err, pgx.ErrNoRows) {
		return httpx.NotFound("Password account")
	}
	if err != nil {
		return err
	}
	adminID := currentAdmin(c).ID
	code, expires, err := s.issueReset(c, login, &adminID)
	if err != nil {
		return err
	}
	// Issuing a reset code is security sensitive: keep a record.
	if _, err := s.pool.Exec(c, `INSERT INTO audit_events (admin_id, action, resource) VALUES ($1, 'password_reset_code', $2)`,
		adminID, c.Param("id")); err != nil {
		return err
	}
	return httpx.Created(c, gin.H{"login": login, "code": code, "expires_at": expires})
}

// noAccountNotices sends the "no password for this email" notice at most
// once an hour per address, so the form can't be used to flood an inbox.
var noAccountNotices = &noticeLimiter{sent: map[string]time.Time{}}

type noticeLimiter struct {
	mu   sync.Mutex
	sent map[string]time.Time
}

func (l *noticeLimiter) allow(key string, now time.Time) bool {
	l.mu.Lock()
	defer l.mu.Unlock()
	for k, t := range l.sent {
		if now.Sub(t) >= time.Hour {
			delete(l.sent, k)
		}
	}
	if _, ok := l.sent[key]; ok {
		return false
	}
	l.sent[key] = now
	return true
}
