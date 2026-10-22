package api

import (
	"github.com/gin-gonic/gin"

	"victor-contest-go/internal/store"
)

// Premium by default, decided per chapter: a chapter (or a scheduled exam)
// without an access policy needs an approved academic enrollment; admins
// make individual chapters free. A course's own policy only limits who sees
// it (department, stream, school). Past national exam sets are part of the
// free tier (guide: free users get past national exam questions).
const premiumModule = "academic"

// defaultAccessSQL is the access a resource has without a policy row, for a
// query that has the resource's kind as `kind` and, for contests, the
// contest row as `co`.
// Self-paced exams are free here: past national sets are free, and a
// challenge's quizzes and model exams follow the challenge's own rules.
const defaultContestAccessSQL = `CASE WHEN co.kind <> 'contest' OR co.school_id IS NOT NULL THEN 'free' ELSE '` + premiumModule + `' END`

// defaultAccess returns the access a resource without a policy requires.
func (s *Server) defaultAccess(c *gin.Context, kind, id string) (string, error) {
	switch kind {
	case "chapter":
		return premiumModule, nil
	case "contest":
		var access string
		err := s.pool.QueryRow(c, `SELECT `+defaultContestAccessSQL+` FROM contests co WHERE co.id = $1`, id).Scan(&access)
		if err == store.ErrNotFound {
			return "free", nil
		}
		return access, err
	}
	return "free", nil
}

// resourceAccess is what the app shows on a course or contest card.
type resourceAccess struct {
	// Premium: the resource needs an enrollment ("academic" or "training").
	Premium bool   `json:"premium"`
	Module  string `json:"module,omitempty"`
	// Locked: premium, and this student has no approved enrollment for it.
	Locked bool `json:"locked"`
}

// accessFor works out premium/locked for many courses or contests in one
// query. kind is "course" or "contest". A course is premium when any of its
// published chapters is, and locked when any of those is locked.
func (s *Server) accessFor(c *gin.Context, st store.Student, kind string, ids []string) (map[string]resourceAccess, error) {
	out := map[string]resourceAccess{}
	if len(ids) == 0 {
		return out, nil
	}
	query := `WITH r AS (
		SELECT co.id, coalesce(p.access, ` + defaultContestAccessSQL + `) AS access
		FROM contests co LEFT JOIN learning_policies p ON p.resource_type = 'contest' AND p.resource_id = co.id
		WHERE co.id = ANY($1))`
	if kind == "course" {
		// One row per premium module used by the course's chapters.
		query = `WITH r AS (
		SELECT DISTINCT ch.course_id AS id, coalesce(p.access, '` + premiumModule + `') AS access
		FROM chapters ch LEFT JOIN learning_policies p ON p.resource_type = 'chapter' AND p.resource_id = ch.id
		WHERE ch.course_id = ANY($1) AND ch.status = 'published')`
	}
	rows, err := s.pool.Query(c, query+`
		SELECT r.id, r.access, r.access <> 'free' AND EXISTS (SELECT 1 FROM enrollment_requests e
			WHERE e.student_id = $2 AND e.module = r.access AND e.status = 'approved' AND e.expires_at > now())
		FROM r`, ids, st.ID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	for rows.Next() {
		var id, access string
		var paid bool
		if err := rows.Scan(&id, &access, &paid); err != nil {
			return nil, err
		}
		a := out[id]
		if access != "free" {
			a.Premium = true
			a.Module = access
			a.Locked = a.Locked || !paid
		}
		out[id] = a
	}
	return out, rows.Err()
}
