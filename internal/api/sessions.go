package api

import (
	"crypto/rand"
	"encoding/hex"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/jackc/pgx/v5"

	"victor-contest-go/internal/auth"
	"victor-contest-go/internal/httpx"
)

// Signed-in devices. Each sign-in is a session named in the student's token;
// an account keeps at most maxDevices sessions, and a newer sign-in signs out
// the device used least recently (so one paid account can't be shared
// widely). A signed-out device's next request is refused with a message.

const (
	maxDevices = 2
	// How long a server instance trusts a session it has checked; a device
	// signed out elsewhere notices within this time.
	sessionRecheck = 2 * time.Minute
)

// checkedSessions: session ID -> when this instance last confirmed it.
var checkedSessions sync.Map

func newSessionID() string {
	b := make([]byte, 16)
	_, _ = rand.Read(b)
	return hex.EncodeToString(b)
}

func clip(s string, n int) string {
	s = strings.TrimSpace(s)
	if len(s) > n {
		return s[:n]
	}
	return s
}

// startSession records a sign-in from the device in the request headers
// (X-Device-Id, X-Device-Name, X-Device-Platform; older apps send none) and
// signs out the devices over the limit.
func (s *Server) startSession(c *gin.Context, studentID string) (string, error) {
	id := newSessionID()
	deviceID := clip(c.GetHeader("X-Device-Id"), 64)
	var ended []string
	err := pgx.BeginFunc(c, s.pool, func(tx pgx.Tx) error {
		ended = nil
		// One sign-in at a time per account, so two can't both stay.
		if _, err := tx.Exec(c, `SELECT pg_advisory_xact_lock(hashtext('session:' || $1))`, studentID); err != nil {
			return err
		}
		if deviceID != "" {
			ids, err := endedIDs(c, tx, `UPDATE student_sessions SET signed_out_at = now(), reason = 'signed_in_again'
				WHERE student_id = $1 AND device_id = $2 AND signed_out_at IS NULL RETURNING id`, studentID, deviceID)
			if err != nil {
				return err
			}
			ended = append(ended, ids...)
		}
		if _, err := tx.Exec(c, `INSERT INTO student_sessions (id, student_id, device_id, device, platform) VALUES ($1, $2, $3, $4, $5)`,
			id, studentID, deviceID, clip(c.GetHeader("X-Device-Name"), 80), clip(c.GetHeader("X-Device-Platform"), 20)); err != nil {
			return err
		}
		ids, err := endedIDs(c, tx, `UPDATE student_sessions SET signed_out_at = now(), reason = 'device_limit'
			WHERE id IN (SELECT id FROM student_sessions WHERE student_id = $1 AND signed_out_at IS NULL
				ORDER BY last_seen_at DESC, created_at DESC OFFSET $2) RETURNING id`, studentID, maxDevices)
		ended = append(ended, ids...)
		return err
	})
	forgetSessions(ended)
	return id, err
}

// endedIDs runs an UPDATE ... RETURNING id that signs sessions out.
func endedIDs(c *gin.Context, tx pgx.Tx, sql string, args ...any) ([]string, error) {
	rows, err := tx.Query(c, sql, args...)
	if err != nil {
		return nil, err
	}
	return pgx.CollectRows(rows, pgx.RowTo[string])
}

// forgetSessions drops signed-out sessions from this instance's checks at
// once (other instances notice within sessionRecheck).
func forgetSessions(ids []string) {
	for _, id := range ids {
		checkedSessions.Delete(id)
	}
}

// checkSession is the token check for student requests (auth.Tokens).
func (s *Server) checkSession(c *gin.Context, claims *auth.Claims) error {
	if t, ok := checkedSessions.Load(claims.Session); ok && s.now().Sub(t.(time.Time)) < sessionRecheck {
		return nil
	}
	// Confirms the session and notes the device was used, in one statement.
	tag, err := s.pool.Exec(c, `UPDATE student_sessions SET last_seen_at = now()
		WHERE id = $1 AND student_id = $2 AND signed_out_at IS NULL`, claims.Session, claims.Subject)
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 1 {
		checkedSessions.Store(claims.Session, s.now())
		return nil
	}
	checkedSessions.Delete(claims.Session)
	var reason string
	_ = s.pool.QueryRow(c, `SELECT reason FROM student_sessions WHERE id = $1`, claims.Session).Scan(&reason)
	msg := "This device was signed out. Please sign in again."
	switch reason {
	case "device_limit":
		msg = "You were signed out because your account was signed in on another device. An account can be used on " +
			"2 devices at a time."
	case "password_changed":
		msg = "Your password was changed. Please sign in again."
	case "":
		msg = "Your session has expired. Please sign in again."
	}
	return httpx.NewError(http.StatusUnauthorized, "signed_out", msg)
}

// signOutOthers signs out every device of the student but keep.
func signOutOthers(c *gin.Context, tx pgx.Tx, studentID, keep, reason string) error {
	ids, err := endedIDs(c, tx, `UPDATE student_sessions SET signed_out_at = now(), reason = $3
		WHERE student_id = $1 AND id <> $2 AND signed_out_at IS NULL RETURNING id`, studentID, keep, reason)
	forgetSessions(ids)
	return err
}

type deviceSession struct {
	ID         string    `json:"id"`
	Device     string    `json:"device"`
	Platform   string    `json:"platform"`
	SignedInAt time.Time `json:"signed_in_at"`
	LastSeenAt time.Time `json:"last_seen_at"`
	Current    bool      `json:"current"`
}

// GET /me/sessions — the devices signed in to this account.
func (s *Server) mySessions(c *gin.Context) error {
	claims := auth.Caller(c)
	rows, err := s.pool.Query(c, `SELECT id, device, platform, created_at, last_seen_at FROM student_sessions
		WHERE student_id = $1 AND signed_out_at IS NULL ORDER BY last_seen_at DESC`, claims.Subject)
	if err != nil {
		return err
	}
	out := []deviceSession{}
	for rows.Next() {
		var d deviceSession
		if err := rows.Scan(&d.ID, &d.Device, &d.Platform, &d.SignedInAt, &d.LastSeenAt); err != nil {
			return err
		}
		d.Current = d.ID == claims.Session
		out = append(out, d)
	}
	if err := rows.Err(); err != nil {
		return err
	}
	return httpx.OK(c, gin.H{"sessions": out, "limit": maxDevices})
}

// DELETE /me/sessions/:id — signs out one device ("current": this one).
func (s *Server) endMySession(c *gin.Context) error {
	claims := auth.Caller(c)
	id := c.Param("id")
	if id == "current" {
		id = claims.Session
	}
	if id == "" {
		return httpx.NoContent(c) // a token from before device sessions
	}
	tag, err := s.pool.Exec(c, `UPDATE student_sessions SET signed_out_at = now(), reason = 'signed_out'
		WHERE id = $1 AND student_id = $2 AND signed_out_at IS NULL`, id, claims.Subject)
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 && id != claims.Session {
		return httpx.NotFound("Device")
	}
	checkedSessions.Delete(id)
	return httpx.NoContent(c)
}
