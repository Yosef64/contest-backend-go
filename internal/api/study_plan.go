package api

import (
	"errors"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/jackc/pgx/v5"

	"victor-contest-go/internal/httpx"
)

// Study plan: students schedule study sessions on a calendar. Every query
// is scoped to the signed-in student.

type studySession struct {
	ID       string    `json:"id"`
	Title    string    `json:"title"`
	Subject  string    `json:"subject"`
	StartsAt time.Time `json:"starts_at"`
	Minutes  int       `json:"minutes"`
	Notes    string    `json:"notes"`
	Done     bool      `json:"done"`
}

const (
	studyCols        = `id, title, subject, starts_at, minutes, notes, done`
	maxStudySessions = 1000
	maxRepeatWeeks   = 12
)

type studyInput struct {
	Title    string    `json:"title" binding:"required,max=120"`
	Subject  string    `json:"subject" binding:"max=60"`
	StartsAt time.Time `json:"starts_at" binding:"required"`
	Minutes  int       `json:"minutes" binding:"required,min=5,max=600"`
	Notes    string    `json:"notes" binding:"max=1000"`
	// Also create the same session on the following N weeks (create only).
	RepeatWeeks int `json:"repeat_weeks" binding:"min=0,max=12"`
}

func (in *studyInput) validate() error {
	in.Title = strings.TrimSpace(in.Title)
	in.Subject = strings.TrimSpace(in.Subject)
	in.Notes = strings.TrimSpace(in.Notes)
	if in.Title == "" {
		return httpx.Field("title", "Give the session a name.")
	}
	if in.Subject != "" && !subjects[in.Subject] {
		return httpx.Field("subject", "Choose a supported subject.")
	}
	if in.StartsAt.Before(time.Now().AddDate(-1, 0, 0)) || in.StartsAt.After(time.Now().AddDate(2, 0, 0)) {
		return httpx.Field("starts_at", "Choose a date within the next two years.")
	}
	return nil
}

func scanStudy(row pgx.Row) (studySession, error) {
	var s studySession
	err := row.Scan(&s.ID, &s.Title, &s.Subject, &s.StartsAt, &s.Minutes, &s.Notes, &s.Done)
	return s, err
}

// listStudySessions returns sessions starting in [from, to) (RFC 3339);
// both default to a window around today.
func (s *Server) listStudySessions(c *gin.Context) error {
	st, err := s.me(c)
	if err != nil {
		return err
	}
	from, to := time.Now().AddDate(0, -1, 0), time.Now().AddDate(0, 3, 0)
	for name, dst := range map[string]*time.Time{"from": &from, "to": &to} {
		if v := c.Query(name); v != "" {
			t, err := time.Parse(time.RFC3339, v)
			if err != nil {
				return httpx.BadRequest("Use an RFC 3339 time for " + name + ".")
			}
			*dst = t
		}
	}
	if !to.After(from) || to.Sub(from) > 400*24*time.Hour {
		return httpx.BadRequest("Choose a range of up to 400 days.")
	}
	rows, err := s.pool.Query(c, `SELECT `+studyCols+` FROM study_sessions
		WHERE student_id = $1 AND starts_at >= $2 AND starts_at < $3 ORDER BY starts_at`, st.ID, from, to)
	if err != nil {
		return err
	}
	defer rows.Close()
	out := []studySession{}
	for rows.Next() {
		v, err := scanStudy(rows)
		if err != nil {
			return err
		}
		out = append(out, v)
	}
	if err := rows.Err(); err != nil {
		return err
	}
	return httpx.OK(c, gin.H{"sessions": out})
}

func (s *Server) createStudySession(c *gin.Context) error {
	st, err := s.me(c)
	if err != nil {
		return err
	}
	var in studyInput
	if err := httpx.Bind(c, &in); err != nil {
		return err
	}
	if err := in.validate(); err != nil {
		return err
	}
	var count int
	if err := s.pool.QueryRow(c, `SELECT count(*) FROM study_sessions WHERE student_id = $1`, st.ID).Scan(&count); err != nil {
		return err
	}
	if count+1+in.RepeatWeeks > maxStudySessions {
		return httpx.BadRequest("Your study plan is full. Delete old sessions first.")
	}
	tx, err := s.pool.Begin(c)
	if err != nil {
		return err
	}
	defer tx.Rollback(c) //nolint:errcheck
	created := make([]studySession, 0, 1+in.RepeatWeeks)
	for week := 0; week <= in.RepeatWeeks; week++ {
		v, err := scanStudy(tx.QueryRow(c, `INSERT INTO study_sessions (student_id, title, subject, starts_at, minutes, notes)
			VALUES ($1, $2, $3, $4, $5, $6) RETURNING `+studyCols,
			st.ID, in.Title, in.Subject, in.StartsAt.AddDate(0, 0, 7*week), in.Minutes, in.Notes))
		if err != nil {
			return err
		}
		created = append(created, v)
	}
	if err := tx.Commit(c); err != nil {
		return err
	}
	return httpx.Created(c, gin.H{"sessions": created})
}

func (s *Server) updateStudySession(c *gin.Context) error {
	st, err := s.me(c)
	if err != nil {
		return err
	}
	var in struct {
		studyInput
		Done bool `json:"done"`
	}
	if err := httpx.Bind(c, &in); err != nil {
		return err
	}
	if err := in.validate(); err != nil {
		return err
	}
	v, err := scanStudy(s.pool.QueryRow(c, `UPDATE study_sessions SET title = $3, subject = $4, starts_at = $5,
		minutes = $6, notes = $7, done = $8, updated_at = now()
		WHERE id = $1 AND student_id = $2 RETURNING `+studyCols,
		c.Param("id"), st.ID, in.Title, in.Subject, in.StartsAt, in.Minutes, in.Notes, in.Done))
	if errors.Is(err, pgx.ErrNoRows) {
		return httpx.NotFound("Study session")
	}
	if err != nil {
		return err
	}
	return httpx.OK(c, gin.H{"session": v})
}

func (s *Server) deleteStudySession(c *gin.Context) error {
	st, err := s.me(c)
	if err != nil {
		return err
	}
	tag, err := s.pool.Exec(c, `DELETE FROM study_sessions WHERE id = $1 AND student_id = $2`, c.Param("id"), st.ID)
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		return httpx.NotFound("Study session")
	}
	return httpx.NoContent(c)
}
