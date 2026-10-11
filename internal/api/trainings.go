package api

import (
	"errors"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/jackc/pgx/v5"

	"victor-contest-go/internal/content"
	"victor-contest-go/internal/httpx"
)

// Training programs: standalone lessons (text and video) on study strategies,
// timetables, time management and habits. They have no chapters, and need the
// separate "training" enrollment unless an admin makes one free.

type training struct {
	ID        string    `json:"id" db:"id"`
	Title     string    `json:"title" db:"title"`
	Summary   string    `json:"summary" db:"summary"`
	Category  string    `json:"category" db:"category"`
	Minutes   int       `json:"minutes" db:"minutes"`
	HTML      string    `json:"html,omitempty" db:"html"`
	Access    string    `json:"access" db:"access"`
	Status    string    `json:"status" db:"status"`
	Position  int       `json:"position" db:"position"`
	Version   int       `json:"version" db:"version"`
	UpdatedAt time.Time `json:"updated_at" db:"updated_at"`
	UpdatedBy string    `json:"updated_by" db:"updated_by"`
}

const trainingCols = `id, title, summary, category, minutes, html, access, status, position, version, updated_at, updated_by`

const maxTrainingHTML = 512 << 10

func (s *Server) loadTraining(c *gin.Context, id string) (training, error) {
	rows, err := s.pool.Query(c, `SELECT `+trainingCols+` FROM trainings WHERE id = $1`, id)
	if err != nil {
		return training{}, err
	}
	t, err := pgx.CollectExactlyOneRow(rows, pgx.RowToStructByName[training])
	if errors.Is(err, pgx.ErrNoRows) {
		return training{}, httpx.NotFound("Training")
	}
	return t, err
}

// Admin ------------------------------------------------------------------------

func (s *Server) adminTrainings(c *gin.Context) error {
	rows, err := s.pool.Query(c, `SELECT `+trainingCols+` FROM trainings ORDER BY position, created_at`)
	if err != nil {
		return err
	}
	list, err := pgx.CollectRows(rows, pgx.RowToStructByName[training])
	if err != nil {
		return err
	}
	for i := range list {
		list[i].HTML = "" // the list doesn't need the content
	}
	return httpx.OK(c, gin.H{"trainings": list})
}

func (s *Server) adminCreateTraining(c *gin.Context) error {
	var in struct {
		Title    string `json:"title" binding:"required,max=160"`
		Summary  string `json:"summary" binding:"max=600"`
		Category string `json:"category" binding:"max=80"`
	}
	if err := httpx.Bind(c, &in); err != nil {
		return err
	}
	title := strings.TrimSpace(in.Title)
	if title == "" {
		return httpx.Field("title", "This field is required.")
	}
	var id string
	if err := s.pool.QueryRow(c, `INSERT INTO trainings (title, summary, category, position, updated_by)
		VALUES ($1, $2, $3, (SELECT coalesce(max(position), 0) + 1 FROM trainings), $4) RETURNING id`,
		title, strings.TrimSpace(in.Summary), strings.TrimSpace(in.Category), currentAdmin(c).Name).Scan(&id); err != nil {
		return err
	}
	t, err := s.loadTraining(c, id)
	if err != nil {
		return err
	}
	return httpx.Created(c, gin.H{"training": t})
}

func (s *Server) adminTraining(c *gin.Context) error {
	t, err := s.loadTraining(c, c.Param("id"))
	if err != nil {
		return err
	}
	return httpx.OK(c, gin.H{"training": t})
}

// adminSaveTraining updates any of the fields. base_version must match the
// stored version, so two admins can't overwrite each other's work unseen.
func (s *Server) adminSaveTraining(c *gin.Context) error {
	var in struct {
		Title       *string `json:"title" binding:"omitempty,max=160"`
		Summary     *string `json:"summary" binding:"omitempty,max=600"`
		Category    *string `json:"category" binding:"omitempty,max=80"`
		Minutes     *int    `json:"minutes" binding:"omitempty,min=1,max=600"`
		HTML        *string `json:"html"`
		Access      *string `json:"access" binding:"omitempty,oneof=free training"`
		Status      *string `json:"status" binding:"omitempty,oneof=draft published"`
		BaseVersion int     `json:"base_version" binding:"required,min=1"`
	}
	if err := httpx.Bind(c, &in); err != nil {
		return err
	}
	t, err := s.loadTraining(c, c.Param("id"))
	if err != nil {
		return err
	}
	if in.BaseVersion != t.Version {
		return httpx.NewError(http.StatusConflict, "version_conflict", "Someone else saved this training. Reload to see their changes.")
	}
	if in.Title != nil {
		if t.Title = strings.TrimSpace(*in.Title); t.Title == "" {
			return httpx.Field("title", "This field is required.")
		}
	}
	if in.Summary != nil {
		t.Summary = strings.TrimSpace(*in.Summary)
	}
	if in.Category != nil {
		t.Category = strings.TrimSpace(*in.Category)
	}
	if in.Minutes != nil {
		t.Minutes = *in.Minutes
	}
	if in.Access != nil {
		t.Access = *in.Access
	}
	if in.Status != nil {
		t.Status = *in.Status
	}
	if in.HTML != nil {
		if len(*in.HTML) > maxTrainingHTML {
			return httpx.Field("html", "This training is too long. Split it into two trainings.")
		}
		t.HTML = content.SanitizeHTML(*in.HTML)
	}
	tag, err := s.pool.Exec(c, `UPDATE trainings SET title = $2, summary = $3, category = $4, minutes = $5, html = $6,
		access = $7, status = $8, version = version + 1, updated_at = now(), updated_by = $9
		WHERE id = $1 AND version = $10`,
		t.ID, t.Title, t.Summary, t.Category, t.Minutes, t.HTML, t.Access, t.Status, currentAdmin(c).Name, in.BaseVersion)
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		return httpx.NewError(http.StatusConflict, "version_conflict", "Someone else saved this training. Reload to see their changes.")
	}
	saved, err := s.loadTraining(c, t.ID)
	if err != nil {
		return err
	}
	return httpx.OK(c, gin.H{"training": saved})
}

func (s *Server) adminReorderTrainings(c *gin.Context) error {
	var in struct {
		IDs []string `json:"ids" binding:"required,max=500"`
	}
	if err := httpx.Bind(c, &in); err != nil {
		return err
	}
	if _, err := s.pool.Exec(c, `UPDATE trainings t SET position = o.n
		FROM unnest($1::text[]) WITH ORDINALITY AS o(id, n) WHERE t.id = o.id`, in.IDs); err != nil {
		return err
	}
	return s.adminTrainings(c)
}

func (s *Server) adminDeleteTraining(c *gin.Context) error {
	tag, err := s.pool.Exec(c, `DELETE FROM trainings WHERE id = $1`, c.Param("id"))
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		return httpx.NotFound("Training")
	}
	c.Status(http.StatusNoContent)
	return nil
}

// App --------------------------------------------------------------------------

// trainingEnrolled reports whether the student has an active training enrollment.
func (s *Server) trainingEnrolled(c *gin.Context, studentID string) (bool, error) {
	var ok bool
	err := s.pool.QueryRow(c, `SELECT EXISTS (SELECT 1 FROM enrollment_requests WHERE student_id = $1
		AND module = 'training' AND status = 'approved' AND expires_at > now())`, studentID).Scan(&ok)
	return ok, err
}

type appTraining struct {
	training
	Locked    bool `json:"locked"`
	Completed bool `json:"completed"`
}

// GET /trainings — published trainings with the student's locks and progress.
func (s *Server) appTrainings(c *gin.Context) error {
	st, err := s.me(c)
	if err != nil {
		return err
	}
	enrolled, err := s.trainingEnrolled(c, st.ID)
	if err != nil {
		return err
	}
	rows, err := s.pool.Query(c, `SELECT `+trainingCols+` FROM trainings WHERE status = 'published' ORDER BY position, created_at`)
	if err != nil {
		return err
	}
	list, err := pgx.CollectRows(rows, pgx.RowToStructByName[training])
	if err != nil {
		return err
	}
	done := map[string]bool{}
	prows, err := s.pool.Query(c, `SELECT training_id FROM training_progress WHERE student_id = $1`, st.ID)
	if err != nil {
		return err
	}
	for prows.Next() {
		var id string
		if err := prows.Scan(&id); err != nil {
			prows.Close()
			return err
		}
		done[id] = true
	}
	prows.Close()
	out := make([]appTraining, len(list))
	for i, t := range list {
		t.HTML = ""
		out[i] = appTraining{training: t, Locked: t.Access != "free" && !enrolled, Completed: done[t.ID]}
	}
	return httpx.OKCached(c, gin.H{"trainings": out, "enrolled": enrolled})
}

// GET /trainings/:id — the content, once the student may read it.
func (s *Server) appTraining(c *gin.Context) error {
	st, err := s.me(c)
	if err != nil {
		return err
	}
	t, err := s.loadTraining(c, c.Param("id"))
	if err != nil {
		return err
	}
	if t.Status != "published" {
		return httpx.NotFound("Training")
	}
	if t.Access != "free" {
		enrolled, err := s.trainingEnrolled(c, st.ID)
		if err != nil {
			return err
		}
		if !enrolled {
			return httpx.NewError(http.StatusForbidden, "enrollment_required", "This training needs a training program enrollment.")
		}
	}
	var completed bool
	if err := s.pool.QueryRow(c, `SELECT EXISTS (SELECT 1 FROM training_progress WHERE student_id = $1 AND training_id = $2)`, st.ID, t.ID).Scan(&completed); err != nil {
		return err
	}
	return httpx.OKCached(c, gin.H{"training": appTraining{training: t, Completed: completed}}, t.ID, strconv.Itoa(t.Version), boolKey(completed))
}

func boolKey(b bool) string {
	if b {
		return "1"
	}
	return "0"
}

// PUT /trainings/:id/progress — mark a training done (or not).
func (s *Server) appTrainingProgress(c *gin.Context) error {
	var in struct {
		Completed bool `json:"completed"`
	}
	if err := httpx.Bind(c, &in); err != nil {
		return err
	}
	st, err := s.me(c)
	if err != nil {
		return err
	}
	if _, err := s.loadTraining(c, c.Param("id")); err != nil {
		return err
	}
	if in.Completed {
		_, err = s.pool.Exec(c, `INSERT INTO training_progress (student_id, training_id) VALUES ($1, $2) ON CONFLICT DO NOTHING`, st.ID, c.Param("id"))
	} else {
		_, err = s.pool.Exec(c, `DELETE FROM training_progress WHERE student_id = $1 AND training_id = $2`, st.ID, c.Param("id"))
	}
	if err != nil {
		return err
	}
	c.Status(http.StatusNoContent)
	return nil
}
