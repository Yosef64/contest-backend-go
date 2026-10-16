package api

import (
	"context"
	"errors"
	"regexp"
	"strings"
	"sync"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/jackc/pgx/v5"

	"victor-contest-go/internal/httpx"
)

// Subjects are managed by admins (table subjects). "Mixed" is a pseudo
// subject for exams across subjects and is always accepted where allowed.

// subjectIcons are the icon keys the apps can draw.
var subjectIcons = map[string]bool{
	"book": true, "sigma": true, "atom": true, "flask": true, "leaf": true, "languages": true,
	"landmark": true, "globe": true, "trending": true, "scale": true, "brain": true, "cpu": true,
	"code": true, "puzzle": true, "lightbulb": true, "palette": true, "music": true, "heart": true,
	"briefcase": true, "calculator": true, "microscope": true, "compass": true, "pen": true, "users": true,
}

var hexColor = regexp.MustCompile(`^#[0-9A-Fa-f]{6}$`)

// subjectCache keeps the active subject names for a short while; a name it
// doesn't know is checked in the database (another server may just have
// added it).
type subjectCache struct {
	mu    sync.Mutex
	names map[string]bool
	at    time.Time
}

var subjectNames subjectCache

func (s *Server) loadSubjects(ctx context.Context) (map[string]bool, error) {
	subjectNames.mu.Lock()
	defer subjectNames.mu.Unlock()
	if subjectNames.names != nil && time.Since(subjectNames.at) < time.Minute {
		return subjectNames.names, nil
	}
	rows, err := s.pool.Query(ctx, `SELECT name FROM subjects WHERE active`)
	if err != nil {
		return nil, err
	}
	names, err := pgx.CollectRows(rows, pgx.RowTo[string])
	if err != nil {
		return nil, err
	}
	m := make(map[string]bool, len(names))
	for _, n := range names {
		m[n] = true
	}
	subjectNames.names, subjectNames.at = m, time.Now()
	return m, nil
}

func forgetSubjects() {
	subjectNames.mu.Lock()
	subjectNames.names = nil
	subjectNames.mu.Unlock()
}

// isSubject reports whether name is an active subject (or "Mixed", where
// mixed is allowed).
func (s *Server) isSubject(ctx context.Context, name string, mixed bool) bool {
	if name == "Mixed" {
		return mixed
	}
	if name == "" {
		return false
	}
	if names, err := s.loadSubjects(ctx); err == nil && names[name] {
		return true
	}
	var ok bool
	if err := s.pool.QueryRow(ctx, `SELECT EXISTS (SELECT 1 FROM subjects WHERE name = $1 AND active)`, name).Scan(&ok); err != nil {
		return false
	}
	if ok {
		forgetSubjects()
	}
	return ok
}

type subjectRow struct {
	Name     string   `json:"name"`
	Icon     string   `json:"icon"`
	Color    string   `json:"color"`
	Levels   []string `json:"levels"`
	Position int      `json:"position"`
	Active   bool     `json:"active"`
	// Admin listing only: courses, questions and exams using it.
	Uses int `json:"uses"`
}

func (s *Server) listSubjects(c *gin.Context, all bool) ([]subjectRow, error) {
	rows, err := s.pool.Query(c, `SELECT x.name, x.icon, x.color, x.levels, x.position, x.active,
			((SELECT count(*) FROM courses WHERE subject = x.name) + (SELECT count(*) FROM questions WHERE subject = x.name)
			 + (SELECT count(*) FROM contests WHERE subject = x.name))::int
		FROM subjects x WHERE $1 OR x.active ORDER BY x.position, x.name`, all)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []subjectRow{}
	for rows.Next() {
		var r subjectRow
		if err := rows.Scan(&r.Name, &r.Icon, &r.Color, &r.Levels, &r.Position, &r.Active, &r.Uses); err != nil {
			return nil, err
		}
		if r.Levels == nil {
			r.Levels = []string{}
		}
		out = append(out, r)
	}
	return out, rows.Err()
}

// GET /subjects — active subjects for the apps (no sign-in needed).
func (s *Server) publicSubjects(c *gin.Context) error {
	list, err := s.listSubjects(c, false)
	if err != nil {
		return err
	}
	for i := range list {
		list[i].Uses = 0
	}
	return httpx.OK(c, gin.H{"subjects": list})
}

// GET /admin/subjects — all subjects with how much content uses them.
func (s *Server) adminSubjects(c *gin.Context) error {
	list, err := s.listSubjects(c, true)
	if err != nil {
		return err
	}
	return httpx.OK(c, gin.H{"subjects": list})
}

type subjectInput struct {
	Name     *string  `json:"name"`
	Icon     *string  `json:"icon"`
	Color    *string  `json:"color"`
	Levels   []string `json:"levels"`
	Position *int     `json:"position"`
	Active   *bool    `json:"active"`
}

func (in *subjectInput) validate(creating bool) error {
	fields := map[string]string{}
	if in.Name != nil || creating {
		name := ""
		if in.Name != nil {
			name = strings.Join(strings.Fields(*in.Name), " ")
			in.Name = &name
		}
		switch {
		case len(name) < 2 || len(name) > 60:
			fields["name"] = "Use 2 to 60 characters."
		case strings.EqualFold(name, "Mixed"):
			fields["name"] = "\"Mixed\" is reserved for exams across subjects."
		}
	}
	if in.Icon != nil && !subjectIcons[*in.Icon] {
		fields["icon"] = "Choose one of the icons."
	}
	if in.Color != nil && *in.Color != "" && !hexColor.MatchString(*in.Color) {
		fields["color"] = "Use a colour like #13814F."
	}
	for _, l := range in.Levels {
		if questionLevel(l) == "" {
			fields["levels"] = "Levels are 9, 10, 11, 12, remedial or freshman."
		}
	}
	if len(fields) > 0 {
		return httpx.Invalid(fields)
	}
	return nil
}

func normalizeLevels(levels []string) []string {
	out := []string{}
	seen := map[string]bool{}
	for _, l := range levels {
		if l = questionLevel(l); l != "" && !seen[l] {
			seen[l] = true
			out = append(out, l)
		}
	}
	return out
}

// POST /admin/subjects
func (s *Server) adminCreateSubject(c *gin.Context) error {
	var in subjectInput
	if err := httpx.Bind(c, &in); err != nil {
		return err
	}
	if err := in.validate(true); err != nil {
		return err
	}
	icon, color := "book", ""
	if in.Icon != nil {
		icon = *in.Icon
	}
	if in.Color != nil {
		color = *in.Color
	}
	_, err := s.pool.Exec(c, `INSERT INTO subjects (name, icon, color, levels, position)
		VALUES ($1, $2, $3, $4, coalesce((SELECT max(position) FROM subjects), 0) + 1)`,
		*in.Name, icon, color, normalizeLevels(in.Levels))
	if err != nil {
		if strings.Contains(err.Error(), "duplicate key") {
			return httpx.Field("name", "This subject already exists.")
		}
		return err
	}
	forgetSubjects()
	return s.adminSubjects(c)
}

// PATCH /admin/subjects/:name — rename (content follows), change the icon,
// colour, levels or order, or hide/show it.
func (s *Server) adminUpdateSubject(c *gin.Context) error {
	var in subjectInput
	if err := httpx.Bind(c, &in); err != nil {
		return err
	}
	if err := in.validate(false); err != nil {
		return err
	}
	old := c.Param("name")
	err := pgx.BeginFunc(c, s.pool, func(tx pgx.Tx) error {
		var exists bool
		if err := tx.QueryRow(c, `SELECT EXISTS (SELECT 1 FROM subjects WHERE name = $1)`, old).Scan(&exists); err != nil {
			return err
		}
		if !exists {
			return httpx.NotFound("Subject")
		}
		if _, err := tx.Exec(c, `UPDATE subjects SET icon = coalesce($2, icon), color = coalesce($3, color),
				levels = coalesce($4, levels), position = coalesce($5, position), active = coalesce($6, active)
			WHERE name = $1`, old, in.Icon, in.Color, nilIfEmpty(in.Levels), in.Position, in.Active); err != nil {
			return err
		}
		if in.Name == nil || *in.Name == old {
			return nil
		}
		name := *in.Name
		var taken bool
		if err := tx.QueryRow(c, `SELECT EXISTS (SELECT 1 FROM subjects WHERE name = $1)`, name).Scan(&taken); err != nil {
			return err
		}
		if taken {
			return httpx.Field("name", "This subject already exists.")
		}
		// The new row first, content moved over, then the old row removed.
		if _, err := tx.Exec(c, `INSERT INTO subjects (name, icon, color, levels, position, active, created_at)
			SELECT $2, icon, color, levels, position, active, created_at FROM subjects WHERE name = $1`, old, name); err != nil {
			return err
		}
		for _, q := range []string{
			`UPDATE courses SET subject = $2 WHERE subject = $1`,
			`UPDATE questions SET subject = $2 WHERE subject = $1`,
			`UPDATE contests SET subject = $2 WHERE subject = $1`,
			`UPDATE score_targets SET subject = $2 WHERE subject = $1`,
			`UPDATE study_sessions SET subject = $2 WHERE subject = $1`,
		} {
			if _, err := tx.Exec(c, q, old, name); err != nil {
				return err
			}
		}
		_, err := tx.Exec(c, `DELETE FROM subjects WHERE name = $1`, old)
		return err
	})
	var httpErr *httpx.Error
	if errors.As(err, &httpErr) {
		return err
	}
	if err != nil {
		return err
	}
	forgetSubjects()
	return s.adminSubjects(c)
}

func nilIfEmpty(levels []string) any {
	if levels == nil {
		return nil
	}
	return normalizeLevels(levels)
}

// DELETE /admin/subjects/:name — only while nothing uses it (hide it
// otherwise).
func (s *Server) adminDeleteSubject(c *gin.Context) error {
	name := c.Param("name")
	var uses int
	if err := s.pool.QueryRow(c, `SELECT ((SELECT count(*) FROM courses WHERE subject = $1) + (SELECT count(*) FROM questions WHERE subject = $1)
		+ (SELECT count(*) FROM contests WHERE subject = $1))::int`, name).Scan(&uses); err != nil {
		return err
	}
	if uses > 0 {
		return httpx.Conflict("subject_in_use", "Courses, questions or exams use this subject. Hide it instead.")
	}
	tag, err := s.pool.Exec(c, `DELETE FROM subjects WHERE name = $1`, name)
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		return httpx.NotFound("Subject")
	}
	forgetSubjects()
	return s.adminSubjects(c)
}
