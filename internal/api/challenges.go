package api

import (
	"errors"
	"fmt"
	"net/url"
	"strconv"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/jackc/pgx/v5"

	"victor-contest-go/internal/httpx"
	"victor-contest-go/internal/store"
)

// Challenges ----------------------------------------------------------------
//
// A challenge runs for a number of weeks; four weeks make a month on its
// roadmap. Each day has a lesson, then a short note, then a quiz (each one
// optional); every seventh day is the week's exam. A day opens when every
// day before it is finished, so a new student works through the same path.
//
// Students on a paid plan open every day. For the others, an admin decides
// per challenge: the first free_days are open, invites can unlock lesson
// days (per day or per week) and weekly exams, and from premium_from_day on
// only paid students get in.

const daysPerWeek = 7

// weeksPerMonth groups weeks into months on the roadmap.
const weeksPerMonth = 4

type challenge struct {
	ID                  string     `json:"id"`
	Title               string     `json:"title"`
	Description         string     `json:"description"`
	Grade               string     `json:"grade"`
	Stream              string     `json:"stream"`
	Weeks               int        `json:"weeks"`
	StartsOn            *time.Time `json:"starts_on"`
	Status              string     `json:"status"`
	FreeDays            int        `json:"free_days"`
	ReferralEnabled     bool       `json:"referral_enabled"`
	ReferralUnit        string     `json:"referral_unit"`
	ExamReferralEnabled bool       `json:"exam_referral_enabled"`
	PremiumFromDay      *int       `json:"premium_from_day"`
	FinalAccess         string     `json:"final_access"`
	Position            int        `json:"position"`
	CreatedBy           string     `json:"created_by"`
	CreatedAt           time.Time  `json:"created_at"`
	UpdatedAt           time.Time  `json:"updated_at"`
}

func (ch challenge) days() int { return ch.Weeks * daysPerWeek }

const challengeCols = `id, title, description, grade, stream, weeks, starts_on, status, free_days, referral_enabled,
	referral_unit, exam_referral_enabled, premium_from_day, final_access, position, created_by, created_at, updated_at`

func scanChallenge(row pgx.Row) (challenge, error) {
	var ch challenge
	err := row.Scan(&ch.ID, &ch.Title, &ch.Description, &ch.Grade, &ch.Stream, &ch.Weeks, &ch.StartsOn, &ch.Status,
		&ch.FreeDays, &ch.ReferralEnabled, &ch.ReferralUnit, &ch.ExamReferralEnabled, &ch.PremiumFromDay,
		&ch.FinalAccess, &ch.Position, &ch.CreatedBy, &ch.CreatedAt, &ch.UpdatedAt)
	return ch, err
}

func (s *Server) loadChallenge(c *gin.Context, id string) (challenge, error) {
	ch, err := scanChallenge(s.pool.QueryRow(c, `SELECT `+challengeCols+` FROM challenges WHERE id = $1`, id))
	if errors.Is(err, pgx.ErrNoRows) {
		return ch, httpx.NotFound("Challenge")
	}
	return ch, err
}

func weekOf(day int) int  { return (day-1)/daysPerWeek + 1 }
func monthOf(day int) int { return (weekOf(day)-1)/weeksPerMonth + 1 }
func isExamDay(day int) bool {
	return day%daysPerWeek == 0
}

// dayQuiz is a day's quiz or weekly exam as the app lists it.
type dayQuiz struct {
	ID              string `json:"id"`
	Questions       int    `json:"questions"`
	DurationMinutes int    `json:"duration_minutes"`
	// The student's result, once submitted.
	Correct *int `json:"correct,omitempty"`
	Total   *int `json:"total,omitempty"`
}

// challengeLock says why a day can't be opened yet.
type challengeLock struct {
	// "previous": an earlier day isn't finished; "soon": not prepared yet;
	// "premium": paid students only; "referral": invite two new students.
	Reason   string `json:"reason"`
	Invited  int    `json:"invited,omitempty"`
	Needed   int    `json:"needed,omitempty"`
	ShareURL string `json:"share_url,omitempty"`
	// What an invite unlocks: "day", "week", "exam" or "final".
	Scope string `json:"scope,omitempty"`
}

type challengeDayState struct {
	Day     int        `json:"day"`
	Week    int        `json:"week"`
	Month   int        `json:"month"`
	Date    *time.Time `json:"date,omitempty"`
	IsExam  bool       `json:"is_exam"`
	Title   string     `json:"title"`
	Subject string     `json:"subject"`
	Minutes int        `json:"minutes"`
	// The day's own access ('' follows the challenge's rules): "free",
	// "referral" or "premium".
	Access string `json:"access"`

	HasLesson bool     `json:"has_lesson"`
	HasNote   bool     `json:"has_note"`
	Quiz      *dayQuiz `json:"quiz"`

	LessonDone bool `json:"lesson_done"`
	NoteDone   bool `json:"note_done"`
	QuizDone   bool `json:"quiz_done"`
	// "done", "open" or "locked".
	Status string         `json:"status"`
	Lock   *challengeLock `json:"lock,omitempty"`
	// Steps finished out of the steps the day has.
	StepsDone  int `json:"steps_done"`
	StepsTotal int `json:"steps_total"`

	// Content, filled in only when one day is opened.
	LessonHTML string `json:"lesson_html,omitempty"`
	// An exam day's lessons ("Day 8 · Limits"), when one day is opened.
	Covers   []string `json:"covers,omitempty"`
	NoteHTML string   `json:"note_html,omitempty"`
}

func (d *challengeDayState) prepared() bool { return d.HasLesson || d.HasNote || d.Quiz != nil }

func (d *challengeDayState) finished() bool {
	return d.prepared() && (!d.HasLesson || d.LessonDone) && (!d.HasNote || d.NoteDone) && (d.Quiz == nil || d.QuizDone)
}

type refKey struct {
	kind string
	unit int
}

// challengeState is one student's view of a challenge.
type challengeState struct {
	challenge
	Days []*challengeDayState `json:"-"`
	Paid bool                 `json:"paid"`
	// Invites count for this challenge (the admin switch and the global one).
	Referrals  bool `json:"-"`
	refs       map[refKey]int
	DaysDone   int            `json:"days_done"`
	TotalDays  int            `json:"total_days"`
	CurrentDay int            `json:"current_day"`
	Completed  bool           `json:"completed"`
	FinalLock  *challengeLock `json:"final_lock,omitempty"`
	shareBase  string
}

// referralsOn reads the global switch for challenge invites.
func (s *Server) referralsOn(c *gin.Context) (bool, error) {
	var v string
	err := s.pool.QueryRow(c, `SELECT value FROM site_settings WHERE key = 'challenge_referrals'`).Scan(&v)
	if errors.Is(err, pgx.ErrNoRows) {
		return true, nil
	}
	return v != "off", err
}

// paidNow reports an approved, unexpired academic plan.
func (s *Server) paidNow(c *gin.Context, studentID string) (bool, error) {
	var paid bool
	err := s.pool.QueryRow(c, `SELECT EXISTS (SELECT 1 FROM enrollment_requests WHERE student_id = $1 AND module = $2
		AND status = 'approved' AND expires_at > now())`, studentID, premiumModule).Scan(&paid)
	return paid, err
}

// challengeStateFor loads the challenge's days with this student's progress
// and works out which days are open.
func (s *Server) challengeStateFor(c *gin.Context, st store.Student, ch challenge) (*challengeState, error) {
	cs := &challengeState{challenge: ch, refs: map[refKey]int{}, TotalDays: ch.days(),
		shareBase: fmt.Sprintf("%s/c/%s/%s", s.cfg.PublicBaseURL, url.PathEscape(st.ID), url.PathEscape(ch.ID))}
	var err error
	if cs.Paid, err = s.paidNow(c, st.ID); err != nil {
		return nil, err
	}
	on, err := s.referralsOn(c)
	if err != nil {
		return nil, err
	}
	cs.Referrals = on

	byDay := map[int]*challengeDayState{}
	for d := 1; d <= ch.days(); d++ {
		ds := &challengeDayState{Day: d, Week: weekOf(d), Month: monthOf(d), IsExam: isExamDay(d), Minutes: 30}
		if ch.StartsOn != nil {
			date := ch.StartsOn.AddDate(0, 0, d-1)
			ds.Date = &date
		}
		byDay[d] = ds
		cs.Days = append(cs.Days, ds)
	}

	rows, err := s.pool.Query(c, `SELECT d.day, d.title, d.subject, d.minutes, d.access, d.lesson_html <> '', d.note_html <> '',
			d.quiz_id, coalesce(co.duration_minutes, 0),
			(SELECT count(*) FROM contest_questions cq WHERE cq.contest_id = d.quiz_id)::int,
			sub.correct, sub.total,
			EXISTS (SELECT 1 FROM challenge_steps x WHERE x.challenge_id = d.challenge_id AND x.day = d.day AND x.student_id = $2 AND x.step = 'lesson'),
			EXISTS (SELECT 1 FROM challenge_steps x WHERE x.challenge_id = d.challenge_id AND x.day = d.day AND x.student_id = $2 AND x.step = 'note')
		FROM challenge_days d
		LEFT JOIN contests co ON co.id = d.quiz_id
		LEFT JOIN submissions sub ON sub.contest_id = d.quiz_id AND sub.student_id = $2
		WHERE d.challenge_id = $1`, ch.ID, st.ID)
	if err != nil {
		return nil, err
	}
	for rows.Next() {
		var day, minutes, duration, questions int
		var title, subject, access string
		var lesson, note, lessonDone, noteDone bool
		var quizID *string
		var correct, total *int
		if err := rows.Scan(&day, &title, &subject, &minutes, &access, &lesson, &note, &quizID, &duration, &questions,
			&correct, &total, &lessonDone, &noteDone); err != nil {
			rows.Close()
			return nil, err
		}
		ds := byDay[day]
		if ds == nil {
			continue // past the challenge's length
		}
		ds.Title, ds.Subject, ds.Minutes, ds.Access = title, subject, minutes, access
		ds.HasLesson, ds.HasNote = lesson, note
		ds.LessonDone, ds.NoteDone = lessonDone, noteDone
		// A quiz without questions is still being prepared.
		if quizID != nil && questions > 0 {
			ds.Quiz = &dayQuiz{ID: *quizID, Questions: questions, DurationMinutes: duration, Correct: correct, Total: total}
			ds.QuizDone = correct != nil
		}
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return nil, err
	}

	rows, err = s.pool.Query(c, `SELECT kind, unit, count(*)::int FROM challenge_referrals
		WHERE challenge_id = $1 AND owner_id = $2 GROUP BY kind, unit`, ch.ID, st.ID)
	if err != nil {
		return nil, err
	}
	for rows.Next() {
		var k refKey
		var n int
		if err := rows.Scan(&k.kind, &k.unit, &n); err != nil {
			rows.Close()
			return nil, err
		}
		cs.refs[k] = n
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return nil, err
	}

	earlierDone := true
	for _, ds := range cs.Days {
		ds.StepsTotal, ds.StepsDone = 0, 0
		for _, step := range [][2]bool{{ds.HasLesson, ds.LessonDone}, {ds.HasNote, ds.NoteDone}, {ds.Quiz != nil, ds.QuizDone}} {
			if step[0] {
				ds.StepsTotal++
				if step[1] {
					ds.StepsDone++
				}
			}
		}
		switch {
		case ds.finished() && earlierDone:
			ds.Status = "done"
			cs.DaysDone++
		case !earlierDone:
			ds.Status, ds.Lock = "locked", &challengeLock{Reason: "previous"}
		case !ds.prepared():
			ds.Status, ds.Lock = "locked", &challengeLock{Reason: "soon"}
		default:
			if lock := cs.dayLock(ds); lock != nil {
				ds.Status, ds.Lock = "locked", lock
			} else {
				ds.Status = "open"
			}
		}
		if ds.Status != "done" {
			if earlierDone && cs.CurrentDay == 0 {
				cs.CurrentDay = ds.Day
			}
			earlierDone = false
		}
	}
	cs.Completed = cs.DaysDone == cs.TotalDays && cs.TotalDays > 0
	if cs.CurrentDay == 0 {
		cs.CurrentDay = cs.TotalDays
	}
	cs.FinalLock = cs.finalLock()
	return cs, nil
}

// dayLock is the payment / invite lock on a day the student has reached.
func (cs *challengeState) dayLock(ds *challengeDayState) *challengeLock {
	if cs.Paid {
		return nil
	}
	// A day's own access wins over the general rules.
	switch ds.Access {
	case "free":
		return nil
	case "premium":
		return &challengeLock{Reason: "premium"}
	case "referral":
		if !cs.Referrals {
			return &challengeLock{Reason: "premium"}
		}
		return cs.inviteLock("day", ds.Day, "day")
	}
	if ds.Day <= cs.FreeDays {
		return nil
	}
	if cs.PremiumFromDay != nil && ds.Day >= *cs.PremiumFromDay {
		return &challengeLock{Reason: "premium"}
	}
	kind, unit, scope := "lesson", ds.Week, "week"
	enabled := cs.ReferralEnabled
	switch {
	case ds.IsExam:
		kind, scope, enabled = "exam", "exam", cs.ExamReferralEnabled
	case cs.ReferralUnit == "day":
		unit, scope = ds.Day, "day"
	}
	if !enabled || !cs.Referrals {
		return &challengeLock{Reason: "premium"}
	}
	return cs.inviteLock(kind, unit, scope)
}

// inviteLock is nil once two new students joined through this unit's link.
func (cs *challengeState) inviteLock(kind string, unit int, scope string) *challengeLock {
	n := cs.refs[refKey{kind, unit}]
	if n >= referralsNeeded {
		return nil
	}
	return &challengeLock{Reason: "referral", Invited: n, Needed: referralsNeeded, Scope: scope,
		ShareURL: fmt.Sprintf("%s/%s/%d", cs.shareBase, kind, unit)}
}

// finalLock covers the model and past exams offered at the end.
func (cs *challengeState) finalLock() *challengeLock {
	switch {
	case !cs.Completed:
		return &challengeLock{Reason: "previous"}
	case cs.Paid || cs.FinalAccess == "free":
		return nil
	case cs.FinalAccess == "referral" && cs.Referrals:
		return cs.inviteLock("final", 0, "final")
	}
	return &challengeLock{Reason: "premium"}
}

func (cs *challengeState) day(n int) (*challengeDayState, error) {
	if n < 1 || n > len(cs.Days) {
		return nil, httpx.NotFound("Day")
	}
	return cs.Days[n-1], nil
}

// lockError explains a locked day to the app.
func lockError(l *challengeLock) error {
	switch l.Reason {
	case "previous":
		return httpx.Conflict("challenge_previous", "Finish the earlier days first.")
	case "soon":
		return httpx.Conflict("challenge_soon", "This day is still being prepared.")
	case "referral":
		return httpx.NewError(403, "challenge_referral", fmt.Sprintf("Invite %d new students to unlock this.", l.Needed))
	}
	return httpx.NewError(403, "challenge_premium", "This part of the challenge is for students on a paid plan.")
}

// visibleTo: published, for the student's grade and (when set) stream.
func (s *Server) challengeVisible(c *gin.Context, st store.Student, ch challenge) error {
	// For the student's grade, or their department (remedial, freshman).
	if ch.Status != "published" || ch.Grade != st.Grade && ch.Grade != s.studentDepartment(c, st.ID) {
		return httpx.NotFound("Challenge")
	}
	if ch.Stream == "" {
		return nil
	}
	var stream string
	err := s.pool.QueryRow(c, `SELECT stream FROM academic_profiles WHERE student_id = $1`, st.ID).Scan(&stream)
	if err != nil && !errors.Is(err, pgx.ErrNoRows) {
		return err
	}
	if stream != "" && stream != ch.Stream {
		return httpx.NotFound("Challenge")
	}
	return nil
}

// studentDepartment is "entrance", "remedial" or "freshman" ("" without a
// profile).
func (s *Server) studentDepartment(c *gin.Context, studentID string) string {
	var dep string
	_ = s.pool.QueryRow(c, `SELECT department FROM academic_profiles WHERE student_id = $1`, studentID).Scan(&dep)
	return dep
}

// Student API ---------------------------------------------------------------

type challengeSummary struct {
	challenge
	TotalDays  int  `json:"total_days"`
	DaysDone   int  `json:"days_done"`
	CurrentDay int  `json:"current_day"`
	Joined     bool `json:"joined"`
	Completed  bool `json:"completed"`
	Members    int  `json:"members"`
}

// GET /challenges — the challenges for the student's grade and stream.
func (s *Server) appChallenges(c *gin.Context) error {
	st, err := s.me(c)
	if err != nil {
		return err
	}
	rows, err := s.pool.Query(c, `SELECT `+challengeCols+` FROM challenges WHERE status = 'published' AND (grade = $1 OR grade = $2)
		ORDER BY position, created_at DESC`, st.Grade, s.studentDepartment(c, st.ID))
	if err != nil {
		return err
	}
	var list []challenge
	for rows.Next() {
		ch, err := scanChallenge(rows)
		if err != nil {
			rows.Close()
			return err
		}
		list = append(list, ch)
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return err
	}
	out := []challengeSummary{}
	for _, ch := range list {
		if s.challengeVisible(c, st, ch) != nil {
			continue
		}
		cs, err := s.challengeStateFor(c, st, ch)
		if err != nil {
			return err
		}
		sum := challengeSummary{challenge: ch, TotalDays: cs.TotalDays, DaysDone: cs.DaysDone, CurrentDay: cs.CurrentDay, Completed: cs.Completed}
		if err := s.pool.QueryRow(c, `SELECT EXISTS (SELECT 1 FROM challenge_members WHERE challenge_id = $1 AND student_id = $2),
				(SELECT count(*) FROM challenge_members WHERE challenge_id = $1)::int`, ch.ID, st.ID).Scan(&sum.Joined, &sum.Members); err != nil {
			return err
		}
		out = append(out, sum)
	}
	return httpx.OK(c, gin.H{"challenges": out})
}

type roadmapWeek struct {
	Week     int                  `json:"week"`
	Days     []*challengeDayState `json:"days"`
	DaysDone int                  `json:"days_done"`
}

type roadmapMonth struct {
	Month    int           `json:"month"`
	Weeks    []roadmapWeek `json:"weeks"`
	DaysDone int           `json:"days_done"`
	Days     int           `json:"days"`
}

type finalExam struct {
	ID              string `json:"id"`
	Kind            string `json:"kind"`
	Title           string `json:"title"`
	Subject         string `json:"subject"`
	ExamYear        *int   `json:"exam_year"`
	DurationMinutes *int   `json:"duration_minutes"`
	Questions       int    `json:"questions"`
	Correct         *int   `json:"correct,omitempty"`
	Total           *int   `json:"total,omitempty"`
}

// GET /challenges/:id — the roadmap: months, weeks and days with progress.
func (s *Server) appChallenge(c *gin.Context) error {
	st, cs, err := s.studentChallenge(c)
	if err != nil {
		return err
	}
	if _, err := s.pool.Exec(c, `INSERT INTO challenge_members (challenge_id, student_id) VALUES ($1, $2) ON CONFLICT DO NOTHING`, cs.ID, st.ID); err != nil {
		return err
	}
	months := []roadmapMonth{}
	for _, d := range cs.Days {
		if len(months) < d.Month {
			months = append(months, roadmapMonth{Month: d.Month, Weeks: []roadmapWeek{}})
		}
		m := &months[d.Month-1]
		if len(m.Weeks) == 0 || m.Weeks[len(m.Weeks)-1].Week != d.Week {
			m.Weeks = append(m.Weeks, roadmapWeek{Week: d.Week, Days: []*challengeDayState{}})
		}
		w := &m.Weeks[len(m.Weeks)-1]
		w.Days = append(w.Days, d)
		m.Days++
		if d.Status == "done" {
			w.DaysDone++
			m.DaysDone++
		}
	}
	finals, err := s.finalExams(c, cs.ID, st.ID)
	if err != nil {
		return err
	}
	return httpx.OK(c, gin.H{"challenge": cs, "months": months, "final_exams": finals,
		"weeks_per_month": weeksPerMonth, "total_weeks": cs.Weeks, "total_months": (cs.Weeks + weeksPerMonth - 1) / weeksPerMonth})
}

func (s *Server) finalExams(c *gin.Context, challengeID, studentID string) ([]finalExam, error) {
	rows, err := s.pool.Query(c, `SELECT co.id, co.kind, co.title, co.subject, co.exam_year, co.duration_minutes,
			(SELECT count(*) FROM contest_questions cq WHERE cq.contest_id = co.id)::int, sub.correct, sub.total
		FROM challenge_final_exams f JOIN contests co ON co.id = f.contest_id
		LEFT JOIN submissions sub ON sub.contest_id = co.id AND sub.student_id = $2
		WHERE f.challenge_id = $1 ORDER BY f.position`, challengeID, studentID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []finalExam{}
	for rows.Next() {
		var f finalExam
		if err := rows.Scan(&f.ID, &f.Kind, &f.Title, &f.Subject, &f.ExamYear, &f.DurationMinutes, &f.Questions, &f.Correct, &f.Total); err != nil {
			return nil, err
		}
		if f.Questions > 0 {
			out = append(out, f)
		}
	}
	return out, rows.Err()
}

func (s *Server) studentChallenge(c *gin.Context) (store.Student, *challengeState, error) {
	st, err := s.me(c)
	if err != nil {
		return st, nil, err
	}
	ch, err := s.loadChallenge(c, c.Param("id"))
	if err != nil {
		return st, nil, err
	}
	if err := s.challengeVisible(c, st, ch); err != nil {
		return st, nil, err
	}
	cs, err := s.challengeStateFor(c, st, ch)
	return st, cs, err
}

// openDay returns the day the student asked for if it is open to them.
func (s *Server) openDay(c *gin.Context) (store.Student, *challengeState, *challengeDayState, error) {
	st, cs, err := s.studentChallenge(c)
	if err != nil {
		return st, nil, nil, err
	}
	n, _ := strconv.Atoi(c.Param("day"))
	ds, err := cs.day(n)
	if err != nil {
		return st, nil, nil, err
	}
	if ds.Lock != nil {
		return st, nil, nil, lockError(ds.Lock)
	}
	return st, cs, ds, nil
}

// GET /challenges/:id/days/:day — the day's lesson and note.
func (s *Server) appChallengeDay(c *gin.Context) error {
	_, cs, ds, err := s.openDay(c)
	if err != nil {
		return err
	}
	if err := s.pool.QueryRow(c, `SELECT lesson_html, note_html FROM challenge_days WHERE challenge_id = $1 AND day = $2`,
		cs.ID, ds.Day).Scan(&ds.LessonHTML, &ds.NoteHTML); err != nil {
		return err
	}
	// An exam day lists the lessons it covers: the week's other days.
	covers := []string{}
	if ds.IsExam {
		for _, d := range cs.Days {
			if d.Week == ds.Week && !d.IsExam && d.Title != "" {
				covers = append(covers, fmt.Sprintf("Day %d · %s", d.Day, d.Title))
			}
		}
	}
	ds.Covers = covers
	return httpx.OK(c, gin.H{"day": ds, "challenge_title": cs.Title})
}

// POST /challenges/:id/days/:day/steps/:step — the lesson or note is read.
// The note opens after the lesson.
func (s *Server) appChallengeStep(c *gin.Context) error {
	st, cs, ds, err := s.openDay(c)
	if err != nil {
		return err
	}
	step := c.Param("step")
	switch {
	case step == "lesson" && !ds.HasLesson, step == "note" && !ds.HasNote:
		return httpx.NotFound("Step")
	case step != "lesson" && step != "note":
		return httpx.NotFound("Step")
	case step == "note" && ds.HasLesson && !ds.LessonDone:
		return httpx.Conflict("challenge_step", "Read the lesson first.")
	}
	if _, err := s.pool.Exec(c, `INSERT INTO challenge_steps (challenge_id, day, student_id, step) VALUES ($1, $2, $3, $4)
		ON CONFLICT DO NOTHING`, cs.ID, ds.Day, st.ID, step); err != nil {
		return err
	}
	cs, err = s.challengeStateFor(c, st, cs.challenge)
	if err != nil {
		return err
	}
	next := (*challengeDayState)(nil)
	if ds.Day < len(cs.Days) {
		next = cs.Days[ds.Day]
	}
	return httpx.OK(c, gin.H{"day": cs.Days[ds.Day-1], "next": next})
}

// GET /challenges/:id/days/:day/leaderboard — a weekly exam's ranking: every
// student who has taken it, so a late joiner is ranked with earlier ones.
func (s *Server) appChallengeLeaderboard(c *gin.Context) error {
	st, cs, err := s.studentChallenge(c)
	if err != nil {
		return err
	}
	n, _ := strconv.Atoi(c.Param("day"))
	ds, err := cs.day(n)
	if err != nil {
		return err
	}
	if ds.Quiz == nil {
		return httpx.NotFound("Exam")
	}
	if !ds.QuizDone {
		return httpx.Conflict("not_submitted", "Take the exam to see the ranking.")
	}
	rows, err := s.store.ServiceRows(c, `WITH ranked AS (
			SELECT rank() OVER (ORDER BY sub.correct DESC, sub.time_seconds ASC)::int AS rank,
				st.id, st.name, st.photo_url, sub.correct, sub.total, sub.time_seconds,
				CASE WHEN sub.total = 0 THEN 0 ELSE round(sub.correct::numeric / sub.total * 100)::int END AS score
			FROM submissions sub JOIN students st ON st.id = sub.student_id WHERE sub.contest_id = $1)
		SELECT jsonb_build_object('rank', rank, 'student_id', id, 'name', name, 'photo_url', photo_url, 'correct', correct,
			'total', total, 'score', score, 'time_seconds', time_seconds, 'me', id = $2)
		FROM ranked WHERE rank <= 50 OR id = $2 ORDER BY rank, name`, ds.Quiz.ID, st.ID)
	if err != nil {
		return err
	}
	var takers int
	if err := s.pool.QueryRow(c, `SELECT count(*) FROM submissions WHERE contest_id = $1`, ds.Quiz.ID).Scan(&takers); err != nil {
		return err
	}
	return httpx.OK(c, gin.H{"standings": rows, "takers": takers, "week": ds.Week})
}

// challengeContestAccess decides access to a challenge's quizzes, weekly
// exams and final exams. handled is false for other exams.
func (s *Server) challengeContestAccess(c *gin.Context, st store.Student, contestID string) (handled bool, err error) {
	var challengeID string
	var day int
	err = s.pool.QueryRow(c, `SELECT challenge_id, day FROM challenge_days WHERE quiz_id = $1`, contestID).Scan(&challengeID, &day)
	if err == nil {
		ch, err := s.loadChallenge(c, challengeID)
		if err != nil {
			return true, err
		}
		if err := s.challengeVisible(c, st, ch); err != nil {
			return true, err
		}
		cs, err := s.challengeStateFor(c, st, ch)
		if err != nil {
			return true, err
		}
		ds, err := cs.day(day)
		if err != nil {
			return true, err
		}
		if ds.Lock != nil {
			return true, lockError(ds.Lock)
		}
		if ds.HasLesson && !ds.LessonDone || ds.HasNote && !ds.NoteDone {
			return true, httpx.Conflict("challenge_step", "Read the lesson and the short note first.")
		}
		return true, nil
	}
	if !errors.Is(err, pgx.ErrNoRows) {
		return true, err
	}

	// Model exams open only through a challenge the student has finished.
	var kind string
	if err := s.pool.QueryRow(c, `SELECT kind FROM contests WHERE id = $1`, contestID).Scan(&kind); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return false, nil
		}
		return true, err
	}
	if kind != "model" && kind != "challenge" {
		return false, nil
	}
	rows, err := s.pool.Query(c, `SELECT `+challengeCols+` FROM challenges WHERE id IN
		(SELECT challenge_id FROM challenge_final_exams WHERE contest_id = $1)`, contestID)
	if err != nil {
		return true, err
	}
	var linked []challenge
	for rows.Next() {
		ch, err := scanChallenge(rows)
		if err != nil {
			rows.Close()
			return true, err
		}
		linked = append(linked, ch)
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return true, err
	}
	var last error = httpx.Forbidden("This exam opens at the end of a challenge.")
	for _, ch := range linked {
		if s.challengeVisible(c, st, ch) != nil {
			continue
		}
		cs, err := s.challengeStateFor(c, st, ch)
		if err != nil {
			return true, err
		}
		if cs.FinalLock == nil {
			return true, nil
		}
		if cs.FinalLock.Reason == "previous" {
			last = httpx.Conflict("challenge_previous", "Finish the challenge to open its model and past exams.")
		} else {
			last = lockError(cs.FinalLock)
		}
	}
	return true, last
}

// Invites ---------------------------------------------------------------------

// POST /challenges/referrals — a new student opened a friend's challenge
// link. Each new account counts once, for one friend and one unlock.
func (s *Server) appChallengeReferral(c *gin.Context) error {
	st, err := s.me(c)
	if err != nil {
		return err
	}
	var in struct {
		OwnerID     string `json:"owner_id" binding:"required,max=120"`
		ChallengeID string `json:"challenge_id" binding:"required,max=120"`
		Kind        string `json:"kind" binding:"required,oneof=lesson exam final day"`
		Unit        int    `json:"unit" binding:"min=0,max=1000"`
	}
	if err := httpx.Bind(c, &in); err != nil {
		return err
	}
	if in.OwnerID == st.ID {
		return httpx.BadRequest("Your own link does not count. Share it with a friend.")
	}
	if s.now().Sub(st.CreatedAt) > referralWindow {
		return httpx.Conflict("not_new", "Invites count for new students only, so this one can't help your friend.")
	}
	ch, err := s.loadChallenge(c, in.ChallengeID)
	if err != nil {
		return err
	}
	on, err := s.referralsOn(c)
	if err != nil {
		return err
	}
	enabled := map[string]bool{"lesson": ch.ReferralEnabled, "exam": ch.ExamReferralEnabled, "final": ch.FinalAccess == "referral"}[in.Kind]
	if in.Kind == "day" {
		// A day set to "invite" on its own.
		if err := s.pool.QueryRow(c, `SELECT EXISTS (SELECT 1 FROM challenge_days WHERE challenge_id = $1 AND day = $2 AND access = 'referral')`,
			ch.ID, in.Unit).Scan(&enabled); err != nil {
			return err
		}
	}
	if ch.Status != "published" || !on || !enabled {
		return httpx.Conflict("referrals_off", "Invites aren't open for this challenge right now.")
	}
	// The same phone signing in to both accounts doesn't count.
	var sameDevice, ownerExists bool
	if err := s.pool.QueryRow(c, `SELECT EXISTS (SELECT 1 FROM students WHERE id = $1),
		EXISTS (SELECT 1 FROM student_sessions a JOIN student_sessions b ON a.device_id = b.device_id
			WHERE a.student_id = $1 AND b.student_id = $2 AND a.device_id <> '')`, in.OwnerID, st.ID).Scan(&ownerExists, &sameDevice); err != nil {
		return err
	}
	if !ownerExists {
		return httpx.NotFound("Student")
	}
	if sameDevice {
		return httpx.Conflict("same_device", "Invites must come from a different phone.")
	}
	tag, err := s.pool.Exec(c, `INSERT INTO challenge_referrals (challenge_id, owner_id, visitor_id, kind, unit)
		VALUES ($1, $2, $3, $4, $5) ON CONFLICT (visitor_id) DO NOTHING`, ch.ID, in.OwnerID, st.ID, in.Kind, in.Unit)
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		return httpx.Conflict("already_counted", "Your account has already counted for an invite.")
	}
	return httpx.NoContent(c)
}
