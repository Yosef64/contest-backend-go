package api

import (
	"errors"
	"fmt"
	"html/template"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/jackc/pgx/v5"

	"victor-contest-go/internal/content"
	"victor-contest-go/internal/httpx"
	"victor-contest-go/internal/store"
)

// Admin: challenges -----------------------------------------------------------

type challengeInput struct {
	Title       string `json:"title" binding:"required,max=160"`
	Description string `json:"description" binding:"max=4000"`
	Grade       string `json:"grade" binding:"required"`
	Stream      string `json:"stream" binding:"omitempty,oneof=natural social"`
	Weeks       int    `json:"weeks" binding:"required,min=1,max=104"`
	// "2026-11-01", or empty for no planned calendar.
	StartsOn            string `json:"starts_on" binding:"omitempty,datetime=2006-01-02"`
	Status              string `json:"status" binding:"required,oneof=draft published"`
	FreeDays            int    `json:"free_days" binding:"min=0,max=728"`
	ReferralEnabled     bool   `json:"referral_enabled"`
	ReferralUnit        string `json:"referral_unit" binding:"required,oneof=day week"`
	ExamReferralEnabled bool   `json:"exam_referral_enabled"`
	PremiumFromDay      *int   `json:"premium_from_day" binding:"omitempty,min=1,max=728"`
	FinalAccess         string `json:"final_access" binding:"required,oneof=free referral premium"`
	// Model and past exams offered at the end, in order.
	FinalExamIDs []string `json:"final_exam_ids" binding:"max=60"`
}

func (in *challengeInput) validate() (grade string, starts *time.Time, err error) {
	grade = NormalizeGrade(in.Grade)
	if in.Grade == "remedial" || in.Grade == "freshman" {
		grade = in.Grade
	}
	if grade == "" {
		return "", nil, httpx.Field("grade", "Choose grade 9, 10, 11 or 12, remedial or freshman.")
	}
	if (grade == "9" || grade == "10") && in.Stream != "" {
		return "", nil, httpx.Field("stream", "Grades 9 and 10 have no stream.")
	}
	if in.PremiumFromDay != nil && *in.PremiumFromDay > in.Weeks*daysPerWeek {
		return "", nil, httpx.Field("premium_from_day", "Choose a day within the challenge.")
	}
	if in.StartsOn != "" {
		t, _ := time.Parse("2006-01-02", in.StartsOn)
		starts = &t
	}
	seen := map[string]bool{}
	for _, id := range in.FinalExamIDs {
		if seen[id] {
			return "", nil, httpx.Field("final_exam_ids", "Each exam can be added once.")
		}
		seen[id] = true
	}
	return grade, starts, nil
}

type adminChallengeRow struct {
	challenge
	Members    int      `json:"members"`
	DaysReady  int      `json:"days_ready"`
	FinalExams []string `json:"final_exam_ids"`
}

func (s *Server) adminChallengeRows(c *gin.Context, where string, args ...any) ([]adminChallengeRow, error) {
	rows, err := s.pool.Query(c, `SELECT `+challengeCols+`,
			(SELECT count(*) FROM challenge_members m WHERE m.challenge_id = challenges.id)::int,
			(SELECT count(*) FROM challenge_days d WHERE d.challenge_id = challenges.id AND d.day <= challenges.weeks * 7
				AND (d.lesson_html <> '' OR d.note_html <> '' OR EXISTS (SELECT 1 FROM contest_questions cq WHERE cq.contest_id = d.quiz_id)))::int,
			coalesce((SELECT array_agg(contest_id ORDER BY position) FROM challenge_final_exams f WHERE f.challenge_id = challenges.id), '{}')
		FROM challenges `+where+` ORDER BY CASE grade WHEN 'remedial' THEN 13 WHEN 'freshman' THEN 14 ELSE grade::int END, position, created_at DESC`, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []adminChallengeRow{}
	for rows.Next() {
		var r adminChallengeRow
		ch := &r.challenge
		if err := rows.Scan(&ch.ID, &ch.Title, &ch.Description, &ch.Grade, &ch.Stream, &ch.Weeks, &ch.StartsOn, &ch.Status,
			&ch.FreeDays, &ch.ReferralEnabled, &ch.ReferralUnit, &ch.ExamReferralEnabled, &ch.PremiumFromDay,
			&ch.FinalAccess, &ch.Position, &ch.CreatedBy, &ch.CreatedAt, &ch.UpdatedAt, &r.Members, &r.DaysReady, &r.FinalExams); err != nil {
			return nil, err
		}
		out = append(out, r)
	}
	return out, rows.Err()
}

// GET /admin/challenges
func (s *Server) adminChallenges(c *gin.Context) error {
	list, err := s.adminChallengeRows(c, "")
	if err != nil {
		return err
	}
	on, err := s.referralsOn(c)
	if err != nil {
		return err
	}
	return httpx.OK(c, gin.H{"challenges": list, "referrals_on": on})
}

// GET /admin/challenges/:id — the challenge and an outline of its days.
func (s *Server) adminChallenge(c *gin.Context) error {
	list, err := s.adminChallengeRows(c, "WHERE id = $1", c.Param("id"))
	if err != nil {
		return err
	}
	if len(list) == 0 {
		return httpx.NotFound("Challenge")
	}
	days, err := s.store.ServiceRows(c, `SELECT jsonb_build_object('day', d.day, 'title', d.title, 'subject', d.subject,
			'minutes', d.minutes, 'access', d.access, 'has_lesson', d.lesson_html <> '', 'has_note', d.note_html <> '', 'quiz_id', d.quiz_id,
			'quiz_questions', (SELECT count(*) FROM contest_questions cq WHERE cq.contest_id = d.quiz_id),
			'completed', (SELECT count(*) FROM challenge_steps x WHERE x.challenge_id = d.challenge_id AND x.day = d.day AND x.step = 'lesson'),
			'quiz_takers', (SELECT count(*) FROM submissions sub WHERE sub.contest_id = d.quiz_id),
			'updated_at', d.updated_at, 'updated_by', d.updated_by)
		FROM challenge_days d WHERE d.challenge_id = $1 ORDER BY d.day`, c.Param("id"))
	if err != nil {
		return err
	}
	return httpx.OK(c, gin.H{"challenge": list[0], "days": days})
}

// saveFinals replaces the challenge's final exams.
func saveFinals(c *gin.Context, tx pgx.Tx, id string, ids []string) error {
	if _, err := tx.Exec(c, `DELETE FROM challenge_final_exams WHERE challenge_id = $1`, id); err != nil {
		return err
	}
	for i, exam := range ids {
		tag, err := tx.Exec(c, `INSERT INTO challenge_final_exams (challenge_id, contest_id, position)
			SELECT $1, id, $3 FROM contests WHERE id = $2 AND kind IN ('model', 'national')`, id, exam, i)
		if err != nil {
			return err
		}
		if tag.RowsAffected() == 0 {
			return httpx.Field("final_exam_ids", "Only model and past national exams can be added.")
		}
	}
	return nil
}

// POST /admin/challenges
func (s *Server) adminCreateChallenge(c *gin.Context) error {
	var in challengeInput
	if err := httpx.Bind(c, &in); err != nil {
		return err
	}
	grade, starts, err := in.validate()
	if err != nil {
		return err
	}
	tx, err := s.pool.Begin(c)
	if err != nil {
		return err
	}
	defer tx.Rollback(c)
	var id string
	if err := tx.QueryRow(c, `INSERT INTO challenges (title, description, grade, stream, weeks, starts_on, status, free_days,
			referral_enabled, referral_unit, exam_referral_enabled, premium_from_day, final_access, created_by, position)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14, coalesce((SELECT max(position) + 1 FROM challenges), 0))
		RETURNING id`, strings.TrimSpace(in.Title), strings.TrimSpace(in.Description), grade, in.Stream, in.Weeks, starts, in.Status,
		in.FreeDays, in.ReferralEnabled, in.ReferralUnit, in.ExamReferralEnabled, in.PremiumFromDay, in.FinalAccess,
		currentAdmin(c).Name).Scan(&id); err != nil {
		return err
	}
	if err := saveFinals(c, tx, id, in.FinalExamIDs); err != nil {
		return err
	}
	if err := tx.Commit(c); err != nil {
		return err
	}
	c.Params = append(c.Params, gin.Param{Key: "id", Value: id})
	return s.adminChallenge(c)
}

// PUT /admin/challenges/:id
func (s *Server) adminUpdateChallenge(c *gin.Context) error {
	var in challengeInput
	if err := httpx.Bind(c, &in); err != nil {
		return err
	}
	grade, starts, err := in.validate()
	if err != nil {
		return err
	}
	tx, err := s.pool.Begin(c)
	if err != nil {
		return err
	}
	defer tx.Rollback(c)
	tag, err := tx.Exec(c, `UPDATE challenges SET title = $2, description = $3, grade = $4, stream = $5, weeks = $6, starts_on = $7,
			status = $8, free_days = $9, referral_enabled = $10, referral_unit = $11, exam_referral_enabled = $12,
			premium_from_day = $13, final_access = $14, updated_at = now()
		WHERE id = $1`, c.Param("id"), strings.TrimSpace(in.Title), strings.TrimSpace(in.Description), grade, in.Stream, in.Weeks,
		starts, in.Status, in.FreeDays, in.ReferralEnabled, in.ReferralUnit, in.ExamReferralEnabled, in.PremiumFromDay, in.FinalAccess)
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		return httpx.NotFound("Challenge")
	}
	if err := saveFinals(c, tx, c.Param("id"), in.FinalExamIDs); err != nil {
		return err
	}
	if _, err := tx.Exec(c, `INSERT INTO audit_events (admin_id, action, resource) VALUES ($1, 'challenge_saved', $2)`,
		currentAdmin(c).ID, c.Param("id")); err != nil {
		return err
	}
	if err := tx.Commit(c); err != nil {
		return err
	}
	return s.adminChallenge(c)
}

// DELETE /admin/challenges/:id — also deletes its quizzes and weekly exams.
func (s *Server) adminDeleteChallenge(c *gin.Context) error {
	tx, err := s.pool.Begin(c)
	if err != nil {
		return err
	}
	defer tx.Rollback(c)
	if _, err := tx.Exec(c, `DELETE FROM contests WHERE id IN (SELECT quiz_id FROM challenge_days WHERE challenge_id = $1)`, c.Param("id")); err != nil {
		return err
	}
	tag, err := tx.Exec(c, `DELETE FROM challenges WHERE id = $1`, c.Param("id"))
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		return httpx.NotFound("Challenge")
	}
	return orNoContent(c, tx.Commit(c))
}

func (s *Server) adminDayParam(c *gin.Context) (challenge, int, error) {
	ch, err := s.loadChallenge(c, c.Param("id"))
	if err != nil {
		return ch, 0, err
	}
	day, _ := strconv.Atoi(c.Param("day"))
	if day < 1 || day > ch.days() {
		return ch, 0, httpx.NotFound("Day")
	}
	return ch, day, nil
}

// GET /admin/challenges/:id/days/:day — everything an editor needs.
func (s *Server) adminChallengeDay(c *gin.Context) error {
	ch, day, err := s.adminDayParam(c)
	if err != nil {
		return err
	}
	out := gin.H{"day": day, "week": weekOf(day), "month": monthOf(day), "is_exam": isExamDay(day),
		"title": "", "subject": "", "minutes": 30, "access": "", "lesson_html": "", "note_html": "", "quiz": nil,
		"challenge": gin.H{"id": ch.ID, "title": ch.Title, "grade": ch.Grade, "stream": ch.Stream, "days": ch.days()}}
	var title, subject, access, lesson, note, updatedBy string
	var minutes int
	var quizID *string
	var updated time.Time
	err = s.pool.QueryRow(c, `SELECT title, subject, minutes, access, lesson_html, note_html, quiz_id, updated_at, updated_by
		FROM challenge_days WHERE challenge_id = $1 AND day = $2`, ch.ID, day).
		Scan(&title, &subject, &minutes, &access, &lesson, &note, &quizID, &updated, &updatedBy)
	switch {
	case errors.Is(err, pgx.ErrNoRows):
	case err != nil:
		return err
	default:
		out["title"], out["subject"], out["minutes"], out["access"] = title, subject, minutes, access
		out["lesson_html"], out["note_html"] = lesson, note
		out["updated_at"], out["updated_by"] = updated, updatedBy
		if quizID != nil {
			quiz, err := s.store.Contest(c, *quizID)
			if err != nil {
				return err
			}
			qs, err := s.store.ContestQuestions(c, quiz.ID)
			if err != nil {
				return err
			}
			out["quiz"] = gin.H{"contest": quiz, "questions": qs}
		}
	}
	return httpx.OK(c, out)
}

// PUT /admin/challenges/:id/days/:day — title, subject, lesson and note.
func (s *Server) adminSaveChallengeDay(c *gin.Context) error {
	var in struct {
		Title   string `json:"title" binding:"max=160"`
		Subject string `json:"subject" binding:"max=80"`
		Minutes int    `json:"minutes" binding:"min=1,max=600"`
		// '' follows the challenge's rules.
		Access     string `json:"access" binding:"omitempty,oneof=free referral premium"`
		LessonHTML string `json:"lesson_html" binding:"max=2000000"`
		NoteHTML   string `json:"note_html" binding:"max=2000000"`
	}
	if err := httpx.Bind(c, &in); err != nil {
		return err
	}
	ch, day, err := s.adminDayParam(c)
	if err != nil {
		return err
	}
	if in.Subject != "" && !subjectOK(in.Subject, true) {
		return httpx.Field("subject", "Choose a supported subject.")
	}
	lesson, note := content.SanitizeHTML(in.LessonHTML), content.SanitizeHTML(in.NoteHTML)
	if strings.TrimSpace(stripTags(lesson)) == "" && !strings.Contains(lesson, "<img") && !strings.Contains(lesson, "<svg") && !strings.Contains(lesson, "<video") && !strings.Contains(lesson, "<iframe") {
		lesson = ""
	}
	if strings.TrimSpace(stripTags(note)) == "" && !strings.Contains(note, "<img") && !strings.Contains(note, "<svg") {
		note = ""
	}
	if _, err := s.pool.Exec(c, `INSERT INTO challenge_days (challenge_id, day, title, subject, minutes, lesson_html, note_html, updated_by, access)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9)
		ON CONFLICT (challenge_id, day) DO UPDATE SET title = $3, subject = $4, minutes = $5, lesson_html = $6, note_html = $7,
			updated_by = $8, access = $9, updated_at = now()`, ch.ID, day, strings.TrimSpace(in.Title), in.Subject, in.Minutes, lesson, note,
		currentAdmin(c).Name, in.Access); err != nil {
		return err
	}
	return s.adminChallengeDay(c)
}

// stripTags is a rough text view of HTML, to tell an empty editor apart.
func stripTags(html string) string {
	var b strings.Builder
	in := false
	for _, r := range html {
		switch {
		case r == '<':
			in = true
		case r == '>':
			in = false
		case !in:
			b.WriteRune(r)
		}
	}
	return strings.ReplaceAll(b.String(), "&nbsp;", "")
}

// PUT /admin/challenges/:id/days/:day/quiz — creates the day's quiz (or the
// week's exam) on first use; sets its questions and time.
func (s *Server) adminSaveChallengeQuiz(c *gin.Context) error {
	var in struct {
		QuestionIDs     []string `json:"question_ids" binding:"max=200"`
		DurationMinutes int      `json:"duration_minutes" binding:"required,min=1,max=600"`
		Subject         string   `json:"subject" binding:"max=80"`
	}
	if err := httpx.Bind(c, &in); err != nil {
		return err
	}
	ch, day, err := s.adminDayParam(c)
	if err != nil {
		return err
	}
	seen := map[string]bool{}
	for _, id := range in.QuestionIDs {
		if seen[id] {
			return httpx.Field("question_ids", "Each question can be used once.")
		}
		seen[id] = true
	}
	// A quiz may cover several subjects.
	subject := in.Subject
	if subject == "" || !subjectOK(subject, true) {
		subject = "Mixed"
	}
	title := fmt.Sprintf("%s · Day %d quiz", ch.Title, day)
	if isExamDay(day) {
		title = fmt.Sprintf("%s · Week %d exam", ch.Title, weekOf(day))
	}
	if _, err := s.pool.Exec(c, `INSERT INTO challenge_days (challenge_id, day, updated_by) VALUES ($1, $2, $3)
		ON CONFLICT DO NOTHING`, ch.ID, day, currentAdmin(c).Name); err != nil {
		return err
	}
	var quizID *string
	if err := s.pool.QueryRow(c, `SELECT quiz_id FROM challenge_days WHERE challenge_id = $1 AND day = $2`, ch.ID, day).Scan(&quizID); err != nil {
		return err
	}
	duration := in.DurationMinutes
	quiz := store.Contest{Title: title, Subject: subject, Grade: ch.Grade, Stream: ch.Stream, Kind: "challenge",
		StartTime: time.Date(2020, 1, 1, 0, 0, 0, 0, time.UTC), EndTime: time.Date(2100, 1, 1, 0, 0, 0, 0, time.UTC),
		QuestionIDs: in.QuestionIDs, DurationMinutes: &duration, ShuffleQuestions: true, ShuffleOptions: true,
		Rules:     "Answer every question. Your score and the correct answers appear as soon as you submit.",
		CreatedBy: currentAdmin(c).Name}
	if quizID == nil {
		out, err := s.store.CreateContest(c, quiz)
		if errors.Is(err, store.ErrInUse) {
			return httpx.Field("question_ids", "One of the selected questions no longer exists.")
		}
		if err != nil {
			return err
		}
		if _, err := s.pool.Exec(c, `UPDATE challenge_days SET quiz_id = $3, updated_at = now(), updated_by = $4
			WHERE challenge_id = $1 AND day = $2`, ch.ID, day, out.ID, currentAdmin(c).Name); err != nil {
			return err
		}
	} else {
		current, err := s.store.Contest(c, *quizID)
		if err != nil {
			return err
		}
		if current.Submissions+current.Attempts > 0 && !sameIDs(current.QuestionIDs, in.QuestionIDs) {
			return httpx.Conflict("questions_locked", "Students have taken this quiz, so its questions are locked.")
		}
		if _, err := s.store.UpdateContest(c, *quizID, quiz); err != nil {
			if errors.Is(err, store.ErrInUse) {
				return httpx.Field("question_ids", "One of the selected questions no longer exists.")
			}
			return err
		}
	}
	return s.adminChallengeDay(c)
}

func sameIDs(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

// DELETE /admin/challenges/:id/days/:day/quiz
func (s *Server) adminDeleteChallengeQuiz(c *gin.Context) error {
	ch, day, err := s.adminDayParam(c)
	if err != nil {
		return err
	}
	var takers int
	if err := s.pool.QueryRow(c, `SELECT count(*) FROM submissions WHERE contest_id =
		(SELECT quiz_id FROM challenge_days WHERE challenge_id = $1 AND day = $2)`, ch.ID, day).Scan(&takers); err != nil {
		return err
	}
	if takers > 0 {
		return httpx.Conflict("questions_locked", "Students have taken this quiz, so it can't be removed.")
	}
	_, err = s.pool.Exec(c, `DELETE FROM contests WHERE id = (SELECT quiz_id FROM challenge_days WHERE challenge_id = $1 AND day = $2)`, ch.ID, day)
	return orNoContent(c, err)
}

// GET /admin/challenges/:id/report — how students move through it.
func (s *Server) adminChallengeReport(c *gin.Context) error {
	ch, err := s.loadChallenge(c, c.Param("id"))
	if err != nil {
		return err
	}
	out := gin.H{"challenge": ch}
	var members, paid, active7, completed, referrals, referralUnlocks int
	if err := s.pool.QueryRow(c, `WITH m AS (SELECT student_id FROM challenge_members WHERE challenge_id = $1)
		SELECT (SELECT count(*) FROM m)::int,
			(SELECT count(*) FROM m WHERE `+store.PaidNow("m.student_id")+`)::int,
			(SELECT count(DISTINCT student_id) FROM challenge_steps WHERE challenge_id = $1 AND done_at > now() - interval '7 days')::int,
			(SELECT count(*) FROM challenge_referrals WHERE challenge_id = $1)::int,
			(SELECT count(*) FROM (SELECT 1 FROM challenge_referrals WHERE challenge_id = $1 GROUP BY owner_id, kind, unit HAVING count(*) >= $2) u)::int`,
		ch.ID, referralsNeeded).Scan(&members, &paid, &active7, &referrals, &referralUnlocks); err != nil {
		return err
	}

	// Per day: students who finished each step, and quiz results.
	days, err := s.store.ServiceRows(c, `SELECT jsonb_build_object('day', d.day, 'title', d.title, 'is_exam', d.day % 7 = 0,
			'lesson', (SELECT count(*) FROM challenge_steps x WHERE x.challenge_id = d.challenge_id AND x.day = d.day AND x.step = 'lesson'),
			'note', (SELECT count(*) FROM challenge_steps x WHERE x.challenge_id = d.challenge_id AND x.day = d.day AND x.step = 'note'),
			'quiz', (SELECT count(*) FROM submissions sub WHERE sub.contest_id = d.quiz_id),
			'average', (SELECT round(avg(CASE WHEN sub.total = 0 THEN 0 ELSE sub.correct * 100.0 / sub.total END)) FROM submissions sub WHERE sub.contest_id = d.quiz_id),
			'has_lesson', d.lesson_html <> '', 'has_note', d.note_html <> '', 'has_quiz', d.quiz_id IS NOT NULL)
		FROM challenge_days d WHERE d.challenge_id = $1 AND d.day <= $2 ORDER BY d.day`, ch.ID, ch.days())
	if err != nil {
		return err
	}

	// Where each member is: the first day they haven't finished.
	students, err := s.store.ServiceRows(c, `WITH m AS (
			SELECT m.student_id, m.joined_at, st.name, st.photo_url, `+store.PaidNow("m.student_id")+` AS paid,
				coalesce(p.stream, '') AS stream
			FROM challenge_members m JOIN students st ON st.id = m.student_id
			LEFT JOIN academic_profiles p ON p.student_id = m.student_id
			WHERE m.challenge_id = $1),
		finished AS (
			SELECT m.student_id, d.day
			FROM m CROSS JOIN challenge_days d
			WHERE d.challenge_id = $1 AND d.day <= $2
			  AND (d.lesson_html <> '' OR d.note_html <> '' OR d.quiz_id IS NOT NULL)
			  AND (d.lesson_html = '' OR EXISTS (SELECT 1 FROM challenge_steps x WHERE x.challenge_id = $1 AND x.day = d.day AND x.student_id = m.student_id AND x.step = 'lesson'))
			  AND (d.note_html = '' OR EXISTS (SELECT 1 FROM challenge_steps x WHERE x.challenge_id = $1 AND x.day = d.day AND x.student_id = m.student_id AND x.step = 'note'))
			  AND (d.quiz_id IS NULL OR NOT EXISTS (SELECT 1 FROM contest_questions cq WHERE cq.contest_id = d.quiz_id)
				OR EXISTS (SELECT 1 FROM submissions sub WHERE sub.contest_id = d.quiz_id AND sub.student_id = m.student_id))),
		reached AS (
			SELECT m.student_id, coalesce((SELECT min(g) FROM generate_series(1, $2) g
				WHERE NOT EXISTS (SELECT 1 FROM finished f WHERE f.student_id = m.student_id AND f.day = g)), $2 + 1) AS day
			FROM m)
		SELECT jsonb_build_object('student_id', m.student_id, 'name', m.name, 'photo_url', m.photo_url, 'paid', m.paid,
			'stream', m.stream, 'joined_at', m.joined_at, 'days_done', r.day - 1,
			'last_active', (SELECT max(done_at) FROM challenge_steps x WHERE x.challenge_id = $1 AND x.student_id = m.student_id),
			'average', (SELECT round(avg(CASE WHEN sub.total = 0 THEN 0 ELSE sub.correct * 100.0 / sub.total END))
				FROM challenge_days d JOIN submissions sub ON sub.contest_id = d.quiz_id AND sub.student_id = m.student_id
				WHERE d.challenge_id = $1),
			'invited', (SELECT count(*) FROM challenge_referrals cr WHERE cr.challenge_id = $1 AND cr.owner_id = m.student_id))
		FROM m JOIN reached r ON r.student_id = m.student_id
		ORDER BY r.day DESC, m.name LIMIT 500`, ch.ID, ch.days())
	if err != nil {
		return err
	}
	for _, r := range students {
		if n, ok := r["days_done"].(float64); ok && int(n) >= ch.days() {
			completed++
		}
	}
	out["totals"] = gin.H{"members": members, "paid": paid, "free": members - paid, "active_7d": active7,
		"completed": completed, "referrals": referrals, "referral_unlocks": referralUnlocks, "days": ch.days()}
	out["days"] = days
	out["students"] = students
	return httpx.OK(c, out)
}

// Shared links ----------------------------------------------------------------

var challengeInvitePage = template.Must(template.New("c").Parse(`<!doctype html>
<html lang="en"><head><meta charset="utf-8"><meta name="viewport" content="width=device-width,initial-scale=1">
<title>Join {{.Inviter}} on Victory</title>
<style>
:root{--g:#13814f;--ink:#121715;--muted:#71777f;--canvas:#eceeef}
*{box-sizing:border-box}body{margin:0;background:var(--canvas);color:var(--ink);font:16px/1.5 Urbanist,system-ui,-apple-system,Segoe UI,Roboto,sans-serif;display:grid;min-height:100vh;place-items:center;padding:16px}
main{background:#fff;border-radius:28px;padding:28px;max-width:420px;width:100%}
.tag{display:inline-block;background:#e7f4ed;color:var(--g);border-radius:999px;padding:4px 12px;font-size:13px;font-weight:600;margin-bottom:14px}
h1{font-size:28px;line-height:1.2;letter-spacing:-.7px;margin:0 0 8px;font-weight:600}
p{color:var(--muted);margin:0 0 20px}
.btn{display:block;text-align:center;text-decoration:none;border-radius:999px;padding:14px 18px;font-weight:600;margin-top:10px}
.primary{background:var(--g);color:#fff}.ghost{border:1px solid #e6e8ea;color:var(--ink)}
small{display:block;color:var(--muted);margin-top:18px;font-size:13px}
</style></head><body><main>
{{if .Challenge}}<span class="tag">{{.Challenge}}</span>{{end}}
<h1>{{.Inviter}} invited you to a study challenge</h1>
<p>Daily lessons, short notes and quizzes, with a weekly exam and a leaderboard. Join Victory and study along.</p>
{{if .AppLink}}<a class="btn primary" href="{{.AppLink}}">Open in the Victory app</a>{{end}}
{{range .Links}}<a class="btn ghost" href="{{.URL}}" rel="noopener">{{.Label}}</a>{{end}}
<small>Create your free account in the app. Your friend's invite counts once you have signed up.</small>
</main></body></html>`))

// GET /c/:owner/:challenge/:kind/:unit — the page behind a challenge invite.
func (s *Server) challengeInviteLanding(c *gin.Context) {
	owner, id, kind := c.Param("owner"), c.Param("challenge"), c.Param("kind")
	unit, _ := strconv.Atoi(c.Param("unit"))
	data := struct {
		Inviter, Challenge string
		AppLink            template.URL
		Links              []struct{ Label, URL string }
	}{Inviter: "A friend", AppLink: "victory://open"}
	var name, title string
	if err := s.pool.QueryRow(c, `SELECT st.name, ch.title FROM students st, challenges ch
		WHERE st.id = $1 AND ch.id = $2 AND ch.status = 'published'`, owner, id).Scan(&name, &title); err == nil {
		data.Inviter = strings.Fields(name + " ")[0]
		data.Challenge = title
		if kind == "lesson" || kind == "exam" || kind == "final" || kind == "day" {
			q := url.Values{"owner": {owner}, "challenge": {id}, "kind": {kind}, "unit": {strconv.Itoa(unit)}}
			data.AppLink = template.URL("victory://challenge-invite?" + q.Encode())
		}
	}
	if settings, err := s.siteSettings(c); err == nil {
		for _, key := range []string{"telegram_channel", "telegram_bot"} {
			if v := settings[key]; strings.HasPrefix(v, "https://") {
				label := map[string]string{"telegram_channel": "Join our Telegram channel", "telegram_bot": "Open the Victory Telegram bot"}[key]
				data.Links = append(data.Links, struct{ Label, URL string }{label, v})
			}
		}
	}
	c.Header("Content-Type", "text/html; charset=utf-8")
	c.Header("Cache-Control", "no-store")
	c.Header("Referrer-Policy", "no-referrer")
	c.Status(http.StatusOK)
	_ = challengeInvitePage.Execute(c.Writer, data)
}

// PUT /admin/challenge-settings — the switch for every challenge's invites.
func (s *Server) adminChallengeSettings(c *gin.Context) error {
	var in struct {
		ReferralsOn bool `json:"referrals_on"`
	}
	if err := httpx.Bind(c, &in); err != nil {
		return err
	}
	value := "off"
	if in.ReferralsOn {
		value = "on"
	}
	if _, err := s.pool.Exec(c, `WITH saved AS (
			INSERT INTO site_settings (key, value) VALUES ('challenge_referrals', $1)
			ON CONFLICT (key) DO UPDATE SET value = excluded.value, updated_at = now())
		INSERT INTO audit_events (admin_id, action, resource) VALUES ($2, 'challenge_referrals_' || $1, 'challenges')`,
		value, currentAdmin(c).ID); err != nil {
		return err
	}
	return httpx.OK(c, gin.H{"referrals_on": in.ReferralsOn})
}
