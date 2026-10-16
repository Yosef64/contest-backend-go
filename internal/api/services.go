package api

import (
	"context"
	"encoding/csv"
	"errors"
	"fmt"
	"github.com/gin-gonic/gin"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"log/slog"
	"net/http"
	"strings"
	"time"
	"victor-contest-go/internal/auth"
	"victor-contest-go/internal/content"
	"victor-contest-go/internal/httpx"
	"victor-contest-go/internal/push"
	"victor-contest-go/internal/store"
)

// School managers have no access to global student, content or administrator routes.
func (s *Server) requirePermission(c *gin.Context) {
	a := currentAdmin(c)
	if a.IsOwner {
		c.Next()
		return
	}
	var role string
	var school *string
	if err := s.pool.QueryRow(c, "SELECT permission_role,school_id FROM admins WHERE id=$1", a.ID).Scan(&role, &school); err != nil {
		httpx.Fail(c, httpx.Forbidden("Access unavailable."))
		return
	}
	c.Set("permission_role", role)
	c.Set("manager_school", school)
	path := c.Request.URL.Path
	// Managers may also read (never change) their own school's students; the
	// student handlers scope those reads to the school.
	readsStudents := c.Request.Method == http.MethodGet && (path == "/api/v1/admin/students" || strings.HasPrefix(path, "/api/v1/admin/students/") && strings.Count(path, "/") == 5)
	if role == "manager" && !readsStudents && !strings.HasSuffix(path, "/auth/me") && !strings.Contains(path, "/services/reports") && !strings.HasSuffix(path, "/auth/password") {
		httpx.Fail(c, httpx.Forbidden("School managers can access their school reports and students only."))
		return
	}
	if role == "analyst" && c.Request.Method != http.MethodGet && !strings.HasSuffix(path, "/auth/password") {
		httpx.Fail(c, httpx.Forbidden("Analysts have read-only access."))
		return
	}
	if strings.HasPrefix(path, "/api/v1/admin/admins") && c.Request.Method != http.MethodGet {
		httpx.Fail(c, httpx.Forbidden("Only the owner manages administrator access."))
		return
	}
	c.Next()
}

// managerSchool reports whether the caller is a school manager and, if so,
// the school they are limited to. A manager without a school gets an id that
// matches no student rather than an empty filter, which would match all.
func managerSchool(c *gin.Context) (string, bool) {
	if role, _ := c.Get("permission_role"); role != "manager" {
		return "", false
	}
	v, _ := c.Get("manager_school")
	if id, _ := v.(*string); id != nil && *id != "" {
		return *id, true
	}
	return "unassigned-school", true
}

func (s *Server) checkLearningAccess(c *gin.Context, st store.Student, kind, id string) error {
	if kind == "contest" {
		if err := s.checkLearningGroup(c, st, kind, id); err != nil {
			return err
		}
	}
	var access, department, stream string
	var school *string
	var referral bool
	err := s.pool.QueryRow(c, "SELECT access,department,stream,school_id,referral_enabled FROM learning_policies WHERE resource_type=$1 AND resource_id=$2", kind, id).Scan(&access, &department, &stream, &school, &referral)
	if err == store.ErrNotFound {
		// No policy: courses and contests are premium by default.
		if access, err = s.defaultAccess(c, kind, id); err != nil {
			return err
		}
		if access == "free" {
			return nil
		}
	} else if err != nil {
		return err
	}
	var dep, str string
	var member *string
	err = s.pool.QueryRow(c, "SELECT department,stream,school_id FROM academic_profiles WHERE student_id=$1", st.ID).Scan(&dep, &str, &member)
	if err != nil && err != store.ErrNotFound {
		return err
	}
	// A stream rule only applies to students who have a stream: grades 9-10
	// study a common curriculum and have none.
	if department != "" && dep != department || stream != "" && str != "" && str != stream || school != nil && (member == nil || *member != *school) {
		return httpx.Forbidden("This resource is assigned to another academic group or school.")
	}
	if access != "free" {
		var paid bool
		if err = s.pool.QueryRow(c, "SELECT EXISTS(SELECT 1 FROM enrollment_requests WHERE student_id=$1 AND module=$2 AND status='approved' AND expires_at>now())", st.ID, access).Scan(&paid); err != nil {
			return err
		}
		if paid {
			return nil
		}
		if !referral || kind != "chapter" {
			return httpx.Forbidden("This is premium content. An approved enrollment is required to open it.")
		}
	}
	if referral {
		var n int
		if err = s.pool.QueryRow(c, "SELECT count(*) FROM referral_visits WHERE owner_id=$1 AND chapter_id=$2", st.ID, id).Scan(&n); err != nil {
			return err
		}
		if n < 2 {
			return httpx.Forbidden("Invite two students to unlock this chapter.")
		}
	}
	return nil
}

func (s *Server) learningHub(c *gin.Context) error {
	st, err := s.me(c)
	if err != nil {
		return err
	}
	out := gin.H{}
	queries := map[string]string{
		"makeup":        "SELECT to_jsonb(m)||jsonb_build_object('title',c.title) FROM makeup_settings m JOIN contests c ON c.id=m.contest_id WHERE c.end_time<now() AND (m.available_until>now() OR EXISTS(SELECT 1 FROM makeup_attempts a WHERE a.contest_id=c.id AND a.student_id=$1 AND a.ends_at>now())) AND c.grade=(SELECT grade FROM students WHERE id=$1) AND NOT EXISTS(SELECT 1 FROM submissions WHERE contest_id=c.id AND student_id=$1)",
		"agent":         "SELECT to_jsonb(a) FROM field_agents a WHERE student_id=$1",
		"profile":       "SELECT to_jsonb(p) FROM academic_profiles p WHERE student_id=$1",
		"enrollments":   "SELECT to_jsonb(p) FROM enrollment_requests p WHERE student_id=$1 ORDER BY created_at DESC",
		"targets":       "SELECT to_jsonb(t)||jsonb_build_object('current_score',coalesce((SELECT round(count(*) FILTER(WHERE (sub.answers->>q.id)::int=q.answer_index)::numeric*100/nullif(count(*),0),1) FROM submissions sub JOIN contests co ON co.id=sub.contest_id JOIN contest_questions cq ON cq.contest_id=co.id JOIN questions q ON q.id=cq.question_id WHERE sub.student_id=$1 AND q.subject=t.subject AND co.end_time<now()),0)) FROM score_targets t WHERE student_id=$1 ORDER BY subject",
		"announcements": "SELECT to_jsonb(a)-'author_id' FROM announcements a WHERE active AND (department='' OR department=(SELECT department FROM academic_profiles WHERE student_id=$1)) AND (school_id IS NULL OR school_id=(SELECT school_id FROM academic_profiles WHERE student_id=$1)) ORDER BY created_at DESC",
		"programs":      "SELECT to_jsonb(p)||jsonb_build_object('started_on',e.started_on,'logs',coalesce((SELECT jsonb_agg(jsonb_build_object('day',l.day,'checked',l.checked) ORDER BY l.day) FROM habit_logs l WHERE l.student_id=$1 AND l.program_id=p.id),'[]'::jsonb)) FROM habit_programs p LEFT JOIN habit_enrollments e ON e.program_id=p.id AND e.student_id=$1 WHERE p.active ORDER BY p.created_at",
	}
	for key, q := range queries {
		rows, e := s.store.ServiceRows(c, q, st.ID)
		if e != nil {
			return e
		}
		if key == "makeup" {
			visible := make([]map[string]any, 0, len(rows))
			for _, row := range rows {
				if s.checkLearningGroup(c, st, "contest", row["contest_id"].(string)) == nil {
					visible = append(visible, row)
				}
			}
			rows = visible
		}
		out[key] = rows
	}
	analytics, err := s.learningAnalytics(c, st.ID)
	if err != nil {
		return err
	}
	out["performance"] = analytics
	return httpx.OK(c, out)
}
func (s *Server) saveAcademicProfile(c *gin.Context) error {
	st, err := s.me(c)
	if err != nil {
		return err
	}
	var in struct {
		Department string `json:"department" binding:"omitempty,oneof=entrance remedial freshman"`
		Stream     string `json:"stream" binding:"omitempty,oneof=natural social"`
		SchoolCode string `json:"school_code" binding:"max=80"`
	}
	if err = httpx.Bind(c, &in); err != nil {
		return err
	}
	grade, department, stream, fields := academicLevel(st.Grade, in.Department, in.Stream)
	if len(fields) > 0 {
		return httpx.Invalid(fields)
	}
	in.Department, in.Stream = department, stream
	var school *string
	if in.SchoolCode != "" {
		var id string
		if err = s.pool.QueryRow(c, "SELECT id FROM schools WHERE join_code=$1", strings.TrimSpace(in.SchoolCode)).Scan(&id); err != nil {
			return httpx.Field("school_code", "The school invitation code is invalid.")
		}
		school = &id
	}
	// Membership cannot be self-switched once assigned; admins perform transfers.
	var existing *string
	err = s.pool.QueryRow(c, "SELECT school_id FROM academic_profiles WHERE student_id=$1", st.ID).Scan(&existing)
	if err != nil && err != store.ErrNotFound {
		return err
	}
	if existing != nil {
		if school != nil && *school != *existing {
			return httpx.Forbidden("Contact an administrator to transfer schools.")
		}
		school = existing
	}
	if grade != st.Grade {
		if _, err = s.pool.Exec(c, "UPDATE students SET grade=$2,updated_at=now() WHERE id=$1", st.ID, grade); err != nil {
			return err
		}
	}
	err = s.store.ServiceExec(c, "INSERT INTO academic_profiles(student_id,department,stream,school_id) VALUES($1,$2,$3,$4) ON CONFLICT(student_id) DO UPDATE SET department=$2,stream=$3,school_id=$4,updated_at=now()", st.ID, in.Department, in.Stream, school)
	return orNoContent(c, err)
}
func (s *Server) requestEnrollment(c *gin.Context) error {
	st, err := s.me(c)
	if err != nil {
		return err
	}
	var in struct {
		Module    string `json:"module" binding:"required,oneof=academic training both"`
		Reference string `json:"reference" binding:"required,min=3,max=120"`
		ProofURL  string `json:"proof_url" binding:"omitempty,url,max=1000"`
	}
	if err = httpx.Bind(c, &in); err != nil {
		return err
	}
	// The "both" plan is one request per plan, with the same reference, so
	// the academy reviews each.
	err = pgx.BeginFunc(c, s.pool, func(tx pgx.Tx) error {
		for _, module := range planModules(in.Module) {
			if _, err := tx.Exec(c, "INSERT INTO enrollment_requests(student_id,module,reference,proof_url) VALUES($1,$2,$3,$4)", st.ID, module, strings.TrimSpace(in.Reference), in.ProofURL); err != nil {
				return err
			}
		}
		return nil
	})
	var pgErr *pgconn.PgError
	if errors.As(err, &pgErr) && pgErr.Code == "23505" {
		return httpx.Conflict("already_pending", "You already have a payment waiting for review for this plan.")
	}
	return orNoContent(c, err)
}
func (s *Server) saveTarget(c *gin.Context) error {
	st, err := s.me(c)
	if err != nil {
		return err
	}
	var in struct {
		Subject string `json:"subject" binding:"required,max=120"`
		Target  int    `json:"target" binding:"required,min=1,max=100"`
	}
	if err = httpx.Bind(c, &in); err != nil {
		return err
	}
	if !subjectOK(in.Subject, false) {
		return httpx.Field("subject", "Choose a supported subject.")
	}
	return orNoContent(c, s.store.ServiceExec(c, "INSERT INTO score_targets(student_id,subject,target) VALUES($1,$2,$3) ON CONFLICT(student_id,subject) DO UPDATE SET target=$3", st.ID, in.Subject, in.Target))
}
func (s *Server) logHabit(c *gin.Context) error {
	st, err := s.me(c)
	if err != nil {
		return err
	}
	var in struct {
		Checked []int `json:"checked" binding:"max=20"`
	}
	if err = httpx.Bind(c, &in); err != nil {
		return err
	}
	// Paid training is enforced independently from academic enrollment.
	var allowed bool
	if err = s.pool.QueryRow(c, "SELECT EXISTS(SELECT 1 FROM enrollment_requests WHERE student_id=$1 AND module='training' AND status='approved' AND expires_at>now())", st.ID).Scan(&allowed); err != nil {
		return err
	}
	if !allowed {
		return httpx.Forbidden("Approved training enrollment is required.")
	}
	var tasks []string
	if err = s.pool.QueryRow(c, "SELECT tasks FROM habit_programs WHERE id=$1 AND active", c.Param("id")).Scan(&tasks); err != nil {
		return httpx.NotFound("Program")
	}
	seen := map[int]bool{}
	for _, i := range in.Checked {
		if i < 0 || i >= len(tasks) || seen[i] {
			return httpx.BadRequest("Choose valid, unique checklist items.")
		}
		seen[i] = true
	}
	tx, err := s.pool.Begin(c)
	if err != nil {
		return err
	}
	defer tx.Rollback(c)
	if _, err = tx.Exec(c, "INSERT INTO habit_enrollments(student_id,program_id) VALUES($1,$2) ON CONFLICT DO NOTHING", st.ID, c.Param("id")); err != nil {
		return err
	}
	var valid bool
	if err = tx.QueryRow(c, "SELECT (now() AT TIME ZONE 'Africa/Addis_Ababa')::date < started_on+30 FROM habit_enrollments WHERE student_id=$1 AND program_id=$2", st.ID, c.Param("id")).Scan(&valid); err != nil {
		return err
	}
	if !valid {
		return httpx.Conflict("program_ended", "Your 30-day program has finished.")
	}
	checked := in.Checked
	if checked == nil {
		checked = []int{}
	}
	if _, err = tx.Exec(c, "INSERT INTO habit_logs(student_id,program_id,day,checked) VALUES($1,$2,(now() AT TIME ZONE 'Africa/Addis_Ababa')::date,$3) ON CONFLICT(student_id,program_id,day) DO UPDATE SET checked=$3", st.ID, c.Param("id"), checked); err != nil {
		return err
	}
	return orNoContent(c, tx.Commit(c))
}
func (s *Server) referralVisit(c *gin.Context) error {
	st, err := s.me(c)
	if err != nil {
		return err
	}
	var in struct {
		OwnerID   string `json:"owner_id" binding:"required,max=120"`
		ChapterID string `json:"chapter_id" binding:"required,max=120"`
	}
	if err = httpx.Bind(c, &in); err != nil {
		return err
	}
	if in.OwnerID == st.ID {
		return httpx.BadRequest("Your own visit does not count.")
	}
	var enabled bool
	if err = s.pool.QueryRow(c, "SELECT EXISTS(SELECT 1 FROM learning_policies WHERE resource_type='chapter' AND resource_id=$1 AND referral_enabled)", in.ChapterID).Scan(&enabled); err != nil {
		return err
	}
	if !enabled {
		return httpx.BadRequest("Referrals are not enabled for this chapter.")
	}
	_, err = s.pool.Exec(c, "INSERT INTO referral_visits(owner_id,visitor_id,chapter_id) VALUES($1,$2,$3) ON CONFLICT DO NOTHING", in.OwnerID, st.ID, in.ChapterID)
	return orNoContent(c, storeErr(err, "Referral"))
}
func (s *Server) adminServices(c *gin.Context) error {
	out := gin.H{}
	queries := map[string]string{
		"schools":        "SELECT to_jsonb(x) FROM schools x ORDER BY name",
		"enrollments":    "SELECT to_jsonb(e)||jsonb_build_object('student_name',s.name) FROM enrollment_requests e JOIN students s ON s.id=e.student_id ORDER BY e.created_at DESC",
		"announcements":  "SELECT to_jsonb(x) FROM announcements x ORDER BY created_at DESC",
		"programs":       "SELECT to_jsonb(x) FROM habit_programs x ORDER BY created_at DESC",
		"exam_modes":     "SELECT jsonb_build_object('contest_id',x.contest_id,'title',c.title,'enabled',x.enabled) FROM exam_mode_settings x JOIN contests c ON c.id=x.contest_id",
		"makeup":         "SELECT to_jsonb(x)||jsonb_build_object('title',c.title) FROM makeup_settings x JOIN contests c ON c.id=x.contest_id ORDER BY available_until DESC",
		"habit_activity": "SELECT jsonb_build_object('student_id',s.id,'student_name',s.name,'program',p.title,'started_on',e.started_on,'days_logged',count(l.day),'tasks_checked',coalesce(sum(cardinality(l.checked)),0),'last_log',max(l.day)) FROM habit_enrollments e JOIN students s ON s.id=e.student_id JOIN habit_programs p ON p.id=e.program_id LEFT JOIN habit_logs l ON l.student_id=e.student_id AND l.program_id=e.program_id GROUP BY s.id,p.id,e.started_on ORDER BY max(l.day) NULLS FIRST",
		"agents":         "SELECT to_jsonb(x) FROM field_agents x ORDER BY created_at DESC",
		"policies":       "SELECT to_jsonb(x) FROM learning_policies x ORDER BY resource_type,resource_id",
		"roles":          "SELECT jsonb_build_object('id',id,'name',name,'role',permission_role,'school_id',school_id,'is_owner',is_owner) FROM admins ORDER BY name",
		"audit":          "SELECT to_jsonb(x) FROM audit_events x ORDER BY created_at DESC LIMIT 100",
		"members":        "SELECT jsonb_build_object('id',s.id,'name',s.name,'department',p.department,'stream',p.stream,'school_id',p.school_id) FROM students s LEFT JOIN academic_profiles p ON p.student_id=s.id ORDER BY s.name",
	}
	for k, q := range queries {
		rows, err := s.store.ServiceRows(c, q)
		if err != nil {
			return err
		}
		out[k] = rows
	}
	return httpx.OK(c, out)
}
func (s *Server) adminServiceWrite(c *gin.Context) error {
	a := currentAdmin(c)
	var in struct {
		Kind         string   `json:"kind" binding:"required"`
		ID           string   `json:"id"`
		Title        string   `json:"title" binding:"max=200"`
		Body         string   `json:"body" binding:"max=5000"`
		City         string   `json:"city" binding:"max=80"`
		Region       string   `json:"region" binding:"max=80"`
		Department   string   `json:"department"`
		Stream       string   `json:"stream"`
		SchoolID     *string  `json:"school_id"`
		Tasks        []string `json:"tasks" binding:"max=20"`
		Active       bool     `json:"active"`
		Status       string   `json:"status"`
		Note         string   `json:"note" binding:"max=1000"`
		Days         int      `json:"days"`
		ResourceType string   `json:"resource_type"`
		ResourceID   string   `json:"resource_id"`
		Access       string   `json:"access"`
		Referral     bool     `json:"referral_enabled"`
		Role         string   `json:"role"`
	}
	if err := httpx.Bind(c, &in); err != nil {
		return err
	}
	if in.Department != "" && in.Department != "entrance" && in.Department != "remedial" && in.Department != "freshman" {
		return httpx.BadRequest("Invalid department.")
	}
	if in.Stream != "" && in.Stream != "social" && in.Stream != "natural" {
		return httpx.BadRequest("Invalid stream.")
	}
	var q string
	var args []any
	switch in.Kind {
	case "school":
		if strings.TrimSpace(in.Title) == "" {
			return httpx.BadRequest("School name is required.")
		}
		if in.ID == "" {
			q = "INSERT INTO schools(name,city,region) VALUES($1,$2,$3)"
			args = []any{in.Title, in.City, in.Region}
		} else {
			q = "UPDATE schools SET name=$1,city=$2,region=$3 WHERE id=$4"
			args = []any{in.Title, in.City, in.Region, in.ID}
		}
	case "announcement":
		if strings.TrimSpace(in.Title) == "" || strings.TrimSpace(in.Body) == "" {
			return httpx.BadRequest("Title and message are required.")
		}
		if in.ID == "" {
			q = "INSERT INTO announcements(title,body,department,school_id,active,author_id) VALUES($1,$2,$3,$4,$5,$6)"
			args = []any{in.Title, in.Body, in.Department, in.SchoolID, in.Active, a.ID}
		} else {
			q = "UPDATE announcements SET title=$1,body=$2,department=$3,school_id=$4,active=$5 WHERE id=$6"
			args = []any{in.Title, in.Body, in.Department, in.SchoolID, in.Active, in.ID}
		}
	case "program":
		if strings.TrimSpace(in.Title) == "" || len(in.Tasks) == 0 || len(in.Tasks) > 20 {
			return httpx.BadRequest("Provide a title and 1 to 20 tasks.")
		}
		for _, t := range in.Tasks {
			if strings.TrimSpace(t) == "" || len(t) > 200 {
				return httpx.BadRequest("Tasks must contain 1 to 200 characters.")
			}
		}
		if in.ID == "" {
			q = "INSERT INTO habit_programs(title,tasks,active) VALUES($1,$2,$3)"
			args = []any{in.Title, in.Tasks, in.Active}
		} else {
			q = "UPDATE habit_programs SET title=$1,active=$2 WHERE id=$3"
			args = []any{in.Title, in.Active, in.ID}
		}
	case "enrollment":
		if in.Status != "approved" && in.Status != "rejected" {
			return httpx.BadRequest("Choose approved or rejected.")
		}
		if in.Status == "approved" && (in.Days < 1 || in.Days > 730) {
			return httpx.BadRequest("Enrollment duration must be 1 to 730 days.")
		}
		q = "UPDATE enrollment_requests SET status=$1,note=$2,expires_at=CASE WHEN $1='approved' THEN now()+make_interval(days=>$3) ELSE NULL END,reviewed_by=$4 WHERE id=$5 AND status='pending'"
		args = []any{in.Status, in.Note, in.Days, a.ID, in.ID}
	case "policy":
		if in.ResourceType != "course" && in.ResourceType != "chapter" && in.ResourceType != "contest" {
			return httpx.BadRequest("Invalid resource type.")
		}
		if in.Access != "free" && in.Access != "academic" && in.Access != "training" {
			return httpx.BadRequest("Invalid access level.")
		}
		if in.Referral && in.ResourceType != "chapter" {
			return httpx.BadRequest("Referral locks apply to chapters only.")
		}
		table := map[string]string{"course": "courses", "chapter": "chapters", "contest": "contests"}[in.ResourceType]
		var exists bool
		if err := s.pool.QueryRow(c, "SELECT EXISTS(SELECT 1 FROM "+table+" WHERE id=$1)", in.ResourceID).Scan(&exists); err != nil {
			return err
		}
		if !exists {
			return httpx.NotFound("Resource")
		}
		q = "INSERT INTO learning_policies(resource_type,resource_id,access,department,stream,school_id,referral_enabled) VALUES($1,$2,$3,$4,$5,$6,$7) ON CONFLICT(resource_type,resource_id) DO UPDATE SET access=$3,department=$4,stream=$5,school_id=$6,referral_enabled=$7"
		args = []any{in.ResourceType, in.ResourceID, in.Access, in.Department, in.Stream, in.SchoolID, in.Referral}
	case "role":
		if !a.IsOwner {
			return httpx.Forbidden("Only the owner can assign roles.")
		}
		if in.Role != "editor" && in.Role != "analyst" && in.Role != "manager" {
			return httpx.BadRequest("Invalid role.")
		}
		if in.Role == "manager" && (in.SchoolID == nil || strings.TrimSpace(*in.SchoolID) == "") {
			return httpx.BadRequest("School managers require a school.")
		}
		q = "UPDATE admins SET permission_role=$1,school_id=$2 WHERE id=$3 AND NOT is_owner"
		args = []any{in.Role, in.SchoolID, in.ID}
	case "membership":
		q = "UPDATE academic_profiles SET school_id=$1,updated_at=now() WHERE student_id=$2"
		args = []any{in.SchoolID, in.ID}
	default:
		return httpx.BadRequest("Unknown operation.")
	}
	tx, err := s.pool.Begin(c)
	if err != nil {
		return err
	}
	defer tx.Rollback(c)
	tag, err := tx.Exec(c, q, args...)
	if err != nil {
		return storeErr(err, "Record")
	}
	if tag.RowsAffected() == 0 {
		return httpx.NotFound("Record")
	}
	if _, err = tx.Exec(c, "INSERT INTO audit_events(admin_id,action,resource) VALUES($1,$2,$3)", a.ID, in.Kind, in.ID+in.ResourceID); err != nil {
		return err
	}
	if err := tx.Commit(c); err != nil {
		return err
	}
	// A new announcement notifies its students' phones (edits don't).
	if in.Kind == "announcement" && in.ID == "" && in.Active {
		s.announce(c, in.Title, in.Body, in.Department, in.SchoolID)
	}
	return httpx.NoContent(c)
}

// announce sends a push notification for an announcement. It runs before
// the response: on Lambda nothing runs after it. A failure is only logged.
func (s *Server) announce(c *gin.Context, title, body, department string, schoolID *string) {
	if s.push == nil {
		return
	}
	ctx, cancel := context.WithTimeout(c, 8*time.Second)
	defer cancel()
	topic, condition := push.Audience(department, schoolID)
	if len(body) > 240 {
		body = body[:237] + "…"
	}
	id, err := s.push.Send(ctx, push.Message{Topic: topic, Condition: condition, Title: title, Body: body,
		Data: map[string]string{"type": "announcement"}})
	if err != nil {
		slog.Error("announcement push", "err", err, "topic", topic, "condition", condition)
		return
	}
	slog.Info("announcement push sent", "topic", topic, "condition", condition, "message", id)
}
func (s *Server) changeAdminPassword(c *gin.Context) error {
	var in struct {
		Current  string `json:"current_password" binding:"required"`
		Password string `json:"password" binding:"required,min=8,max=72"`
	}
	if err := httpx.Bind(c, &in); err != nil {
		return err
	}
	a := currentAdmin(c)
	if !auth.CheckPassword(a.PasswordHash, in.Current) {
		return httpx.Forbidden("Current password is incorrect.")
	}
	hash, err := auth.HashPassword(in.Password)
	if err != nil {
		return err
	}
	return orNoContent(c, s.store.ServiceExec(c, "UPDATE admins SET password_hash=$1 WHERE id=$2", hash, a.ID))
}

// reportData runs one report breakdown (group: overall, subject, chapter,
// topic, gender, …) with the request's filters. School managers only ever
// see their own school.
func (s *Server) reportData(c *gin.Context, group string) ([]map[string]any, error) {
	school := c.Query("school_id")
	if role, _ := c.Get("permission_role"); role == "manager" {
		v, _ := c.Get("manager_school")
		id, _ := v.(*string)
		if id == nil {
			return nil, httpx.Forbidden("No school assigned.")
		}
		school = *id
	}
	// Postgres rejects a bare string constant in GROUP BY, hence the cast on "overall".
	column := map[string]string{"overall": "'All students'::text", "school": "s.school", "gender": "s.gender", "city": "s.city", "region": "s.region", "subject": "co.subject", "chapter": "breakdown.subject || ' / ' || breakdown.chapter", "topic": "breakdown.topic", "department": "coalesce(p.department,'unassigned')", "stream": "coalesce(p.stream,'unassigned')",
		// Score distribution: each submission falls in one 20-point band.
		"score": "CASE WHEN sub.total = 0 OR sub.correct*5 < sub.total THEN '0–19%' WHEN sub.correct*5 < sub.total*2 THEN '20–39%' WHEN sub.correct*5 < sub.total*3 THEN '40–59%' WHEN sub.correct*5 < sub.total*4 THEN '60–79%' ELSE '80–100%' END"}[group]
	if column == "" {
		return nil, httpx.BadRequest("Invalid report dimension.")
	}
	grade := c.Query("grade")
	if grade != "" {
		grade = NormalizeGrade(grade)
		if grade == "" {
			return nil, httpx.BadRequest("Invalid report grade.")
		}
	}
	subject := c.Query("subject")
	if subject != "" && !subjectOK(subject, true) {
		return nil, httpx.BadRequest("Invalid report subject.")
	}
	from, until, err := reportDateRange(c.Query("from"), c.Query("to"))
	if err != nil {
		return nil, err
	}
	query := fmt.Sprintf(`SELECT jsonb_build_object('group',%s,'attempts',count(*),'students',count(DISTINCT s.id),'accuracy',round(coalesce(sum(sub.correct)::numeric/nullif(sum(sub.total),0),0)*100,2),'pass',count(*) FILTER(WHERE sub.correct*2>=sub.total AND sub.total>0),'fail',count(*) FILTER(WHERE sub.correct*2<sub.total OR sub.total=0),'average_seconds',round(avg(sub.time_seconds))) FROM submissions sub JOIN students s ON s.id=sub.student_id JOIN contests co ON co.id=sub.contest_id LEFT JOIN academic_profiles p ON p.student_id=s.id WHERE ($1='' OR p.school_id=$1) AND ($2='' OR co.grade=$2) AND ($3='' OR co.subject=$3) AND ($4::timestamptz IS NULL OR sub.submitted_at >= $4) AND ($5::timestamptz IS NULL OR sub.submitted_at < $5) GROUP BY %s ORDER BY %s`, column, column, column)
	if group == "chapter" || group == "topic" {
		// Per-question breakdown: each submission is scored separately for
		// every chapter (or chapter topic) its questions cover. Contests count
		// once they have ended; national exam practice shows answers right
		// after submitting, so it counts at once.
		label, keys := "b.subject||' / '||b.chapter", "b.subject,b.chapter"
		topic, inner := "''::text", "q.chapter"
		if group == "topic" {
			label, keys = "b.subject||' / '||b.chapter||' / '||b.topic", "b.subject,b.chapter,b.topic"
			topic, inner = "coalesce(nullif(q.topic,''),'General')", "q.chapter,q.topic"
		}
		query = `WITH breakdown AS (
SELECT sub.contest_id,sub.student_id,sub.submitted_at,sub.time_seconds,q.subject,coalesce(nullif(q.chapter,''),'Uncategorized') AS chapter,` + topic + ` AS topic,
count(*) AS total,count(*) FILTER(WHERE sub.answers->>q.id=q.answer_index::text) AS correct
FROM submissions sub JOIN contests co ON co.id=sub.contest_id JOIN contest_questions cq ON cq.contest_id=co.id JOIN questions q ON q.id=cq.question_id
WHERE co.end_time<now() OR co.kind='national' GROUP BY sub.contest_id,sub.student_id,sub.submitted_at,sub.time_seconds,q.subject,` + inner + `)
SELECT jsonb_build_object('group',` + label + `,'attempts',count(*),'students',count(DISTINCT b.student_id),
'accuracy',round(sum(b.correct)::numeric/nullif(sum(b.total),0)*100,2),
'pass',count(*) FILTER(WHERE b.correct*2>=b.total),'fail',count(*) FILTER(WHERE b.correct*2<b.total),'average_seconds',round(avg(b.time_seconds)))
FROM breakdown b JOIN students s ON s.id=b.student_id JOIN contests co ON co.id=b.contest_id LEFT JOIN academic_profiles p ON p.student_id=s.id
WHERE ($1='' OR p.school_id=$1) AND ($2='' OR co.grade=$2) AND ($3='' OR b.subject=$3) AND ($4::timestamptz IS NULL OR b.submitted_at >= $4) AND ($5::timestamptz IS NULL OR b.submitted_at < $5)
GROUP BY ` + keys + ` ORDER BY ` + keys
	}
	return s.store.ServiceRows(c, query, school, grade, subject, from, until)
}

func (s *Server) adminReports(c *gin.Context) error {
	rows, err := s.reportData(c, c.DefaultQuery("group", "school"))
	if err != nil {
		return err
	}
	if c.Query("format") == "csv" {
		c.Header("Content-Disposition", "attachment; filename=performance-report.csv")
		c.Header("Content-Type", "text/csv; charset=utf-8")
		w := csv.NewWriter(c.Writer)
		defer w.Flush()
		keys := []string{"group", "attempts", "students", "accuracy", "pass", "fail", "average_seconds"}
		if err = w.Write(keys); err != nil {
			return err
		}
		for _, r := range rows {
			record := []string{}
			for _, k := range keys {
				value := fmt.Sprint(r[k])
				if strings.ContainsAny(value[:min(1, len(value))], "=+-@") {
					value = "'" + value
				}
				record = append(record, value)
			}
			if err = w.Write(record); err != nil {
				return err
			}
		}
		return nil
	}
	return httpx.OK(c, gin.H{"rows": rows, "pass_threshold": 50, "generated_at": time.Now()})
}

// Group isolation also applies to discovery; paid content can remain discoverable.
func (s *Server) checkLearningGroup(c *gin.Context, st store.Student, kind, id string) error {
	if kind == "contest" {
		co, err := s.store.Contest(c, id)
		if err != nil {
			return storeErr(err, "Exam")
		}
		academic, err := s.academicProfile(c, st.ID)
		if err != nil {
			return err
		}
		department, _ := academic["department"].(string)
		stream, _ := academic["stream"].(string)
		if !examAudienceMatches(co, st.Grade, department, stream) {
			return httpx.Forbidden("This exam is for another grade, level or science stream.")
		}
	}
	var permitted bool
	err := s.pool.QueryRow(c, `SELECT NOT EXISTS(SELECT 1 FROM learning_policies p LEFT JOIN academic_profiles a ON a.student_id=$1 WHERE p.resource_type=$2 AND p.resource_id=$3 AND ((p.department<>'' AND p.department<>coalesce(a.department,'')) OR (p.stream<>'' AND coalesce(a.stream,'')<>'' AND p.stream<>a.stream) OR (p.school_id IS NOT NULL AND p.school_id IS DISTINCT FROM a.school_id)))`, st.ID, kind, id).Scan(&permitted)
	if err != nil {
		return err
	}
	if !permitted {
		return httpx.Forbidden("This resource belongs to another academic group or school.")
	}
	return nil
}

func (s *Server) adminQuestionBatch(c *gin.Context) error {
	var in struct {
		IDs     []string `json:"ids" binding:"required,min=1,max=500"`
		Action  string   `json:"action" binding:"required,oneof=delete tag"`
		Subject string   `json:"subject"`
		Chapter string   `json:"chapter" binding:"max=120"`
	}
	if err := httpx.Bind(c, &in); err != nil {
		return err
	}
	if in.Action == "tag" && (!subjectOK(in.Subject, false) || strings.TrimSpace(in.Chapter) == "") {
		return httpx.BadRequest("Choose a subject and chapter.")
	}
	tx, err := s.pool.Begin(c)
	if err != nil {
		return err
	}
	defer tx.Rollback(c)
	// Locks protect against concurrent attachment to a contest.
	rows, err := tx.Query(c, "SELECT id FROM questions WHERE id=ANY($1) FOR UPDATE", in.IDs)
	if err != nil {
		return err
	}
	n := 0
	for rows.Next() {
		n++
	}
	err = rows.Err()
	rows.Close()
	if err != nil {
		return err
	}
	if n != len(in.IDs) {
		return httpx.BadRequest("Questions must exist and be unique.")
	}
	var used bool
	if err = tx.QueryRow(c, "SELECT EXISTS(SELECT 1 FROM contest_questions WHERE question_id=ANY($1))", in.IDs).Scan(&used); err != nil {
		return err
	}
	if used {
		return httpx.Conflict("in_use", "Remove questions from exams before changing or deleting them.")
	}
	q := "DELETE FROM questions WHERE id=ANY($1)"
	args := []any{in.IDs}
	if in.Action == "tag" {
		q = "UPDATE questions SET subject=$2,chapter=$3 WHERE id=ANY($1)"
		args = append(args, in.Subject, in.Chapter)
	}
	if _, err = tx.Exec(c, q, args...); err != nil {
		return err
	}
	if _, err = tx.Exec(c, "INSERT INTO audit_events(admin_id,action,resource) VALUES($1,$2,$3)", currentAdmin(c).ID, "questions."+in.Action, strings.Join(in.IDs, ",")); err != nil {
		return err
	}
	return orNoContent(c, tx.Commit(c))
}

func (s *Server) registerAgent(c *gin.Context) error {
	var in struct{}
	if err := httpx.Bind(c, &in); err != nil {
		return err
	}
	st, err := s.me(c)
	if err != nil {
		return err
	}
	return orNoContent(c, storeErr(s.store.ServiceExec(c, "INSERT INTO field_agents(student_id,name,phone,city,region) VALUES($1,$2,$3,$4,$5)", st.ID, st.Name, st.PhoneNumber, st.City, st.Region), "Agent registration"))
}
func (s *Server) startMakeup(c *gin.Context) error {
	var in struct{}
	if err := httpx.Bind(c, &in); err != nil {
		return err
	}
	st, err := s.me(c)
	if err != nil {
		return err
	}
	co, err := s.store.Contest(c, c.Param("id"))
	if err != nil {
		return storeErr(err, "Exam")
	}
	if err = s.checkLearningAccess(c, st, "contest", co.ID); err != nil {
		return err
	}
	if co.IsNational() {
		return httpx.BadRequest("National exam sets are practice, so there is no makeup exam.")
	}
	if co.Phase != "ended" {
		return httpx.BadRequest("Makeup exams are available after the scheduled exam ends.")
	}
	var ends time.Time
	// Resuming never resets the timer. Existing submissions are ineligible.
	err = s.pool.QueryRow(c, `INSERT INTO makeup_attempts(contest_id,student_id,ends_at) SELECT a.contest_id,$2,a.ends_at FROM makeup_attempts a WHERE a.contest_id=$1 AND a.student_id=$2 AND a.ends_at>now() AND NOT EXISTS(SELECT 1 FROM submissions WHERE contest_id=$1 AND student_id=$2) UNION ALL SELECT m.contest_id,$2,least(now()+make_interval(mins=>m.minutes),m.available_until) FROM makeup_settings m WHERE m.contest_id=$1 AND m.available_until>now() AND NOT EXISTS(SELECT 1 FROM makeup_attempts WHERE contest_id=$1 AND student_id=$2) AND NOT EXISTS(SELECT 1 FROM submissions WHERE contest_id=$1 AND student_id=$2) ON CONFLICT(contest_id,student_id) DO UPDATE SET student_id=EXCLUDED.student_id RETURNING ends_at`, co.ID, st.ID).Scan(&ends)
	if err != nil {
		return httpx.Conflict("makeup_unavailable", "This exam is unavailable or has already been submitted.")
	}
	if !ends.After(s.now()) {
		return httpx.Conflict("makeup_ended", "Your makeup timer has expired.")
	}
	qs, err := s.store.ContestQuestions(c, co.ID)
	if err != nil {
		return err
	}
	return httpx.OK(c, gin.H{"questions": playQuestions(co, st.ID, qs), "ends_at": ends})
}
func (s *Server) submitMakeup(c *gin.Context) error {
	st, err := s.me(c)
	if err != nil {
		return err
	}
	if err = s.checkLearningAccess(c, st, "contest", c.Param("id")); err != nil {
		return err
	}
	var in struct {
		Answers map[string]int `json:"answers"`
	}
	if err = httpx.Bind(c, &in); err != nil {
		return err
	}
	var started, ends time.Time
	if err = s.pool.QueryRow(c, "SELECT started_at,ends_at FROM makeup_attempts WHERE contest_id=$1 AND student_id=$2", c.Param("id"), st.ID).Scan(&started, &ends); err != nil {
		return httpx.Forbidden("Start your makeup exam first.")
	}
	if s.now().After(ends.Add(submitGrace)) {
		return httpx.Conflict("makeup_ended", "Your makeup submission window has expired.")
	}
	co, err := s.store.Contest(c, c.Param("id"))
	if err != nil {
		return storeErr(err, "Exam")
	}
	qs, err := s.store.ContestQuestions(c, co.ID)
	if err != nil {
		return err
	}
	given := unshuffleAnswers(co, st.ID, qs, in.Answers)
	answers := map[string]int{}
	correct := 0
	for _, q := range qs {
		v, ok := given[q.ID]
		if ok && v >= 0 && v < len(q.Options) {
			answers[q.ID] = v
			if v == q.AnswerIndex {
				correct++
			}
		}
	}
	seconds := int(min(s.now().Sub(started), ends.Sub(started)).Seconds())
	sub := store.Submission{ContestID: c.Param("id"), StudentID: st.ID, Answers: answers, Correct: correct, Total: len(qs), TimeSeconds: max(seconds, 0), SubmittedAt: s.now()}
	if err = s.store.CreateSubmission(c, sub); err != nil {
		return storeErr(err, "Submission")
	}
	return httpx.Created(c, gin.H{"submission": summarize(sub, true)})
}

func (s *Server) adminMakeup(c *gin.Context) error {
	var in struct {
		ContestID string    `json:"contest_id" binding:"required"`
		Minutes   int       `json:"minutes" binding:"required,min=1,max=240"`
		Until     time.Time `json:"available_until" binding:"required"`
	}
	if err := httpx.Bind(c, &in); err != nil {
		return err
	}
	co, err := s.store.Contest(c, in.ContestID)
	if err != nil {
		return storeErr(err, "Exam")
	}
	if co.Phase != "ended" || !in.Until.After(s.now()) {
		return httpx.BadRequest("Choose an ended exam and a future makeup deadline.")
	}
	return s.auditServiceChange(c, "makeup.save", in.ContestID, "INSERT INTO makeup_settings(contest_id,minutes,available_until) VALUES($1,$2,$3) ON CONFLICT(contest_id) DO UPDATE SET minutes=$2,available_until=$3", in.ContestID, in.Minutes, in.Until)
}
func (s *Server) adminAgentReview(c *gin.Context) error {
	var in struct {
		Status string `json:"status" binding:"required,oneof=approved rejected"`
	}
	if err := httpx.Bind(c, &in); err != nil {
		return err
	}
	return orNoContent(c, s.store.ServiceExec(c, "UPDATE field_agents SET status=$1 WHERE id=$2", in.Status, c.Param("id")))
}

func (s *Server) adminBulkChapters(c *gin.Context) error {
	var in struct {
		Chapters []struct {
			CourseID string `json:"course_id" binding:"required"`
			Title    string `json:"title" binding:"required,max=200"`
			Summary  string `json:"summary" binding:"max=1000"`
			HTML     string `json:"html" binding:"max=200000"`
			Minutes  int    `json:"minutes" binding:"required,min=1,max=240"`
			Status   string `json:"status" binding:"required,oneof=draft published"`
		} `json:"chapters" binding:"required,min=1,max=100,dive"`
	}
	if err := httpx.Bind(c, &in); err != nil {
		return err
	}
	tx, err := s.pool.Begin(c)
	if err != nil {
		return err
	}
	defer tx.Rollback(c)
	// Lock courses in a consistent order to serialize numbering with other bulk imports.
	courseIDs := []string{}
	for _, ch := range in.Chapters {
		courseIDs = append(courseIDs, ch.CourseID)
	}
	rows, err := tx.Query(c, "SELECT id FROM courses WHERE id=ANY($1) ORDER BY id FOR UPDATE", courseIDs)
	if err != nil {
		return err
	}
	found := make(map[string]bool)
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			rows.Close()
			return err
		}
		found[id] = true
	}
	err = rows.Err()
	rows.Close()
	if err != nil {
		return err
	}
	for _, id := range courseIDs {
		if !found[id] {
			return httpx.Field("chapters", "One or more target courses do not exist.")
		}
	}
	for _, ch := range in.Chapters {
		if strings.TrimSpace(ch.Title) == "" {
			return httpx.BadRequest("Chapter titles cannot be empty.")
		}
		var id string
		html := content.SanitizeHTML(ch.HTML)
		err = tx.QueryRow(c, `INSERT INTO chapters(course_id,position,title,summary,minutes,html,status,updated_by) VALUES($1,(SELECT coalesce(max(position),0)+1 FROM chapters WHERE course_id=$1),$2,$3,$4,$5,$6,$7) RETURNING id`, ch.CourseID, strings.TrimSpace(ch.Title), ch.Summary, ch.Minutes, html, ch.Status, currentAdmin(c).Name).Scan(&id)
		if err != nil {
			return err
		}
		if _, err = tx.Exec(c, "INSERT INTO chapter_versions(chapter_id,version,title,html,author) VALUES($1,1,$2,$3,$4)", id, ch.Title, html, currentAdmin(c).Name); err != nil {
			return err
		}
	}
	if _, err = tx.Exec(c, "INSERT INTO audit_events(admin_id,action,resource) VALUES($1,'chapters.import',$2)", currentAdmin(c).ID, fmt.Sprint(len(in.Chapters))); err != nil {
		return err
	}
	if err = tx.Commit(c); err != nil {
		return err
	}
	return httpx.Created(c, gin.H{"created": len(in.Chapters)})
}

// Offline notes receive at most a 24-hour lease, capped by required enrollment expiry.
// offlineLease is how long a lesson may stay in the app's encrypted offline
// storage before the app must check with the server again (guide: secure
// offline access). Premium lessons never outlive the enrollment that opens
// them.
const offlineLease = 30 * 24 * time.Hour

func (s *Server) offlineExpiry(c *gin.Context, st store.Student, courseID, chapterID string) (time.Time, error) {
	until := s.now().Add(offlineLease)
	// The chapter's own access decides (premium unless made free).
	var module string
	err := s.pool.QueryRow(c, `SELECT coalesce(
			(SELECT access FROM learning_policies WHERE resource_type='chapter' AND resource_id=$1),
			'`+premiumModule+`')`, chapterID).Scan(&module)
	if err != nil || module == "free" {
		return until, err
	}
	var expires *time.Time
	if err = s.pool.QueryRow(c, "SELECT max(expires_at) FROM enrollment_requests WHERE student_id=$1 AND module=$2 AND status='approved' AND expires_at>now()", st.ID, module).Scan(&expires); err != nil {
		return until, err
	}
	if expires != nil && expires.Before(until) {
		until = *expires
	}
	return until, nil
}

func (s *Server) adminExamMode(c *gin.Context) error {
	var in struct {
		ContestID string `json:"contest_id" binding:"required"`
		Entry     string `json:"entry_password" binding:"max=72"`
		Exit      string `json:"exit_password" binding:"max=72"`
		Enabled   bool   `json:"enabled"`
	}
	if err := httpx.Bind(c, &in); err != nil {
		return err
	}
	if !in.Enabled {
		return s.auditServiceChange(c, "exam_mode.disable", in.ContestID, "UPDATE exam_mode_settings SET enabled=false WHERE contest_id=$1", in.ContestID)
	}
	if len(in.Entry) < 8 || len(in.Exit) < 8 || in.Entry == in.Exit {
		return httpx.BadRequest("Use different entry and exit passwords with at least 8 characters.")
	}
	if _, err := s.store.Contest(c, in.ContestID); err != nil {
		return storeErr(err, "Exam")
	}
	entry, err := auth.HashPassword(in.Entry)
	if err != nil {
		return err
	}
	exit, err := auth.HashPassword(in.Exit)
	if err != nil {
		return err
	}
	return s.auditServiceChange(c, "exam_mode.save", in.ContestID, "INSERT INTO exam_mode_settings(contest_id,entry_hash,exit_hash) VALUES($1,$2,$3) ON CONFLICT(contest_id) DO UPDATE SET entry_hash=$2,exit_hash=$3,enabled=true", in.ContestID, entry, exit)
}
func (s *Server) examModePassword(c *gin.Context) error {
	st, err := s.me(c)
	if err != nil {
		return err
	}
	// "check" validates an entry the same way as "enter" without starting the
	// session, so the app can confirm the password before locking the phone.
	action := c.Param("action")
	if action == "enter" || action == "check" {
		if err = s.checkLearningAccess(c, st, "contest", c.Param("id")); err != nil {
			return err
		}
	}
	var in struct {
		Password string `json:"password" binding:"required,max=72"`
	}
	if err = httpx.Bind(c, &in); err != nil {
		return err
	}
	if action != "enter" && action != "check" && action != "exit" {
		return httpx.BadRequest("Invalid exam action.")
	}
	var entry, exit string
	var enabled bool
	if err = s.pool.QueryRow(c, "SELECT entry_hash,exit_hash,enabled FROM exam_mode_settings WHERE contest_id=$1", c.Param("id")).Scan(&entry, &exit, &enabled); err != nil {
		return httpx.NotFound("Exam mode")
	}
	if action != "exit" && !enabled {
		return httpx.NotFound("Exam mode")
	}
	hash := entry
	if action == "exit" {
		hash = exit
	}
	if !auth.CheckPassword(hash, in.Password) {
		return httpx.Forbidden("Exam password is incorrect.")
	}
	if action == "exit" {
		return orNoContent(c, s.store.ServiceExec(c, "UPDATE exam_mode_sessions SET exited_at=now() WHERE contest_id=$1 AND student_id=$2 AND exited_at IS NULL", c.Param("id"), st.ID))
	}
	co, err := s.store.Contest(c, c.Param("id"))
	if err != nil {
		return err
	}
	if co.Phase != "active" {
		return httpx.Conflict("not_active", "Exam mode is available during the scheduled exam.")
	}
	// Past national exams are taken without registering.
	if co.Kind != "national" {
		var registered bool
		if err = s.pool.QueryRow(c, "SELECT EXISTS(SELECT 1 FROM registrations WHERE contest_id=$1 AND student_id=$2)", co.ID, st.ID).Scan(&registered); err != nil {
			return err
		}
		if !registered {
			return httpx.Forbidden("Register for the exam first.")
		}
	}
	if action == "check" {
		c.Status(http.StatusNoContent)
		return nil
	}
	return orNoContent(c, s.store.ServiceExec(c, "INSERT INTO exam_mode_sessions(contest_id,student_id) VALUES($1,$2) ON CONFLICT(contest_id,student_id) DO UPDATE SET entered_at=now(),exited_at=NULL", co.ID, st.ID))
}
func (s *Server) requireExamEntry(c *gin.Context, studentID, contestID string) error {
	var permitted bool
	err := s.pool.QueryRow(c, `SELECT NOT EXISTS(SELECT 1 FROM exam_mode_settings WHERE contest_id=$1 AND enabled) OR EXISTS(SELECT 1 FROM exam_mode_sessions WHERE contest_id=$1 AND student_id=$2 AND exited_at IS NULL)`, contestID, studentID).Scan(&permitted)
	if err != nil {
		return err
	}
	if !permitted {
		return httpx.Forbidden("Enter the exam password before starting.")
	}
	return nil
}

// reportDateRange treats dates as UTC calendar days and includes the whole final day.
func reportDateRange(from, through string) (*time.Time, *time.Time, error) {
	var start, end *time.Time
	if from != "" {
		d, err := time.Parse("2006-01-02", from)
		if err != nil {
			return nil, nil, httpx.BadRequest("Invalid report start date.")
		}
		start = &d
	}
	if through != "" {
		d, err := time.Parse("2006-01-02", through)
		if err != nil {
			return nil, nil, httpx.BadRequest("Invalid report end date.")
		}
		d = d.AddDate(0, 0, 1)
		end = &d
	}
	if start != nil && end != nil && !start.Before(*end) {
		return nil, nil, httpx.BadRequest("Report end date must be on or after start date.")
	}
	return start, end, nil
}

// Only the schedule is closed. Attempts and submissions retain their existing data.
func (s *Server) adminCloseMakeup(c *gin.Context) error {
	return s.auditServiceChange(c, "makeup.close", c.Param("id"), "UPDATE makeup_settings SET available_until=least(available_until,now()) WHERE contest_id=$1", c.Param("id"))
}
func (s *Server) auditServiceChange(c *gin.Context, action, resource, query string, args ...any) error {
	tx, err := s.pool.Begin(c)
	if err != nil {
		return err
	}
	defer tx.Rollback(c)
	tag, err := tx.Exec(c, query, args...)
	if err != nil {
		return storeErr(err, "Settings")
	}
	if tag.RowsAffected() == 0 {
		return httpx.NotFound("Settings")
	}
	if _, err = tx.Exec(c, "INSERT INTO audit_events(admin_id,action,resource) VALUES($1,$2,$3)", currentAdmin(c).ID, action, resource); err != nil {
		return err
	}
	return orNoContent(c, tx.Commit(c))
}

// Legacy numeric grades remain available to students stored in that grade.
func examAudienceMatches(co store.Contest, grade, department, stream string) bool {
	return (co.Grade == grade || ((co.Grade == "remedial" || co.Grade == "freshman") && co.Grade == department)) && (co.Stream == "" || co.Stream == stream)
}
