package api

import (
	"crypto/rand"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"math/big"
	"sort"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/jackc/pgx/v5"

	"victor-contest-go/internal/content"
	"victor-contest-go/internal/httpx"
	"victor-contest-go/internal/live"
	"victor-contest-go/internal/store"
)

// signalRoom updates the room's Firestore document after a change (someone
// joined or left, the exam started, someone submitted, it finished), so
// phones in the room fetch it again instead of asking every few seconds.
func (s *Server) signalRoom(c *gin.Context, id string) {
	if s.rooms == nil {
		return
	}
	var room live.Room
	err := s.pool.QueryRow(c, `SELECT g.status, count(m.student_id)::int, count(m.submitted_at)::int
		FROM group_exams g LEFT JOIN group_exam_members m ON m.group_id = g.id
		WHERE g.id = $1 GROUP BY g.status`, id).Scan(&room.Status, &room.Members, &room.Submitted)
	if err == nil {
		err = s.rooms.Update(c, id, room)
	}
	if err != nil {
		slog.Warn("group room signal", "group", id, "err", err)
	}
}

// Group exams ------------------------------------------------------------------
//
// A student creates a group exam with their own questions (typed one by one
// or imported from a file in the app), shares the invitation code, and starts
// it once friends have joined the waiting room. Everyone gets the same questions and
// the same server-side deadline. Answers are saved as students go; when time
// is up the last saved answers of anyone who has not submitted are scored.
// Clients poll GET /group-exams/:id (every few seconds) and use server_now to
// line up their countdown with the server clock.

const (
	groupCodeAlphabet = "ABCDEFGHJKLMNPQRSTUVWXYZ23456789"
	groupCodeLength   = 6
	// Gap between pressing Start and the first question, so every device
	// sees the start (they poll) and begins together.
	groupStartDelay = 5 * time.Second
	// Late saves are accepted this long after the deadline (network delay
	// when the timer auto-submits).
	groupGrace     = 30 * time.Second
	groupMaxMember = 50
)

type groupExam struct {
	ID              string     `json:"id"`
	Code            string     `json:"code"`
	Title           string     `json:"title"`
	Subject         string     `json:"subject"`
	Grade           string     `json:"grade"`
	CreatorID       string     `json:"creator_id"`
	DurationMinutes int        `json:"duration_minutes"`
	QuestionCount   int        `json:"question_count"`
	Status          string     `json:"status"`
	CreatedAt       time.Time  `json:"created_at"`
	StartedAt       *time.Time `json:"started_at"`
	EndsAt          *time.Time `json:"ends_at"`
}

type groupMember struct {
	StudentID   string     `json:"student_id"`
	Name        string     `json:"name"`
	PhotoURL    string     `json:"photo_url"`
	JoinedAt    time.Time  `json:"joined_at"`
	Answered    int        `json:"answered"`
	SubmittedAt *time.Time `json:"submitted_at"`
	Correct     *int       `json:"correct,omitempty"`
	Total       *int       `json:"total,omitempty"`
	TimeSeconds *int       `json:"time_seconds,omitempty"`
	Rank        int        `json:"rank,omitempty"`
	answers     map[string]int
}

func newGroupCode() (string, error) {
	b := make([]byte, groupCodeLength)
	for i := range b {
		n, err := rand.Int(rand.Reader, big.NewInt(int64(len(groupCodeAlphabet))))
		if err != nil {
			return "", err
		}
		b[i] = groupCodeAlphabet[n.Int64()]
	}
	return string(b), nil
}

const groupCols = `g.id, g.code, g.title, g.subject, g.grade, g.creator_id, g.duration_minutes,
	(SELECT count(*) FROM group_exam_questions q WHERE q.group_id = g.id)::int,
	g.status, g.created_at, g.started_at, g.ends_at`

func scanGroup(row pgx.Row) (groupExam, error) {
	var g groupExam
	err := row.Scan(&g.ID, &g.Code, &g.Title, &g.Subject, &g.Grade, &g.CreatorID, &g.DurationMinutes, &g.QuestionCount,
		&g.Status, &g.CreatedAt, &g.StartedAt, &g.EndsAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return g, store.ErrNotFound
	}
	return g, err
}

func (s *Server) loadGroup(c *gin.Context, id string) (groupExam, error) {
	return scanGroup(s.pool.QueryRow(c, `SELECT `+groupCols+` FROM group_exams g WHERE g.id = $1`, id))
}

func (s *Server) groupMembers(c *gin.Context, id string) ([]groupMember, error) {
	rows, err := s.pool.Query(c, `SELECT m.student_id, st.name, st.photo_url, m.joined_at, m.answers, m.submitted_at, m.correct, m.total, m.time_seconds
		FROM group_exam_members m JOIN students st ON st.id = m.student_id WHERE m.group_id = $1 ORDER BY m.joined_at`, id)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []groupMember
	for rows.Next() {
		var m groupMember
		var raw []byte
		if err := rows.Scan(&m.StudentID, &m.Name, &m.PhotoURL, &m.JoinedAt, &raw, &m.SubmittedAt, &m.Correct, &m.Total, &m.TimeSeconds); err != nil {
			return nil, err
		}
		m.answers = map[string]int{}
		_ = json.Unmarshal(raw, &m.answers)
		m.Answered = len(m.answers)
		out = append(out, m)
	}
	return out, rows.Err()
}

// finishGroupIfDue scores everyone still working once time is up (their last
// saved answers count), or once all members have submitted.
func (s *Server) finishGroupIfDue(c *gin.Context, g *groupExam) error {
	if g.Status != "running" || g.EndsAt == nil {
		return nil
	}
	var waiting int
	if err := s.pool.QueryRow(c, `SELECT count(*) FROM group_exam_members WHERE group_id = $1 AND submitted_at IS NULL`, g.ID).Scan(&waiting); err != nil {
		return err
	}
	if waiting > 0 && s.now().Before(g.EndsAt.Add(groupGrace)) {
		return nil
	}
	qs, err := s.groupQuestions(c, *g)
	if err != nil {
		return err
	}
	err = pgx.BeginFunc(c, s.pool, func(tx pgx.Tx) error {
		var status string
		if err := tx.QueryRow(c, `SELECT status FROM group_exams WHERE id = $1 FOR UPDATE`, g.ID).Scan(&status); err != nil {
			return err
		}
		if status != "running" {
			return nil // finished by another request meanwhile
		}
		rows, err := tx.Query(c, `SELECT student_id, answers FROM group_exam_members WHERE group_id = $1 AND submitted_at IS NULL`, g.ID)
		if err != nil {
			return err
		}
		type pending struct {
			id      string
			answers map[string]int
		}
		var list []pending
		for rows.Next() {
			var p pending
			var raw []byte
			if err := rows.Scan(&p.id, &raw); err != nil {
				rows.Close()
				return err
			}
			p.answers = map[string]int{}
			_ = json.Unmarshal(raw, &p.answers)
			list = append(list, p)
		}
		rows.Close()
		full := int(g.EndsAt.Sub(*g.StartedAt).Seconds())
		for _, p := range list {
			_, correct := scoreAnswers(qs, p.answers)
			if _, err := tx.Exec(c, `UPDATE group_exam_members SET submitted_at = $3, correct = $4, total = $5, time_seconds = $6
				WHERE group_id = $1 AND student_id = $2`, g.ID, p.id, *g.EndsAt, correct, len(qs), full); err != nil {
				return err
			}
		}
		if _, err := tx.Exec(c, `UPDATE group_exams SET status = 'finished' WHERE id = $1`, g.ID); err != nil {
			return err
		}
		g.Status = "finished"
		return nil
	})
	if err == nil && g.Status == "finished" {
		s.signalRoom(c, g.ID)
	}
	return err
}

// groupQuestions returns the exam's questions in their fixed order.
func (s *Server) groupQuestions(c *gin.Context, g groupExam) ([]store.Question, error) {
	rows, err := s.pool.Query(c, `SELECT id, text, options, answer_index, explanation
		FROM group_exam_questions WHERE group_id = $1 ORDER BY position`, g.ID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []store.Question
	for rows.Next() {
		q := store.Question{Subject: g.Subject, Grade: g.Grade}
		if err := rows.Scan(&q.ID, &q.Text, &q.Options, &q.AnswerIndex, &q.Explanation); err != nil {
			return nil, err
		}
		out = append(out, q)
	}
	return out, rows.Err()
}

// groupState is what GET /group-exams/:id returns, shaped for the viewer.
func (s *Server) groupState(c *gin.Context, g groupExam, st store.Student) (gin.H, error) {
	if err := s.finishGroupIfDue(c, &g); err != nil {
		return nil, err
	}
	members, err := s.groupMembers(c, g.ID)
	if err != nil {
		return nil, err
	}
	var me *groupMember
	for i := range members {
		if members[i].StudentID == st.ID {
			me = &members[i]
		}
	}
	if me == nil {
		return nil, httpx.NotFound("Group exam")
	}
	now := s.now()
	out := gin.H{
		"group":      g,
		"is_creator": g.CreatorID == st.ID,
		"server_now": now,
	}
	// Scores stay hidden until the exam is over for everyone.
	if g.Status == "finished" {
		sort.SliceStable(members, func(i, j int) bool {
			a, b := members[i], members[j]
			ac, bc := deref(a.Correct), deref(b.Correct)
			if ac != bc {
				return ac > bc
			}
			return deref(a.TimeSeconds) < deref(b.TimeSeconds)
		})
		for i := range members {
			members[i].Rank = i + 1
			if i > 0 && deref(members[i].Correct) == deref(members[i-1].Correct) && deref(members[i].TimeSeconds) == deref(members[i-1].TimeSeconds) {
				members[i].Rank = members[i-1].Rank
			}
		}
	} else {
		for i := range members {
			members[i].Correct, members[i].Total = nil, nil
		}
	}
	out["participants"] = members
	if g.Status == "running" && g.StartedAt != nil && !now.Before(*g.StartedAt) {
		qs, err := s.groupQuestions(c, g)
		if err != nil {
			return nil, err
		}
		public := make([]store.PublicQuestion, len(qs))
		for i, q := range qs {
			public[i] = q.Public()
		}
		out["questions"] = public
		out["my_answers"] = me.answers
	}
	if g.Status == "finished" {
		qs, err := s.groupQuestions(c, g)
		if err != nil {
			return nil, err
		}
		out["review"] = qs // answers and explanations, after the exam
		out["my_answers"] = me.answers
	}
	return out, nil
}

func deref(p *int) int {
	if p == nil {
		return 0
	}
	return *p
}

// groupQuestionInput is one question the creator wrote.
type groupQuestionInput struct {
	Text        string   `json:"text"`
	Options     []string `json:"options"`
	AnswerIndex *int     `json:"answer_index"`
	Explanation string   `json:"explanation"`
}

const maxGroupQuestions = 100

// POST /group-exams {title, subject?, duration_minutes, questions: [...]}
func (s *Server) createGroupExam(c *gin.Context) error {
	var in struct {
		Title           string               `json:"title" binding:"max=120"`
		Subject         string               `json:"subject" binding:"max=40"`
		DurationMinutes int                  `json:"duration_minutes" binding:"required,min=1,max=300"`
		Questions       []groupQuestionInput `json:"questions"`
	}
	if err := httpx.Bind(c, &in); err != nil {
		return err
	}
	st, err := s.me(c)
	if err != nil {
		return err
	}
	if len(in.Questions) == 0 {
		return httpx.Field("questions", "Add at least one question.")
	}
	if len(in.Questions) > maxGroupQuestions {
		return httpx.Field("questions", fmt.Sprintf("A group exam can have at most %d questions.", maxGroupQuestions))
	}
	type clean struct {
		text, explanation string
		options           []string
		answer            int
	}
	qs := make([]clean, 0, len(in.Questions))
	fields := map[string]string{}
	for i, q := range in.Questions {
		key := fmt.Sprintf("questions[%d]", i)
		text := strings.TrimSpace(q.Text)
		opts := trimAll(q.Options)
		switch {
		case text == "":
			fields[key] = "The question text is empty."
		case len(text) > 2000:
			fields[key] = "The question is longer than 2000 characters."
		case len(opts) < 2 || len(opts) > 6 || len(opts) != len(q.Options):
			fields[key] = "Give 2 to 6 answer choices, none empty."
		case q.AnswerIndex == nil || *q.AnswerIndex < 0 || *q.AnswerIndex >= len(opts):
			fields[key] = "Choose the correct answer."
		case len(q.Explanation) > 2000:
			fields[key] = "The explanation is longer than 2000 characters."
		}
		if fields[key] != "" {
			continue
		}
		for j := range opts {
			if len(opts[j]) > 500 {
				fields[key] = "An answer choice is longer than 500 characters."
			}
			opts[j] = content.SanitizeText(opts[j])
		}
		if fields[key] != "" {
			continue
		}
		qs = append(qs, clean{content.SanitizeText(text), content.SanitizeText(strings.TrimSpace(q.Explanation)), opts, *q.AnswerIndex})
	}
	if len(fields) > 0 {
		return httpx.Invalid(fields)
	}
	title := strings.TrimSpace(in.Title)
	if title == "" {
		title = strings.Fields(st.Name + " ")[0] + "'s group exam"
	}
	subject := strings.TrimSpace(in.Subject)

	var g groupExam
	for attempt := 0; attempt < 5 && g.ID == ""; attempt++ {
		code, err := newGroupCode()
		if err != nil {
			return err
		}
		err = pgx.BeginFunc(c, s.pool, func(tx pgx.Tx) error {
			var id string
			if err := tx.QueryRow(c, `INSERT INTO group_exams (code, creator_id, title, subject, grade, duration_minutes)
				VALUES ($1, $2, $3, $4, $5, $6) RETURNING id`,
				code, st.ID, title, subject, st.Grade, in.DurationMinutes).Scan(&id); err != nil {
				return err
			}
			for i, q := range qs {
				if _, err := tx.Exec(c, `INSERT INTO group_exam_questions (group_id, position, text, options, answer_index, explanation)
					VALUES ($1, $2, $3, $4, $5, $6)`, id, i+1, q.text, q.options, q.answer, q.explanation); err != nil {
					return err
				}
			}
			if _, err := tx.Exec(c, `INSERT INTO group_exam_members (group_id, student_id) VALUES ($1, $2)`, id, st.ID); err != nil {
				return err
			}
			g, err = scanGroup(tx.QueryRow(c, `SELECT `+groupCols+` FROM group_exams g WHERE g.id = $1`, id))
			return err
		})
		if err != nil && !strings.Contains(err.Error(), "group_exams_code_key") {
			return err
		}
	}
	if g.ID == "" {
		return errors.New("could not generate a unique group code")
	}
	s.signalRoom(c, g.ID)
	state, err := s.groupState(c, g, st)
	if err != nil {
		return err
	}
	return httpx.Created(c, state)
}

// POST /group-exams/join {code}
func (s *Server) joinGroupExam(c *gin.Context) error {
	var in struct {
		Code string `json:"code" binding:"required,max=20"`
	}
	if err := httpx.Bind(c, &in); err != nil {
		return err
	}
	st, err := s.me(c)
	if err != nil {
		return err
	}
	code := strings.ToUpper(strings.ReplaceAll(strings.TrimSpace(in.Code), " ", ""))
	g, err := scanGroup(s.pool.QueryRow(c, `SELECT `+groupCols+` FROM group_exams g WHERE g.code = $1`, code))
	if errors.Is(err, store.ErrNotFound) {
		return httpx.Field("code", "No group exam uses this code. Check it with the person who invited you.")
	}
	if err != nil {
		return err
	}
	var already bool
	if err = s.pool.QueryRow(c, `SELECT EXISTS(SELECT 1 FROM group_exam_members WHERE group_id = $1 AND student_id = $2)`, g.ID, st.ID).Scan(&already); err != nil {
		return err
	}
	if !already {
		switch g.Status {
		case "running", "finished":
			return httpx.Conflict("group_started", "This group exam has already started, so no one else can join.")
		case "cancelled":
			return httpx.Conflict("group_cancelled", "This group exam was cancelled by its host.")
		}
		// The status is checked again inside the insert, so a join racing
		// with Start never lands after the exam began.
		tag, err := s.pool.Exec(c, `INSERT INTO group_exam_members (group_id, student_id)
			SELECT $1, $2 FROM group_exams g WHERE g.id = $1 AND g.status = 'waiting'
				AND (SELECT count(*) FROM group_exam_members WHERE group_id = $1) < $3
			ON CONFLICT DO NOTHING`, g.ID, st.ID, groupMaxMember)
		if err != nil {
			return err
		}
		if tag.RowsAffected() == 0 {
			if g, err = s.loadGroup(c, g.ID); err != nil {
				return err
			}
			if g.Status != "waiting" {
				return httpx.Conflict("group_started", "This group exam has already started, so no one else can join.")
			}
			return httpx.Conflict("group_full", "This group exam is full.")
		}
		s.signalRoom(c, g.ID)
	}
	state, err := s.groupState(c, g, st)
	if err != nil {
		return err
	}
	return httpx.OK(c, state)
}

func (s *Server) memberGroup(c *gin.Context) (groupExam, store.Student, error) {
	st, err := s.me(c)
	if err != nil {
		return groupExam{}, st, err
	}
	g, err := s.loadGroup(c, c.Param("id"))
	if err != nil {
		return g, st, storeErr(err, "Group exam")
	}
	var member bool
	if err = s.pool.QueryRow(c, `SELECT EXISTS(SELECT 1 FROM group_exam_members WHERE group_id = $1 AND student_id = $2)`, g.ID, st.ID).Scan(&member); err != nil {
		return g, st, err
	}
	if !member {
		return g, st, httpx.NotFound("Group exam")
	}
	return g, st, nil
}

// GET /group-exams/:id
func (s *Server) getGroupExam(c *gin.Context) error {
	g, st, err := s.memberGroup(c)
	if err != nil {
		return err
	}
	state, err := s.groupState(c, g, st)
	if err != nil {
		return err
	}
	return httpx.OK(c, state)
}

// GET /group-exams — the student's recent group exams.
func (s *Server) listGroupExams(c *gin.Context) error {
	st, err := s.me(c)
	if err != nil {
		return err
	}
	rows, err := s.pool.Query(c, `SELECT `+groupCols+`, (SELECT count(*) FROM group_exam_members x WHERE x.group_id = g.id)::int
		FROM group_exams g JOIN group_exam_members m ON m.group_id = g.id AND m.student_id = $1
		WHERE g.status <> 'cancelled' ORDER BY g.created_at DESC LIMIT 20`, st.ID)
	if err != nil {
		return err
	}
	defer rows.Close()
	type item struct {
		groupExam
		Participants int  `json:"participants"`
		IsCreator    bool `json:"is_creator"`
	}
	out := []item{}
	for rows.Next() {
		var it item
		g := &it.groupExam
		if err := rows.Scan(&g.ID, &g.Code, &g.Title, &g.Subject, &g.Grade, &g.CreatorID, &g.DurationMinutes, &g.QuestionCount,
			&g.Status, &g.CreatedAt, &g.StartedAt, &g.EndsAt, &it.Participants); err != nil {
			return err
		}
		it.IsCreator = g.CreatorID == st.ID
		out = append(out, it)
	}
	return httpx.OK(c, gin.H{"group_exams": out, "server_now": s.now()})
}

// POST /group-exams/:id/start — creator only.
func (s *Server) startGroupExam(c *gin.Context) error {
	g, st, err := s.memberGroup(c)
	if err != nil {
		return err
	}
	if g.CreatorID != st.ID {
		return httpx.Forbidden("Only the person who created this group exam can start it.")
	}
	start := s.now().Add(groupStartDelay)
	end := start.Add(time.Duration(g.DurationMinutes) * time.Minute)
	tag, err := s.pool.Exec(c, `UPDATE group_exams SET status = 'running', started_at = $2, ends_at = $3 WHERE id = $1 AND status = 'waiting'`, g.ID, start, end)
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		return httpx.Conflict("group_not_waiting", "This group exam has already started or ended.")
	}
	s.signalRoom(c, g.ID)
	g, err = s.loadGroup(c, g.ID)
	if err != nil {
		return err
	}
	state, err := s.groupState(c, g, st)
	if err != nil {
		return err
	}
	return httpx.OK(c, state)
}

// POST /group-exams/:id/leave — leave the waiting room; the creator cancels.
func (s *Server) leaveGroupExam(c *gin.Context) error {
	g, st, err := s.memberGroup(c)
	if err != nil {
		return err
	}
	if g.Status != "waiting" {
		return httpx.Conflict("group_started", "The exam has started; your answers are kept until time is up.")
	}
	if g.CreatorID == st.ID {
		_, err = s.pool.Exec(c, `UPDATE group_exams SET status = 'cancelled' WHERE id = $1 AND status = 'waiting'`, g.ID)
	} else {
		_, err = s.pool.Exec(c, `DELETE FROM group_exam_members WHERE group_id = $1 AND student_id = $2`, g.ID, st.ID)
	}
	if err == nil {
		s.signalRoom(c, g.ID)
	}
	return orNoContent(c, err)
}

type groupAnswers struct {
	Answers map[string]int `json:"answers"`
}

// cleanAnswers keeps answers for this exam's questions and valid options.
func cleanAnswers(qs []store.Question, in map[string]int) map[string]int {
	out, _ := scoreAnswers(qs, in)
	return out
}

// PUT /group-exams/:id/answers — saves progress (any time while running).
func (s *Server) saveGroupAnswers(c *gin.Context) error {
	var in groupAnswers
	if err := httpx.Bind(c, &in); err != nil {
		return err
	}
	g, st, err := s.memberGroup(c)
	if err != nil {
		return err
	}
	now := s.now()
	if g.Status != "running" || g.StartedAt == nil || now.Before(*g.StartedAt) {
		return httpx.Conflict("group_not_running", "The exam isn't running.")
	}
	if now.After(g.EndsAt.Add(groupGrace)) {
		return httpx.Conflict("group_time_up", "Time is up; your last saved answers were counted.")
	}
	qs, err := s.groupQuestions(c, g)
	if err != nil {
		return err
	}
	raw, _ := json.Marshal(cleanAnswers(qs, in.Answers))
	tag, err := s.pool.Exec(c, `UPDATE group_exam_members SET answers = $3, saved_at = now()
		WHERE group_id = $1 AND student_id = $2 AND submitted_at IS NULL`, g.ID, st.ID, raw)
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		return httpx.Conflict("already_submitted", "You have already submitted this exam.")
	}
	return httpx.OK(c, gin.H{"saved_at": now, "server_now": now})
}

// POST /group-exams/:id/submit — final answers.
func (s *Server) submitGroupExam(c *gin.Context) error {
	var in groupAnswers
	if err := httpx.Bind(c, &in); err != nil {
		return err
	}
	g, st, err := s.memberGroup(c)
	if err != nil {
		return err
	}
	now := s.now()
	if g.Status != "running" || g.StartedAt == nil || now.Before(*g.StartedAt) {
		if g.Status == "finished" {
			return httpx.Conflict("group_time_up", "Time is up; your last saved answers were counted.")
		}
		return httpx.Conflict("group_not_running", "The exam isn't running.")
	}
	if now.After(g.EndsAt.Add(groupGrace)) {
		return httpx.Conflict("group_time_up", "Time is up; your last saved answers were counted.")
	}
	qs, err := s.groupQuestions(c, g)
	if err != nil {
		return err
	}
	answers, correct := scoreAnswers(qs, in.Answers)
	raw, _ := json.Marshal(answers)
	spent := int(now.Sub(*g.StartedAt).Seconds())
	if limit := int(g.EndsAt.Sub(*g.StartedAt).Seconds()); spent > limit {
		spent = limit
	}
	tag, err := s.pool.Exec(c, `UPDATE group_exam_members SET answers = $3, saved_at = now(), submitted_at = now(),
			correct = $4, total = $5, time_seconds = $6
		WHERE group_id = $1 AND student_id = $2 AND submitted_at IS NULL`, g.ID, st.ID, raw, correct, len(qs), spent)
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		return httpx.Conflict("already_submitted", "You have already submitted this exam.")
	}
	s.signalRoom(c, g.ID)
	state, err := s.groupState(c, g, st)
	if err != nil {
		return err
	}
	return httpx.OK(c, state)
}
