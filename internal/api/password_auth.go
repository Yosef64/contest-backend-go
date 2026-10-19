package api

import (
	"errors"
	"net/http"
	"net/mail"
	"regexp"
	"strings"
	"sync"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"victor-contest-go/internal/auth"
	"victor-contest-go/internal/httpx"
)

// Email / phone + password accounts for students.

var (
	phoneSeparators = regexp.MustCompile(`[\s\-().]`)
	ethioPhone      = regexp.MustCompile(`^(?:\+?251|0)?([79]\d{8})$`)
	hasLetter       = regexp.MustCompile(`[A-Za-z]`)
	hasDigit        = regexp.MustCompile(`\d`)
)

// normalizeLogin accepts an email address or an Ethiopian mobile number
// (09…, 07…, +2519…, 2519…) and returns a canonical login.
func normalizeLogin(raw string) (login, email, phone string, ok bool) {
	raw = strings.TrimSpace(raw)
	if strings.Contains(raw, "@") {
		addr, err := mail.ParseAddress(raw)
		domain := raw[strings.LastIndex(raw, "@")+1:]
		if err != nil || addr.Address != raw || len(raw) > 254 ||
			!strings.Contains(domain, ".") || strings.HasSuffix(domain, ".") {
			return "", "", "", false
		}
		e := strings.ToLower(raw)
		return e, e, "", true
	}
	m := ethioPhone.FindStringSubmatch(phoneSeparators.ReplaceAllString(raw, ""))
	if m == nil {
		return "", "", "", false
	}
	p := "+251" + m[1]
	return p, "", p, true
}

func validPassword(pw string) bool {
	return len(pw) >= 8 && len(pw) <= 72 && hasLetter.MatchString(pw) && hasDigit.MatchString(pw)
}

const passwordRule = "Use 8 to 72 characters with at least one letter and one number."

// loginLimiter slows password guessing: after too many failures for the same
// key within 15 minutes, further attempts are refused until the window
// passes. In-memory, so it applies per API instance.
type loginLimiter struct {
	mu   sync.Mutex
	hits map[string][]time.Time
}

const (
	limitWindow = 15 * time.Minute
	// Per login: stops guessing one account's password.
	limitPerLogin = 8
	// Per address: higher, because a whole school may share one IP.
	limitPerAddress = 30
)

func limitFor(key string) int {
	if strings.HasPrefix(key, "ip:") {
		return limitPerAddress
	}
	return limitPerLogin
}

func (l *loginLimiter) blocked(keys ...string) bool {
	l.mu.Lock()
	defer l.mu.Unlock()
	cutoff := time.Now().Add(-limitWindow)
	for _, k := range keys {
		kept := l.hits[k][:0]
		for _, t := range l.hits[k] {
			if t.After(cutoff) {
				kept = append(kept, t)
			}
		}
		if len(kept) == 0 {
			delete(l.hits, k)
			continue
		}
		l.hits[k] = kept
		if len(kept) >= limitFor(k) {
			return true
		}
	}
	return false
}

func (l *loginLimiter) fail(keys ...string) {
	l.mu.Lock()
	defer l.mu.Unlock()
	if l.hits == nil {
		l.hits = map[string][]time.Time{}
	}
	for _, k := range keys {
		l.hits[k] = append(l.hits[k], time.Now())
	}
}

func (l *loginLimiter) reset(key string) {
	l.mu.Lock()
	defer l.mu.Unlock()
	delete(l.hits, key)
}

var passwordLimiter = &loginLimiter{}

func passwordAccount(name, login, email, phone string) account {
	return account{Provider: "password", ID: login, DisplayName: name, Email: email, Username: phone}
}

func (s *Server) signUpPassword(c *gin.Context) error {
	var in struct {
		Name     string `json:"name" binding:"required,min=2,max=80"`
		Login    string `json:"login" binding:"required,max=254"`
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
	hash, err := auth.HashPassword(in.Password)
	if err != nil {
		return err
	}
	if email != "" {
		// One email, one account: a Google account adds a password with
		// Forgot password (the emailed code proves the email is theirs).
		var google bool
		if err := s.pool.QueryRow(c, `SELECT EXISTS (SELECT 1 FROM google_accounts WHERE email = $1)`, login).Scan(&google); err != nil {
			return err
		}
		if google {
			return httpx.Field("login", "This email is already used with Google. Tap Continue with Google, or use Forgot password to add a password.")
		}
	}
	name := strings.TrimSpace(in.Name)
	studentID := "p_" + strings.ReplaceAll(uuid.NewString(), "-", "")
	tag, err := s.pool.Exec(c, `INSERT INTO student_credentials (login, student_id, display_name, password_hash)
		VALUES ($1, $2, $3, $4) ON CONFLICT (login) DO NOTHING`, login, studentID, name, hash)
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		return httpx.Field("login", "An account with this email or phone already exists. Sign in instead.")
	}
	return s.completeSignIn(c, passwordAccount(name, login, email, phone), studentID)
}

func (s *Server) signInPassword(c *gin.Context) error {
	var in struct {
		Login    string `json:"login" binding:"required,max=254"`
		Password string `json:"password" binding:"required,max=72"`
	}
	if err := httpx.Bind(c, &in); err != nil {
		return err
	}
	login, email, phone, ok := normalizeLogin(in.Login)
	if !ok {
		return httpx.Field("login", "Enter a valid email address or Ethiopian phone number.")
	}
	ipKey, loginKey := "ip:"+c.ClientIP(), "login:"+login
	if passwordLimiter.blocked(ipKey, loginKey) {
		return httpx.NewError(http.StatusTooManyRequests, "too_many_attempts", "Too many sign-in attempts. Wait 15 minutes and try again.")
	}
	var studentID, name, hash string
	err := s.pool.QueryRow(c, `SELECT student_id, display_name, password_hash FROM student_credentials WHERE login = $1`, login).
		Scan(&studentID, &name, &hash)
	if errors.Is(err, pgx.ErrNoRows) {
		auth.CheckPassword(dummyHash, in.Password) // same cost as a real check
		passwordLimiter.fail(ipKey, loginKey)
		return httpx.Unauthorized("The email/phone or password is incorrect.")
	}
	if err != nil {
		return err
	}
	if !auth.CheckPassword(hash, in.Password) {
		passwordLimiter.fail(ipKey, loginKey)
		return httpx.Unauthorized("The email/phone or password is incorrect.")
	}
	passwordLimiter.reset(loginKey)
	return s.completeSignIn(c, passwordAccount(name, login, email, phone), studentID)
}

// changeStudentPassword lets a password-account student set a new password
// from account settings. Google and Telegram accounts have no password here.
func (s *Server) changeStudentPassword(c *gin.Context) error {
	var in struct {
		Current  string `json:"current_password" binding:"required,max=72"`
		Password string `json:"password" binding:"required"`
	}
	if err := httpx.Bind(c, &in); err != nil {
		return err
	}
	claims := auth.Caller(c)
	if claims.Provider != "password" {
		return httpx.BadRequest("This account signs in with " + claims.Provider + " and has no Victory password.")
	}
	if !validPassword(in.Password) {
		return httpx.Field("password", passwordRule)
	}
	var hash string
	if err := s.pool.QueryRow(c, `SELECT password_hash FROM student_credentials WHERE login = $1 AND student_id = $2`,
		claims.Identity, claims.Subject).Scan(&hash); err != nil {
		return httpx.NotFound("Account")
	}
	if !auth.CheckPassword(hash, in.Current) {
		return httpx.Field("current_password", "Current password is incorrect.")
	}
	newHash, err := auth.HashPassword(in.Password)
	if err != nil {
		return err
	}
	_, err = s.pool.Exec(c, `UPDATE student_credentials SET password_hash = $1, password_changed_at = now() WHERE login = $2`,
		newHash, claims.Identity)
	return orNoContent(c, err)
}
