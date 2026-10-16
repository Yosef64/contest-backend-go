//go:build integration

// Runs the migrations and the new SQL against a real, throwaway Postgres:
//
//	docker run -d --name victory-test-pg -e POSTGRES_PASSWORD=test -e POSTGRES_DB=victory -p 55432:5432 postgres:16-alpine
//	TEST_DATABASE_URL=postgres://postgres:test@localhost:55432/victory?sslmode=disable go test -tags integration ./internal/api -run Integration -v
//
// Never point it at a real database: it creates and changes data.
package api

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/gin-gonic/gin"

	"victor-contest-go/internal/config"
	"victor-contest-go/internal/db"
	"victor-contest-go/internal/store"
)

func integrationServer(t *testing.T) *Server {
	t.Helper()
	url := os.Getenv("TEST_DATABASE_URL")
	if url == "" {
		t.Skip("TEST_DATABASE_URL not set")
	}
	if !strings.Contains(url, "localhost") && !strings.Contains(url, "127.0.0.1") {
		t.Fatal("TEST_DATABASE_URL must be a local throwaway database")
	}
	ctx := context.Background()
	pool, err := db.Open(ctx, url)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(pool.Close)
	if _, err := db.Migrate(ctx, pool); err != nil {
		t.Fatal("migrate:", err)
	}
	return NewServer(config.Config{JWTSecret: []byte("test-secret-test-secret-test-secret")}, pool, nil)
}

func exec(t *testing.T, s *Server, sql string, args ...any) {
	t.Helper()
	if _, err := s.pool.Exec(context.Background(), sql, args...); err != nil {
		t.Fatalf("%s: %v", sql, err)
	}
}

// call runs a handler as an admin (or a student) with the query string.
func call(t *testing.T, h func(*gin.Context) error, method, target string, body string, set map[string]any, params gin.Params) map[string]any {
	t.Helper()
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = httptest.NewRequest(method, target, strings.NewReader(body))
	c.Request.Header.Set("Content-Type", "application/json")
	c.Params = params
	for k, v := range set {
		c.Set(k, v)
	}
	if err := h(c); err != nil {
		t.Fatalf("%s %s: %v", method, target, err)
	}
	out := map[string]any{}
	if w.Body.Len() > 0 {
		if err := json.Unmarshal(w.Body.Bytes(), &out); err != nil {
			t.Fatalf("%s %s: %v: %s", method, target, err, w.Body.String())
		}
	}
	if w.Code >= 300 {
		t.Fatalf("%s %s: %d %s", method, target, w.Code, w.Body.String())
	}
	return out
}

func TestIntegrationNewFeatures(t *testing.T) {
	s := integrationServer(t)
	ctx := context.Background()
	now := time.Now()

	// Subjects: seeded, and a new one for freshman.
	if !s.isSubject(ctx, "Mathematics", false) || s.isSubject(ctx, "Mixed", false) || !s.isSubject(ctx, "Mixed", true) {
		t.Fatal("seeded subjects")
	}
	exec(t, s, `DELETE FROM subjects WHERE name IN ('Logic', 'Formal Logic')`)
	exec(t, s, `INSERT INTO subjects (name, icon, color, levels) VALUES ('Logic', 'brain', '#4F46E5', '{freshman}')`)
	forgetSubjects()
	if !s.isSubject(ctx, "Logic", false) {
		t.Fatal("new subject")
	}
	list := call(t, s.publicSubjects, http.MethodGet, "/subjects", "", nil, nil)
	if n := len(list["subjects"].([]any)); n < 10 {
		t.Fatalf("subjects: %d", n)
	}

	// Data: a paid and a free student, one exam, two questions.
	exec(t, s, `DELETE FROM students WHERE id IN ('it_paid', 'it_free')`)
	exec(t, s, `DELETE FROM contests WHERE id = 'it_exam'`)
	exec(t, s, `DELETE FROM questions WHERE id IN ('it_q1', 'it_q2')`)
	for _, id := range []string{"it_paid", "it_free"} {
		exec(t, s, `INSERT INTO students (id, auth_provider, auth_subject, name, grade, gender, region, city, school)
			VALUES ($1, 'dev', $1, $1, '12', 'female', 'Oromia', 'Adama', 'Adama Science')`, id)
		exec(t, s, `INSERT INTO academic_profiles (student_id) VALUES ($1) ON CONFLICT DO NOTHING`, id)
	}
	exec(t, s, `INSERT INTO enrollment_requests (student_id, module, reference, status, expires_at, created_at)
		VALUES ('it_paid', 'academic', 'ref', 'approved', now() + interval '30 days', now() - interval '1 day')`)
	exec(t, s, `INSERT INTO questions (id, text, options, answer_index, subject, chapter, grade, option_images, explanation_image_url)
		VALUES ('it_q1', 'Q1', '{A,B,C,D}', 2, 'Logic', 'Arguments', '12', '{"","","https://x/c.png",""}', 'https://x/e.png'),
		       ('it_q2', 'Q2', '{A,B}', 0, 'Logic', 'Arguments', '12', '{}', '')`)
	exec(t, s, `INSERT INTO contests (id, title, subject, grade, start_time, end_time, shuffle_questions, shuffle_options)
		VALUES ('it_exam', 'Logic test', 'Logic', '12', $1, $2, true, true)`, now.Add(-2*time.Hour), now.Add(-time.Hour))
	exec(t, s, `INSERT INTO contest_questions (contest_id, question_id, position) VALUES ('it_exam', 'it_q1', 1), ('it_exam', 'it_q2', 2)`)

	// Shuffle: the questions load with pictures and settings.
	co, err := s.store.Contest(ctx, "it_exam")
	if err != nil || !co.ShuffleQuestions || !co.ShuffleOptions {
		t.Fatalf("contest shuffle settings: %v %+v", err, co)
	}
	qs, err := s.store.ContestQuestions(ctx, "it_exam")
	if err != nil || len(qs) != 2 || qs[0].OptionImages[2] != "https://x/c.png" || qs[0].ExplanationImageURL == "" {
		t.Fatalf("questions: %v %+v", err, qs)
	}
	// Scored through the shuffle: the paid student right, the free one wrong.
	for id, right := range map[string]bool{"it_paid": true, "it_free": false} {
		given := map[string]int{}
		for _, p := range playQuestions(co, id, qs) {
			want := "C"
			if p.ID == "it_q2" {
				want = "A"
			}
			for i, o := range p.Options {
				if (o == want) == right {
					given[p.ID] = i
					break
				}
			}
		}
		answers, correct := scoreAnswers(qs, unshuffleAnswers(co, id, qs, given))
		js, _ := json.Marshal(answers)
		exec(t, s, `INSERT INTO submissions (contest_id, student_id, answers, correct, total, submitted_at) VALUES ('it_exam', $1, $2, $3, 2, now() - interval '90 minutes')`, id, js, correct)
		if right && correct != 2 || !right && correct != 0 {
			t.Fatalf("%s correct = %d", id, correct)
		}
	}

	// Reports: paid only by default, free apart, and the plan breakdown.
	report := func(q string) []any {
		out := call(t, s.adminReports, http.MethodGet, "/x?"+q, "", nil, nil)
		return out["rows"].([]any)
	}
	acc := func(rows []any, group string) float64 {
		for _, r := range rows {
			m := r.(map[string]any)
			if m["group"] == group {
				return m["accuracy"].(float64)
			}
		}
		t.Fatalf("no %q in %v", group, rows)
		return 0
	}
	if a := acc(report("group=overall&subject=Logic"), "All students"); a != 100 {
		t.Fatalf("paid accuracy = %v", a)
	}
	if a := acc(report("group=overall&subject=Logic&plan=free"), "All students"); a != 0 {
		t.Fatalf("free accuracy = %v", a)
	}
	plans := report("group=plan&subject=Logic&plan=all")
	if acc(plans, "Paid") != 100 || acc(plans, "Free") != 0 {
		t.Fatalf("plan breakdown: %v", plans)
	}
	if a := acc(report("group=chapter&subject=Logic"), "Logic / Arguments"); a != 100 {
		t.Fatalf("chapter accuracy = %v", a)
	}
	for _, g := range reportDimensions {
		report("group=" + g + "&plan=all")
	}
	master := call(t, s.adminMasterReport, http.MethodGet, "/x?plan=all", "", nil, nil)
	if len(master["sections"].(map[string]any)) != len(reportDimensions) {
		t.Fatalf("master sections: %v", master["sections"])
	}

	// Student insights: the paid student is strong in Logic / Arguments.
	ins := call(t, s.adminStudentInsights, http.MethodGet, "/x", "", nil, gin.Params{{Key: "id", Value: "it_paid"}})["insights"].(map[string]any)
	if ins["exams"].(float64) != 1 || ins["accuracy"].(float64) != 100 || len(ins["recommendations"].([]any)) == 0 {
		t.Fatalf("insights: %v", ins)
	}
	weak := call(t, s.adminStudentInsights, http.MethodGet, "/x", "", nil, gin.Params{{Key: "id", Value: "it_free"}})["insights"].(map[string]any)
	if weak["accuracy"].(float64) != 0 || len(weak["insights"].([]any)) == 0 {
		t.Fatalf("weak insights: %v", weak)
	}

	// Exam-mode breaks: a limit, two reports, the proctor's view.
	exec(t, s, `INSERT INTO exam_mode_settings (contest_id, entry_hash, exit_hash, max_violations) VALUES ('it_exam', 'x', 'y', 2)
		ON CONFLICT (contest_id) DO UPDATE SET max_violations = 2, enabled = true`)
	exec(t, s, `INSERT INTO registrations (contest_id, student_id) VALUES ('it_exam', 'it_free') ON CONFLICT DO NOTHING`)
	exec(t, s, `DELETE FROM exam_violations WHERE contest_id = 'it_exam'`)
	exec(t, s, `INSERT INTO exam_violations (contest_id, student_id, kind, at) VALUES ('it_exam', 'it_free', 'unpinned', now()), ('it_exam', 'it_free', 'left_app', now())`)
	v := call(t, s.adminExamViolations, http.MethodGet, "/x", "", nil, gin.Params{{Key: "id", Value: "it_exam"}})
	rows := v["violations"].([]any)
	if len(rows) != 1 || rows[0].(map[string]any)["unpinned"].(float64) != 1 || v["max_violations"].(float64) != 2 {
		t.Fatalf("violations: %v", v)
	}

	// Partner school content: a teacher's question and exam belong to the
	// school, other schools (and the teacher's list) don't see others'.
	exec(t, s, `DELETE FROM contests WHERE title = 'School quiz'`)
	exec(t, s, `DELETE FROM questions WHERE text = 'School Q'`)
	exec(t, s, `INSERT INTO schools (id, name) VALUES ('it_school', 'Adama Science') ON CONFLICT (id) DO NOTHING`)
	exec(t, s, `UPDATE academic_profiles SET school_id = 'it_school' WHERE student_id = 'it_free'`)
	school := "it_school"
	teacher := map[string]any{"permission_role": "teacher", "manager_school": &school, "admin": store.Admin{Name: "Teacher T"}}
	created := call(t, s.adminCreateQuestion, http.MethodPost, "/x",
		`{"text":"School Q","options":["A","B"],"answer_index":0,"subject":"Logic","chapter":"Sets","grade":"12"}`, teacher, nil)
	qid := created["question"].(map[string]any)["id"].(string)
	if created["question"].(map[string]any)["school_id"] != "it_school" {
		t.Fatalf("teacher's question school: %v", created)
	}
	mine := call(t, s.adminQuestions, http.MethodGet, "/x", "", teacher, nil)["questions"].([]any)
	if len(mine) != 1 {
		t.Fatalf("teacher sees %d questions, want only their school's", len(mine))
	}
	start, end := now.Add(-time.Minute).UTC().Format(time.RFC3339), now.Add(time.Hour).UTC().Format(time.RFC3339)
	exam := call(t, s.adminCreateContest, http.MethodPost, "/x",
		`{"title":"School quiz","subject":"Logic","grade":"12","start_time":"`+start+`","end_time":"`+end+`","question_ids":["`+qid+`"]}`, teacher, nil)
	examID := exam["contest"].(map[string]any)["id"].(string)
	if exam["contest"].(map[string]any)["school_id"] != "it_school" {
		t.Fatalf("school exam: %v", exam)
	}
	free, _ := s.store.StudentByID(ctx, "it_free")
	paid, _ := s.store.StudentByID(ctx, "it_paid")
	w := httptest.NewRecorder()
	gc, _ := gin.CreateTestContext(w)
	gc.Request = httptest.NewRequest(http.MethodGet, "/", nil)
	if err := s.checkLearningAccess(gc, free, "contest", examID); err != nil {
		t.Fatalf("the school's student: %v", err)
	}
	if err := s.checkLearningAccess(gc, paid, "contest", examID); err == nil {
		t.Fatal("a student of another school reached the school's exam")
	}

	// Uploader role is allowed by the constraint.
	exec(t, s, `UPDATE admins SET permission_role = 'uploader' WHERE false`)
	exec(t, s, `SELECT 1 WHERE 'uploader' IN ('editor', 'analyst', 'manager', 'uploader')`)
}
