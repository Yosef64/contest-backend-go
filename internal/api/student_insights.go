package api

import (
	"fmt"
	"math"
	"sort"
	"strings"

	"github.com/gin-gonic/gin"

	"victor-contest-go/internal/httpx"
)

// One student's results broken down by subject and chapter, with plain
// recommendations worked out from the numbers, and (when the server has an
// AI key and it's asked for) an AI-written note for the teacher.

type insightArea struct {
	Subject  string  `json:"subject"`
	Chapter  string  `json:"chapter,omitempty"`
	Correct  int     `json:"correct"`
	Total    int     `json:"total"`
	Accuracy float64 `json:"accuracy"`
}

type studentInsights struct {
	Exams     int           `json:"exams"`
	Accuracy  float64       `json:"accuracy"`
	Recent    *float64      `json:"recent_accuracy"`  // last 5 exams
	Earlier   *float64      `json:"earlier_accuracy"` // the 5 before
	Trend     string        `json:"trend"`            // up, down, steady, new
	Subjects  []insightArea `json:"subjects"`
	Weakest   []insightArea `json:"weakest_chapters"`
	Strongest []insightArea `json:"strongest_chapters"`
	Insights  []string      `json:"insights"`
	Actions   []string      `json:"recommendations"`
	AI        *aiNote       `json:"ai,omitempty"`
}

type aiNote struct {
	Summary string   `json:"summary"`
	Actions []string `json:"actions"`
}

func pct(correct, total int) float64 {
	if total == 0 {
		return 0
	}
	return math.Round(float64(correct)/float64(total)*1000) / 10
}

// GET /admin/students/:id/insights[?ai=1]
func (s *Server) adminStudentInsights(c *gin.Context) error {
	id := c.Param("id")
	if school, manager := managerSchool(c); manager {
		var ok bool
		if err := s.pool.QueryRow(c, `SELECT EXISTS (SELECT 1 FROM academic_profiles WHERE student_id = $1 AND school_id = $2)`, id, school).Scan(&ok); err != nil {
			return err
		}
		if !ok {
			return httpx.NotFound("Student")
		}
	}
	var name string
	if err := s.pool.QueryRow(c, `SELECT name FROM students WHERE id = $1`, id).Scan(&name); err != nil {
		return httpx.NotFound("Student")
	}

	// Per exam, newest first (for the trend).
	rows, err := s.pool.Query(c, `SELECT sub.correct, sub.total FROM submissions sub
		WHERE sub.student_id = $1 AND sub.total > 0 ORDER BY sub.submitted_at DESC`, id)
	if err != nil {
		return err
	}
	var exams [][2]int
	for rows.Next() {
		var r [2]int
		if err := rows.Scan(&r[0], &r[1]); err != nil {
			rows.Close()
			return err
		}
		exams = append(exams, r)
	}
	rows.Close()
	out := studentInsights{Exams: len(exams), Subjects: []insightArea{}, Weakest: []insightArea{}, Strongest: []insightArea{}, Insights: []string{}, Actions: []string{}}
	sum := func(list [][2]int) *float64 {
		if len(list) == 0 {
			return nil
		}
		c, t := 0, 0
		for _, r := range list {
			c, t = c+r[0], t+r[1]
		}
		v := pct(c, t)
		return &v
	}
	if all := sum(exams); all != nil {
		out.Accuracy = *all
	}
	out.Recent = sum(exams[:min(5, len(exams))])
	if len(exams) > 5 {
		out.Earlier = sum(exams[5:min(10, len(exams))])
	}
	switch {
	case out.Earlier == nil:
		out.Trend = "new"
	case *out.Recent-*out.Earlier >= 5:
		out.Trend = "up"
	case *out.Earlier-*out.Recent >= 5:
		out.Trend = "down"
	default:
		out.Trend = "steady"
	}

	// Per chapter, from each answered question (exams whose answers are out).
	rows, err = s.pool.Query(c, `SELECT q.subject, coalesce(nullif(q.chapter, ''), 'General'),
			count(*) FILTER (WHERE sub.answers->>q.id = q.answer_index::text)::int, count(*)::int
		FROM submissions sub JOIN contests co ON co.id = sub.contest_id
		JOIN contest_questions cq ON cq.contest_id = co.id JOIN questions q ON q.id = cq.question_id
		WHERE sub.student_id = $1 AND (co.end_time < now() OR co.kind <> 'contest')
		GROUP BY 1, 2`, id)
	if err != nil {
		return err
	}
	var chapters []insightArea
	subjects := map[string]*insightArea{}
	for rows.Next() {
		var a insightArea
		if err := rows.Scan(&a.Subject, &a.Chapter, &a.Correct, &a.Total); err != nil {
			rows.Close()
			return err
		}
		a.Accuracy = pct(a.Correct, a.Total)
		chapters = append(chapters, a)
		sub := subjects[a.Subject]
		if sub == nil {
			sub = &insightArea{Subject: a.Subject}
			subjects[a.Subject] = sub
		}
		sub.Correct += a.Correct
		sub.Total += a.Total
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return err
	}
	for _, sub := range subjects {
		sub.Accuracy = pct(sub.Correct, sub.Total)
		out.Subjects = append(out.Subjects, *sub)
	}
	sort.Slice(out.Subjects, func(i, j int) bool { return out.Subjects[i].Accuracy > out.Subjects[j].Accuracy })

	// Chapters with enough questions to judge.
	var judged []insightArea
	for _, ch := range chapters {
		if ch.Total >= 3 {
			judged = append(judged, ch)
		}
	}
	sort.Slice(judged, func(i, j int) bool { return judged[i].Accuracy < judged[j].Accuracy })
	for _, ch := range judged {
		if ch.Accuracy < 60 && len(out.Weakest) < 5 {
			out.Weakest = append(out.Weakest, ch)
		}
	}
	for i := len(judged) - 1; i >= 0 && len(out.Strongest) < 3; i-- {
		if judged[i].Accuracy >= 75 {
			out.Strongest = append(out.Strongest, judged[i])
		}
	}

	first := strings.Fields(name)
	who := name
	if len(first) > 0 {
		who = first[0]
	}
	out.Insights, out.Actions = insightText(who, out)

	if c.Query("ai") == "1" && s.ai.Enabled() && out.Exams > 0 {
		var b strings.Builder
		fmt.Fprintf(&b, "Student: %s. Exams: %d. Overall accuracy %.1f%%. Trend: %s.\n", who, out.Exams, out.Accuracy, out.Trend)
		for _, sub := range out.Subjects {
			fmt.Fprintf(&b, "- %s: %.1f%% (%d questions)\n", sub.Subject, sub.Accuracy, sub.Total)
		}
		for _, ch := range judged {
			fmt.Fprintf(&b, "  chapter %s / %s: %.1f%% (%d questions)\n", ch.Subject, ch.Chapter, ch.Accuracy, ch.Total)
		}
		var note aiNote
		system := "You are a supportive teacher at Victory Academy in Ethiopia, preparing students for national exams. Pass mark 50%. Be specific, kind and practical; use only the numbers given."
		prompt := b.String() + "\nReturn JSON: {\"summary\": \"2–3 sentences for the teacher about this student\", \"actions\": [\"3–5 concrete study steps for the student, naming chapters\"]}"
		if err := s.ai.JSON(c, system, prompt, 700, &note); err == nil {
			out.AI = &note
		}
	}
	return httpx.OK(c, gin.H{"insights": out, "ai_available": s.ai.Enabled()})
}

// insightText turns the numbers into short findings and next steps.
func insightText(who string, in studentInsights) (insights, actions []string) {
	insights, actions = []string{}, []string{}
	if in.Exams == 0 {
		return append(insights, who+" hasn't taken an exam yet."),
			append(actions, "Start with a short practice exam to find where to focus.")
	}
	level := "below the 50% pass mark"
	switch {
	case in.Accuracy >= 75:
		level = "strong"
	case in.Accuracy >= 50:
		level = "passing"
	}
	insights = append(insights, fmt.Sprintf("%s has taken %d exams with %.0f%% accuracy overall (%s).", who, in.Exams, in.Accuracy, level))
	switch in.Trend {
	case "up":
		insights = append(insights, fmt.Sprintf("Improving: the last 5 exams average %.0f%%, up from %.0f%%.", *in.Recent, *in.Earlier))
	case "down":
		insights = append(insights, fmt.Sprintf("Slipping: the last 5 exams average %.0f%%, down from %.0f%%.", *in.Recent, *in.Earlier))
		actions = append(actions, "Check in with "+who+": recent results dropped. Review the last exams' mistakes together.")
	}
	if len(in.Subjects) > 1 {
		best, worst := in.Subjects[0], in.Subjects[len(in.Subjects)-1]
		if best.Accuracy-worst.Accuracy >= 10 {
			insights = append(insights, fmt.Sprintf("Best in %s (%.0f%%), weakest in %s (%.0f%%).", best.Subject, best.Accuracy, worst.Subject, worst.Accuracy))
		}
	}
	for i, ch := range in.Weakest {
		if i == 3 {
			break
		}
		actions = append(actions, fmt.Sprintf("Re-read %s: %s (%.0f%% correct), then retry its questions.", ch.Subject, ch.Chapter, ch.Accuracy))
	}
	if len(in.Strongest) > 0 {
		names := []string{}
		for _, ch := range in.Strongest {
			names = append(names, ch.Chapter)
		}
		insights = append(insights, "Secure chapters: "+strings.Join(names, ", ")+".")
	}
	if in.Exams < 3 {
		actions = append(actions, "Take more practice exams: a few more results make these recommendations reliable.")
	} else if in.Accuracy >= 75 && len(in.Weakest) == 0 {
		actions = append(actions, "Move on to timed past national exams to build speed.")
	}
	if len(actions) == 0 {
		actions = append(actions, "Keep a steady routine: one practice exam a week and a review of every wrong answer.")
	}
	return insights, actions
}
