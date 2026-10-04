package api

import (
	"errors"
	"net/url"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"

	"victor-contest-go/internal/httpx"
)

// isUniqueViolation: the one-pending-request-per-program index was hit.
func isUniqueViolation(err error) bool {
	var pgErr *pgconn.PgError
	return errors.As(err, &pgErr) && pgErr.Code == "23505"
}

// adminEditEnrollment corrects any payment verification record, whatever
// its status: program, payment reference, proof link, decision, access
// expiry and note. Changing the decision records the reviewing admin.
func (s *Server) adminEditEnrollment(c *gin.Context) error {
	a := currentAdmin(c)
	var in struct {
		Module    string `json:"module" binding:"required,oneof=academic training"`
		Reference string `json:"reference" binding:"required,max=120"`
		ProofURL  string `json:"proof_url" binding:"max=1000"`
		Status    string `json:"status" binding:"required,oneof=pending approved rejected"`
		// Last day of access (YYYY-MM-DD), required when approved.
		ExpiresOn string `json:"expires_on"`
		Note      string `json:"note" binding:"max=1000"`
	}
	if err := httpx.Bind(c, &in); err != nil {
		return err
	}
	in.Reference = strings.TrimSpace(in.Reference)
	in.ProofURL = strings.TrimSpace(in.ProofURL)
	in.Note = strings.TrimSpace(in.Note)
	fields := map[string]string{}
	if len(in.Reference) < 3 {
		fields["reference"] = "Enter the payment reference (at least 3 characters)."
	}
	if in.ProofURL != "" {
		if u, err := url.Parse(in.ProofURL); err != nil || (u.Scheme != "https" && u.Scheme != "http") || u.Host == "" {
			fields["proof_url"] = "Enter a full link starting with https://."
		}
	}
	var expires *time.Time
	if in.Status == "approved" {
		d, err := time.Parse("2006-01-02", in.ExpiresOn)
		switch {
		case err != nil:
			fields["expires_on"] = "Choose the last day of access."
		case d.After(time.Now().AddDate(2, 0, 0)):
			fields["expires_on"] = "Access can be granted for up to two years."
		default:
			// Access lasts through the whole chosen day (Addis Ababa time).
			end := time.Date(d.Year(), d.Month(), d.Day(), 23, 59, 59, 0, addisAbaba)
			expires = &end
		}
	}
	if len(fields) > 0 {
		return httpx.Invalid(fields)
	}

	tx, err := s.pool.Begin(c)
	if err != nil {
		return err
	}
	defer tx.Rollback(c) //nolint:errcheck
	var row map[string]any
	err = tx.QueryRow(c, `UPDATE enrollment_requests e SET module = $2, reference = $3, proof_url = $4,
			reviewed_by = CASE WHEN e.status <> $5 OR e.expires_at IS DISTINCT FROM $6 THEN $8 ELSE e.reviewed_by END,
			status = $5, expires_at = $6, note = $7
		WHERE e.id = $1
		RETURNING to_jsonb(e) || jsonb_build_object('student_name', (SELECT name FROM students WHERE id = e.student_id))`,
		c.Param("id"), in.Module, in.Reference, in.ProofURL, in.Status, expires, in.Note, a.ID).Scan(&row)
	if errors.Is(err, pgx.ErrNoRows) {
		return httpx.NotFound("Enrollment request")
	}
	if isUniqueViolation(err) {
		return httpx.Field("status", "This student already has a pending request for that program.")
	}
	if err != nil {
		return err
	}
	if _, err := tx.Exec(c, `INSERT INTO audit_events (admin_id, action, resource) VALUES ($1, 'enrollment_edit', $2)`, a.ID, c.Param("id")); err != nil {
		return err
	}
	if err := tx.Commit(c); err != nil {
		return err
	}
	return httpx.OK(c, gin.H{"enrollment": row})
}

var addisAbaba = func() *time.Location {
	if loc, err := time.LoadLocation("Africa/Addis_Ababa"); err == nil {
		return loc
	}
	return time.FixedZone("EAT", 3*60*60)
}()
