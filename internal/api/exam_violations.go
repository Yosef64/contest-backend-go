package api

import (
	"time"

	"github.com/gin-gonic/gin"

	"victor-contest-go/internal/httpx"
)

// Exam-mode violations. A normal Android phone lets a student unpin the app
// (hold Back + Recents) and no app can prevent it, only notice it. The app
// pauses the exam until it is pinned again, reports each time here, and
// submits the exam itself once the exam's limit is reached. Proctors see the
// log per exam. (Managed school phones are locked for real: see
// docs/exam-mode-devices.md.)

var violationKinds = map[string]bool{"unpinned": true, "left_app": true, "auto_submitted": true}

// POST /contests/:id/exam-violations {kind, at}
func (s *Server) appExamViolation(c *gin.Context) error {
	st, err := s.me(c)
	if err != nil {
		return err
	}
	var in struct {
		Kind string    `json:"kind" binding:"required"`
		At   time.Time `json:"at"`
	}
	if err := httpx.Bind(c, &in); err != nil {
		return err
	}
	if !violationKinds[in.Kind] {
		return httpx.Field("kind", "Unknown violation.")
	}
	co, err := s.store.Contest(c, c.Param("id"))
	if err != nil {
		return storeErr(err, "Exam")
	}
	now := s.now()
	// The phone's time, kept within the exam (offline reports arrive late).
	at := in.At
	if at.IsZero() || at.After(now) {
		at = now
	}
	if at.Before(co.StartTime) {
		at = co.StartTime
	}
	var limit int
	var registered bool
	if err := s.pool.QueryRow(c, `SELECT coalesce((SELECT max_violations FROM exam_mode_settings WHERE contest_id = $1 AND enabled), -1),
			EXISTS (SELECT 1 FROM registrations WHERE contest_id = $1 AND student_id = $2)`, co.ID, st.ID).Scan(&limit, &registered); err != nil {
		return err
	}
	if limit < 0 {
		return httpx.Conflict("not_exam_mode", "This exam doesn't run in exam mode.")
	}
	if !registered {
		return httpx.Forbidden("Register for the exam first.")
	}
	var count int
	if err := s.pool.QueryRow(c, `WITH added AS (
			INSERT INTO exam_violations (contest_id, student_id, kind, at) VALUES ($1, $2, $3, $4) RETURNING 1)
		SELECT (SELECT count(*) FROM exam_violations WHERE contest_id = $1 AND student_id = $2 AND kind <> 'auto_submitted')::int
			+ (CASE WHEN $3 = 'auto_submitted' THEN 0 ELSE 1 END)`, co.ID, st.ID, in.Kind, at).Scan(&count); err != nil {
		return err
	}
	return httpx.Created(c, gin.H{"violations": count, "max_violations": limit})
}

type violationRow struct {
	StudentID string    `json:"student_id"`
	Name      string    `json:"name"`
	School    string    `json:"school"`
	Unpinned  int       `json:"unpinned"`
	LeftApp   int       `json:"left_app"`
	Submitted bool      `json:"auto_submitted"`
	First     time.Time `json:"first_at"`
	Last      time.Time `json:"last_at"`
}

// GET /admin/contests/:id/violations — who broke exam mode, how often.
func (s *Server) adminExamViolations(c *gin.Context) error {
	if err := s.ownsItem(c, "contests", c.Param("id")); err != nil {
		return err
	}
	rows, err := s.pool.Query(c, `SELECT v.student_id, coalesce(st.name, ''), coalesce(st.school, ''),
			count(*) FILTER (WHERE v.kind = 'unpinned')::int,
			count(*) FILTER (WHERE v.kind = 'left_app')::int,
			bool_or(v.kind = 'auto_submitted'),
			min(v.at), max(v.at)
		FROM exam_violations v LEFT JOIN students st ON st.id = v.student_id
		WHERE v.contest_id = $1
		GROUP BY v.student_id, st.name, st.school
		ORDER BY count(*) DESC, max(v.at) DESC`, c.Param("id"))
	if err != nil {
		return err
	}
	defer rows.Close()
	out := []violationRow{}
	for rows.Next() {
		var r violationRow
		if err := rows.Scan(&r.StudentID, &r.Name, &r.School, &r.Unpinned, &r.LeftApp, &r.Submitted, &r.First, &r.Last); err != nil {
			return err
		}
		out = append(out, r)
	}
	if err := rows.Err(); err != nil {
		return err
	}
	var limit *int
	_ = s.pool.QueryRow(c, `SELECT max_violations FROM exam_mode_settings WHERE contest_id = $1`, c.Param("id")).Scan(&limit)
	return httpx.OK(c, gin.H{"violations": out, "max_violations": limit})
}

// studentBreaks counts a student's exam-mode breaks per exam.
func (s *Server) studentBreaks(c *gin.Context, studentID string) (map[string]int, error) {
	rows, err := s.pool.Query(c, `SELECT contest_id, count(*)::int FROM exam_violations
		WHERE student_id = $1 AND kind <> 'auto_submitted' GROUP BY contest_id`, studentID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := map[string]int{}
	for rows.Next() {
		var id string
		var n int
		if err := rows.Scan(&id, &n); err != nil {
			return nil, err
		}
		out[id] = n
	}
	return out, rows.Err()
}
