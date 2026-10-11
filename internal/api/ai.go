package api

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"sort"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/jackc/pgx/v5"

	"victor-contest-go/internal/ai"
	"victor-contest-go/internal/content"
	"victor-contest-go/internal/httpx"
	"victor-contest-go/internal/store"
)

// AI features -------------------------------------------------------------------
//
// Following the guide: an AI tutor that explains in Victory Academy's own
// friendly, analogy-rich style (in context of the chapter being read), a
// personal performance analysis with suggestions, a weekly review quiz built
// from weak areas and recurring mistakes, and AI summaries of institutional
// reports for admins and school managers. The API key stays on the server.

const tutorSystem = `You are Victory Academy's AI tutor for Ethiopian high school (grades 9–12), Grade 12 national exam, remedial and freshman students.

How you teach (Victory Academy's method):
- Friendly, encouraging and clear. Short paragraphs, simple words.
- Explain step by step, then give a real-world analogy from everyday Ethiopian life (markets, coffee ceremony, taxis, farming, football, rain seasons…) when it helps.
- When a student is stuck, offer a different explanation or analogy instead of repeating the same one.
- Use the Ethiopian national curriculum and national exam style. Use LaTeX between $…$ for maths.
- For a question with choices, guide the student to the reasoning; give the answer with a short justification.
- Stay on study topics. If asked for something unrelated or unsafe, kindly steer back to learning.
- If unsure, say so rather than inventing facts.`

func (s *Server) aiOff() error {
	return httpx.Conflict("ai_unavailable", "The AI tutor isn't switched on yet. Ask the academy to enable it.")
}

// useAI counts one AI request for the student today and refuses past the
// daily limit. Returns how many are left today.
func (s *Server) useAI(c *gin.Context, studentID string) (int, error) {
	var n int
	err := s.pool.QueryRow(c, `INSERT INTO ai_usage (student_id, day, requests) VALUES ($1, `+addisDay+`, 1)
		ON CONFLICT (student_id, day) DO UPDATE SET requests = ai_usage.requests + 1
		RETURNING requests`, studentID).Scan(&n)
	if err != nil {
		return 0, err
	}
	if n > s.cfg.AIDailyLimit {
		return 0, httpx.NewError(http.StatusTooManyRequests, "ai_limit",
			fmt.Sprintf("You've used today's %d AI requests. The tutor is back tomorrow.", s.cfg.AIDailyLimit))
	}
	return s.cfg.AIDailyLimit - n, nil
}

func (s *Server) aiFailed(err error) error {
	if errors.Is(err, ai.ErrDisabled) {
		return s.aiOff()
	}
	return httpx.NewError(http.StatusBadGateway, "ai_failed", "The AI tutor couldn't answer right now. Try again in a moment.")
}

// POST /ai/tutor {messages: [{role, content}], chapter_id?}
func (s *Server) aiTutor(c *gin.Context) error {
	var in struct {
		Messages  []ai.Message `json:"messages" binding:"required,min=1,max=30"`
		ChapterID string       `json:"chapter_id"`
	}
	if err := httpx.Bind(c, &in); err != nil {
		return err
	}
	if !s.ai.Enabled() {
		return s.aiOff()
	}
	st, err := s.me(c)
	if err != nil {
		return err
	}
	// Only the last turns go to the model; each one is bounded.
	msgs := in.Messages
	if len(msgs) > 12 {
		msgs = msgs[len(msgs)-12:]
	}
	for len(msgs) > 0 && msgs[0].Role != "user" {
		msgs = msgs[1:]
	}
	if len(msgs) == 0 || msgs[len(msgs)-1].Role != "user" {
		return httpx.BadRequest("End with the student's question.")
	}
	for i := range msgs {
		if msgs[i].Role != "user" && msgs[i].Role != "assistant" {
			return httpx.BadRequest("Unknown message role.")
		}
		if len(msgs[i].Content) > 4000 {
			msgs[i].Content = msgs[i].Content[:4000]
		}
	}
	left, err := s.useAI(c, st.ID)
	if err != nil {
		return err
	}
	academic, _ := s.academicProfile(c, st.ID)
	system := tutorSystem + fmt.Sprintf("\n\nThe student: %s, grade %s%s.", strings.Fields(st.Name + " ")[0], st.Grade, levelNote(academic))
	// In-context tutor: the chapter the student is reading, if they may open it.
	if in.ChapterID != "" {
		if ch, err := s.store.Chapter(c, in.ChapterID); err == nil && ch.Status == "published" &&
			s.checkLearningGroup(c, st, "course", ch.CourseID) == nil && s.checkLearningAccess(c, st, "chapter", ch.ID) == nil {
			text := content.PlainText(ch.HTML)
			if len(text) > 12000 {
				text = text[:12000]
			}
			system += fmt.Sprintf("\n\nThe student is reading the chapter %q. Base your explanations on it:\n<chapter>\n%s\n</chapter>", ch.Title, text)
		}
	}
	reply, err := s.ai.Complete(c, system, msgs, 1200)
	if err != nil {
		return s.aiFailed(err)
	}
	return httpx.OK(c, gin.H{"reply": reply, "remaining": left})
}

func levelNote(academic map[string]any) string {
	var parts []string
	if d, _ := academic["department"].(string); d != "" {
		parts = append(parts, map[string]string{"entrance": "Grade 12 entrance", "remedial": "remedial", "freshman": "freshman"}[d])
	}
	if st, _ := academic["stream"].(string); st != "" {
		parts = append(parts, st+" science stream")
	}
	if len(parts) == 0 {
		return ""
	}
	return ", " + strings.Join(parts, ", ")
}

type studyAnalysis struct {
	Summary       string `json:"summary"`
	Strengths     []string
	Focus         []focusArea `json:"focus"`
	WeeklyPlan    []planDay   `json:"weekly_plan"`
	Encouragement string      `json:"encouragement"`
	GeneratedAt   time.Time   `json:"generated_at"`
}

type focusArea struct {
	Area   string `json:"area"`
	Reason string `json:"reason"`
	Action string `json:"action"`
}

type planDay struct {
	Day  string `json:"day"`
	Task string `json:"task"`
}

// MarshalJSON keeps the field names stable for the app.
func (a studyAnalysis) MarshalJSON() ([]byte, error) {
	type out struct {
		Summary       string      `json:"summary"`
		Strengths     []string    `json:"strengths"`
		Focus         []focusArea `json:"focus"`
		WeeklyPlan    []planDay   `json:"weekly_plan"`
		Encouragement string      `json:"encouragement"`
		GeneratedAt   time.Time   `json:"generated_at"`
	}
	return json.Marshal(out(a))
}

func (a *studyAnalysis) UnmarshalJSON(b []byte) error {
	var in struct {
		Summary       string      `json:"summary"`
		Strengths     []string    `json:"strengths"`
		Focus         []focusArea `json:"focus"`
		WeeklyPlan    []planDay   `json:"weekly_plan"`
		Encouragement string      `json:"encouragement"`
		GeneratedAt   time.Time   `json:"generated_at"`
	}
	if err := json.Unmarshal(b, &in); err != nil {
		return err
	}
	*a = studyAnalysis(in)
	return nil
}

// studentPicture is the data the analysis and review plan are based on.
func (s *Server) studentPicture(c *gin.Context, st store.Student) (string, store.StudentStats, error) {
	stats, err := s.store.StudentStats(c, st.ID)
	if err != nil {
		return "", stats, err
	}
	var b strings.Builder
	fmt.Fprintf(&b, "Grade %s. %d exams taken, %d questions, overall accuracy %.0f%%, %d chapters completed.\n",
		st.Grade, stats.Contests, stats.Questions, stats.Accuracy*100, stats.ChaptersCompleted)
	writeStats := func(title string, m map[string]store.SubjectStat) {
		type row struct {
			name     string
			acc      float64
			n, right int
		}
		var rows []row
		for k, v := range m {
			if v.Total == 0 {
				continue
			}
			rows = append(rows, row{k, float64(v.Correct) / float64(v.Total), v.Total, v.Correct})
		}
		sort.Slice(rows, func(i, j int) bool { return rows[i].acc < rows[j].acc })
		if len(rows) > 20 {
			rows = rows[:20]
		}
		fmt.Fprintf(&b, "%s (weakest first):\n", title)
		for _, r := range rows {
			fmt.Fprintf(&b, "- %s: %d/%d correct (%.0f%%)\n", r.name, r.right, r.n, r.acc*100)
		}
	}
	writeStats("By subject", stats.Subjects)
	writeStats("By chapter", stats.Chapters)
	if len(stats.Trend) > 0 {
		b.WriteString("Monthly accuracy: ")
		for _, t := range stats.Trend {
			fmt.Fprintf(&b, "%s %.0f%% (%d questions); ", t.Month, t.Accuracy*100, t.Questions)
		}
		b.WriteString("\n")
	}
	rows, err := s.pool.Query(c, `SELECT subject, target FROM score_targets WHERE student_id = $1`, st.ID)
	if err == nil {
		for rows.Next() {
			var subject string
			var target int
			if rows.Scan(&subject, &target) == nil {
				fmt.Fprintf(&b, "Target score in %s: %d%%\n", subject, target)
			}
		}
		rows.Close()
	}
	return b.String(), stats, nil
}

// GET /ai/analysis — a personal analysis with suggestions and a weekly plan.
// Cached for the day; ?refresh=1 asks for a new one.
func (s *Server) aiAnalysis(c *gin.Context) error {
	st, err := s.me(c)
	if err != nil {
		return err
	}
	if c.Query("refresh") != "1" {
		var cached studyAnalysis
		var raw []byte
		if err := s.pool.QueryRow(c, `SELECT content FROM ai_cache WHERE owner_id = $1 AND kind = 'analysis' AND day = `+addisDay, st.ID).Scan(&raw); err == nil &&
			json.Unmarshal(raw, &cached) == nil {
			return httpx.OK(c, gin.H{"analysis": cached})
		}
	}
	if !s.ai.Enabled() {
		return s.aiOff()
	}
	picture, _, err := s.studentPicture(c, st)
	if err != nil {
		return err
	}
	if _, err := s.useAI(c, st.ID); err != nil {
		return err
	}
	var out studyAnalysis
	prompt := "Here is the student's learning data:\n" + picture + `
Write a personal study analysis as JSON with these fields:
{"summary": "2–3 sentences on how they are doing", "strengths": ["up to 3 strengths"],
 "focus": [{"area": "chapter or skill", "reason": "what the data shows", "action": "one concrete thing to do"}] (2–4 items, weakest areas first),
 "weekly_plan": [{"day": "Mon", "task": "a 20–45 minute task"}] (7 days, mix review, practice questions and one past national exam),
 "encouragement": "one warm sentence"}
If there is little data, say so kindly and plan how to start.`
	if err := s.ai.JSON(c, tutorSystem, prompt, 1500, &out); err != nil {
		return s.aiFailed(err)
	}
	out.GeneratedAt = s.now()
	raw, _ := json.Marshal(out)
	_, _ = s.pool.Exec(c, `INSERT INTO ai_cache (owner_id, kind, day, content) VALUES ($1, 'analysis', `+addisDay+`, $2)
		ON CONFLICT (owner_id, kind, day) DO UPDATE SET content = $2, created_at = now()`, st.ID, raw)
	return httpx.OK(c, gin.H{"analysis": out})
}

// POST /ai/review-quiz — the Smart Review Planner: up to 10 questions from
// the student's recurring mistakes and weakest chapters, with answers (it is
// practice). Works without AI too; with AI the intro is personal.
func (s *Server) aiReviewQuiz(c *gin.Context) error {
	st, err := s.me(c)
	if err != nil {
		return err
	}
	_, stats, err := s.studentPicture(c, st)
	if err != nil {
		return err
	}
	type weak struct {
		chapter string
		acc     float64
	}
	var weakest []weak
	for k, v := range stats.Chapters {
		if v.Total >= 2 && float64(v.Correct)/float64(v.Total) < 0.7 {
			weakest = append(weakest, weak{k, float64(v.Correct) / float64(v.Total)})
		}
	}
	sort.Slice(weakest, func(i, j int) bool { return weakest[i].acc < weakest[j].acc })
	if len(weakest) > 4 {
		weakest = weakest[:4]
	}
	chapters := make([]string, len(weakest))
	for i, w := range weakest {
		chapters[i] = w.chapter
	}
	// Questions answered wrong before (only from exams whose answers are
	// out: ended contests and national practice), then fresh questions from
	// the weakest chapters. Never questions of an exam still running.
	rows, err := s.pool.Query(c, `
		WITH missed AS (
			SELECT DISTINCT q.id, 0 AS pri FROM submissions sub
			JOIN contests co ON co.id = sub.contest_id
			JOIN contest_questions cq ON cq.contest_id = co.id
			JOIN questions q ON q.id = cq.question_id
			WHERE sub.student_id = $1 AND (co.kind = 'national' OR co.end_time < now())
			AND coalesce((sub.answers ->> q.id)::int, -1) <> q.answer_index),
		fresh AS (
			SELECT q.id, 1 AS pri FROM questions q
			WHERE q.chapter = ANY($2) AND q.grade = $3
			AND NOT EXISTS (SELECT 1 FROM contest_questions cq JOIN contests co ON co.id = cq.contest_id
				WHERE cq.question_id = q.id AND co.kind <> 'national' AND co.end_time > now())),
		pick AS (SELECT id, min(pri) AS pri FROM (SELECT * FROM missed UNION ALL SELECT * FROM fresh) x GROUP BY id)
		SELECT q.id, q.text, q.options, q.answer_index, q.explanation, q.subject, q.chapter, q.grade, q.image_url
		FROM pick JOIN questions q ON q.id = pick.id
		ORDER BY pick.pri, random() LIMIT 10`, st.ID, chapters, st.Grade)
	if err != nil {
		return err
	}
	type quizQuestion struct {
		ID          string   `json:"id"`
		Text        string   `json:"text"`
		Options     []string `json:"options"`
		AnswerIndex int      `json:"answer_index"`
		Explanation string   `json:"explanation"`
		Subject     string   `json:"subject"`
		Chapter     string   `json:"chapter"`
		Grade       string   `json:"grade"`
		ImageURL    string   `json:"image_url"`
	}
	qs := []quizQuestion{}
	for rows.Next() {
		var q quizQuestion
		if err := rows.Scan(&q.ID, &q.Text, &q.Options, &q.AnswerIndex, &q.Explanation, &q.Subject, &q.Chapter, &q.Grade, &q.ImageURL); err != nil {
			rows.Close()
			return err
		}
		qs = append(qs, q)
	}
	rows.Close()
	intro := "Here are questions from what you missed before and your weakest chapters. Take your time and read each explanation."
	if len(qs) == 0 {
		intro = "Take a few exams first: your review quiz is built from the questions you miss and your weakest chapters."
	}
	if len(qs) > 0 && s.ai.Enabled() {
		if _, err := s.useAI(c, st.ID); err == nil {
			prompt := fmt.Sprintf("Weakest chapters: %s. The student is about to take a %d-question review quiz from past mistakes and these chapters. Write 2 short, encouraging sentences telling them what to focus on.", strings.Join(chapters, ", "), len(qs))
			if text, err := s.ai.Complete(c, tutorSystem, []ai.Message{{Role: "user", Content: prompt}}, 200); err == nil && text != "" {
				intro = text
			}
		}
	}
	return httpx.OK(c, gin.H{"intro": intro, "focus": chapters, "questions": qs})
}

// GET /admin/services/reports/ai-summary — an AI summary of the report with
// key findings and recommended actions (school managers get their school).
func (s *Server) aiReportSummary(c *gin.Context) error {
	if !s.ai.Enabled() {
		return httpx.Conflict("ai_unavailable", "AI summaries are switched off (no ANTHROPIC_API_KEY on the server).")
	}
	var b strings.Builder
	for _, group := range []string{"overall", "subject", "chapter", "topic", "gender", "region", "score"} {
		rows, err := s.reportData(c, group)
		if err != nil {
			return err
		}
		if len(rows) > 25 {
			rows = rows[:25]
		}
		fmt.Fprintf(&b, "By %s:\n", group)
		for _, r := range rows {
			fmt.Fprintf(&b, "- %v: %v students, %v attempts, accuracy %v%%, %v passed, %v failed, average %v s\n",
				r["group"], r["students"], r["attempts"], r["accuracy"], r["pass"], r["fail"], r["average_seconds"])
		}
	}
	scope := "all students"
	if school, manager := managerSchool(c); manager {
		var name string
		_ = s.pool.QueryRow(c, `SELECT name FROM schools WHERE id = $1`, school).Scan(&name)
		scope = "the partner school " + name
	}
	var out struct {
		Summary  string   `json:"summary"`
		Findings []string `json:"findings"`
		Actions  []string `json:"actions"`
	}
	system := "You are an education data analyst for Victory Academy in Ethiopia. You write clear, practical summaries of exam results for school leaders. Pass mark is 50%. Be specific with numbers; don't invent data."
	prompt := fmt.Sprintf("Exam results for %s:\n%s\nReturn JSON: {\"summary\": \"3–4 sentence overview\", \"findings\": [\"3–6 key findings with numbers\"], \"actions\": [\"3–5 recommended actions for teachers and the school\"]}", scope, b.String())
	if err := s.ai.JSON(c, system, prompt, 1500, &out); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return err
		}
		return httpx.NewError(http.StatusBadGateway, "ai_failed", "The AI summary couldn't be written right now. Try again in a moment.")
	}
	return httpx.OK(c, gin.H{"summary": out.Summary, "findings": out.Findings, "actions": out.Actions, "generated_at": s.now()})
}

// GET /ai/status — whether the AI features are on and today's allowance.
func (s *Server) aiStatus(c *gin.Context) error {
	st, err := s.me(c)
	if err != nil {
		return err
	}
	var used int
	_ = s.pool.QueryRow(c, `SELECT requests FROM ai_usage WHERE student_id = $1 AND day = `+addisDay, st.ID).Scan(&used)
	return httpx.OK(c, gin.H{"enabled": s.ai.Enabled(), "remaining": max(0, s.cfg.AIDailyLimit-used), "daily_limit": s.cfg.AIDailyLimit})
}
