package store

import (
	"context"
	"fmt"
	"time"

	"golang.org/x/sync/errgroup"
)

type Totals struct {
	Students          int `json:"students"`
	NewStudents30d    int `json:"new_students_30d"`
	ActiveContests    int `json:"active_contests"`
	UpcomingContests  int `json:"upcoming_contests"`
	Courses           int `json:"courses"`
	ChaptersPublished int `json:"chapters_published"`
	ChaptersDraft     int `json:"chapters_draft"`
	Registrations30d  int `json:"registrations_30d"`
}

// Earnings is the money taken from plan payments, in ETB.
type Earnings struct {
	Currency string `json:"currency"`
	// Total is Online plus Manual.
	Total float64 `json:"total"`
	// Online: Chapa payments, the amounts actually paid.
	Online      float64 `json:"online"`
	OnlineCount int     `json:"online_count"`
	// Manual: bank transfers approved by an admin. Their amount isn't
	// recorded, so each is valued at the plan's current price.
	Manual      float64 `json:"manual"`
	ManualCount int     `json:"manual_count"`
	ThisMonth   float64 `json:"this_month"`
	LastMonth   float64 `json:"last_month"`
}

// earningsQuery adds up online payments and approved bank transfers (the
// approvals Chapa makes itself are skipped: they are counted as online).
const earningsQuery = `WITH price AS (
		SELECT
			coalesce((SELECT CASE WHEN value ~ '^[0-9]+(\.[0-9]+)?$' THEN value::numeric END FROM site_settings WHERE key = 'price_academic'), 0) AS academic,
			coalesce((SELECT CASE WHEN value ~ '^[0-9]+(\.[0-9]+)?$' THEN value::numeric END FROM site_settings WHERE key = 'price_training'), 0) AS training
	), online AS (
		SELECT coalesce(verified_at, created_at) AS at, amount FROM chapa_payments WHERE status = 'success'
	), manual AS (
		SELECT e.created_at AS at, CASE e.module WHEN 'academic' THEN p.academic ELSE p.training END AS amount
		FROM enrollment_requests e CROSS JOIN price p
		WHERE e.status = 'approved' AND e.reference NOT LIKE 'Chapa %'
	), paid AS (
		SELECT at, amount FROM online UNION ALL SELECT at, amount FROM manual
	)
	SELECT
		coalesce((SELECT sum(amount) FROM online), 0)::float8,
		(SELECT count(*) FROM online)::int,
		coalesce((SELECT sum(amount) FROM manual), 0)::float8,
		(SELECT count(*) FROM manual)::int,
		coalesce((SELECT sum(amount) FROM paid WHERE at >= date_trunc('month', now())), 0)::float8,
		coalesce((SELECT sum(amount) FROM paid WHERE at >= date_trunc('month', now()) - interval '1 month' AND at < date_trunc('month', now())), 0)::float8`

type MonthCount struct {
	Month string `json:"month" db:"month"`
	Count int    `json:"count" db:"count"`
}

type GradeCount struct {
	Grade string `json:"grade" db:"grade"`
	Count int    `json:"count" db:"count"`
}

type Overview struct {
	Totals               Totals       `json:"totals"`
	SignupsByMonth       []MonthCount `json:"signups_by_month"`
	ParticipationByMonth []MonthCount `json:"participation_by_month"`
	StudentsByGrade      []GradeCount `json:"students_by_grade"`
	NextContest          *Contest     `json:"next_contest"`
	RecentContests       []Contest    `json:"recent_contests"`
	RecentStudents       []Student    `json:"recent_students"`
	Earnings             Earnings     `json:"earnings"`
}

// monthly counts for the last six months, including empty months.
const monthly = `SELECT to_char(m, 'Mon') AS month, count(t.ts)::int AS count
	FROM generate_series(date_trunc('month', now()) - interval '5 months', date_trunc('month', now()), interval '1 month') m
	LEFT JOIN (%s) t(ts) ON date_trunc('month', t.ts) = m
	GROUP BY m ORDER BY m`

// Overview gathers dashboard numbers. The independent queries run in
// parallel so latency is one round trip, not eight.
func (s *Store) Overview(ctx context.Context) (Overview, error) {
	var o Overview
	g, ctx := errgroup.WithContext(ctx)
	g.Go(func() error {
		t := &o.Totals
		return s.db.QueryRow(ctx, `SELECT
			(SELECT count(*) FROM students),
			(SELECT count(*) FROM students WHERE created_at > now() - interval '30 days'),
			(SELECT count(*) FROM contests WHERE kind = 'contest' AND start_time <= now() AND end_time > now()),
			(SELECT count(*) FROM contests WHERE kind = 'contest' AND start_time > now()),
			(SELECT count(*) FROM courses),
			(SELECT count(*) FROM chapters WHERE status = 'published'),
			(SELECT count(*) FROM chapters WHERE status = 'draft'),
			(SELECT count(*) FROM registrations WHERE registered_at > now() - interval '30 days')`).Scan(
			&t.Students, &t.NewStudents30d, &t.ActiveContests, &t.UpcomingContests, &t.Courses, &t.ChaptersPublished, &t.ChaptersDraft, &t.Registrations30d)
	})
	g.Go(func() (err error) {
		o.SignupsByMonth, err = collect[MonthCount](ctx, s.db, fmt.Sprintf(monthly, `SELECT created_at FROM students`))
		return
	})
	g.Go(func() (err error) {
		o.ParticipationByMonth, err = collect[MonthCount](ctx, s.db, fmt.Sprintf(monthly, `SELECT submitted_at FROM submissions`))
		return
	})
	g.Go(func() (err error) {
		o.StudentsByGrade, err = collect[GradeCount](ctx, s.db, `SELECT g AS grade, count(s.id)::int AS count
			FROM unnest(ARRAY['9','10','11','12']) g LEFT JOIN students s ON s.grade = g GROUP BY g ORDER BY g::int`)
		return
	})
	g.Go(func() error {
		next, err := collect[Contest](ctx, s.db, contestSelect+` WHERE c.kind = 'contest' AND c.end_time > now() ORDER BY c.start_time LIMIT 1`)
		if err == nil && len(next) == 1 {
			withPhase(next)
			o.NextContest = &next[0]
		}
		return err
	})
	g.Go(func() error {
		recent, err := collect[Contest](ctx, s.db, contestSelect+` WHERE c.kind = 'contest' ORDER BY abs(extract(epoch FROM c.start_time - now())) LIMIT 5`)
		o.RecentContests = withPhase(recent)
		return err
	})
	g.Go(func() (err error) {
		o.RecentStudents, err = collect[Student](ctx, s.db, `SELECT `+studentCols+` FROM students ORDER BY created_at DESC LIMIT 6`)
		return
	})
	g.Go(func() error {
		e := &o.Earnings
		e.Currency = "ETB"
		err := s.db.QueryRow(ctx, earningsQuery).Scan(&e.Online, &e.OnlineCount, &e.Manual, &e.ManualCount, &e.ThisMonth, &e.LastMonth)
		e.Total = e.Online + e.Manual
		return err
	})
	return o, g.Wait()
}

type SubjectStat struct {
	Total   int `json:"total"`
	Correct int `json:"correct"`
}

type StudentStats struct {
	Contests          int                    `json:"contests"`
	Questions         int                    `json:"questions"`
	Correct           int                    `json:"correct"`
	Accuracy          float64                `json:"accuracy"`
	AverageSeconds    int                    `json:"average_seconds"`
	ChaptersCompleted int                    `json:"chapters_completed"`
	GlobalRank        int                    `json:"global_rank"`
	Subjects          map[string]SubjectStat `json:"subjects"`
	Chapters          map[string]SubjectStat `json:"chapters"`
	Topics            map[string]SubjectStat `json:"topics"`
	Trend             []TrendPoint           `json:"trend"`
}

type TrendPoint struct {
	Month     string  `json:"month" db:"month"`
	Accuracy  float64 `json:"accuracy" db:"accuracy"`
	Questions int     `json:"questions" db:"questions"`
}

// StudentStats is computed from the student's submissions.
func (s *Store) StudentStats(ctx context.Context, studentID string) (StudentStats, error) {
	st := StudentStats{Subjects: map[string]SubjectStat{}, Chapters: map[string]SubjectStat{}, Topics: map[string]SubjectStat{}, Trend: []TrendPoint{}}
	err := s.db.QueryRow(ctx, `SELECT count(*)::int, coalesce(sum(total), 0)::int, coalesce(sum(correct), 0)::int,
		CASE WHEN coalesce(sum(total), 0) = 0 THEN 0 ELSE (sum(time_seconds) / sum(total))::int END,
		(SELECT count(*)::int FROM chapter_progress WHERE student_id = $1)
		FROM submissions WHERE student_id = $1`, studentID).Scan(&st.Contests, &st.Questions, &st.Correct, &st.AverageSeconds, &st.ChaptersCompleted)
	if err != nil {
		return st, err
	}
	if st.Questions > 0 {
		st.Accuracy = float64(st.Correct) / float64(st.Questions)
	}

	// Per subject/chapter/topic: expand each submission's answers against the questions.
	rows, err := s.db.Query(ctx, `SELECT q.subject, q.chapter, coalesce(nullif(trim(q.topic), ''), 'General'), (sub.answers ->> q.id)::int = q.answer_index AS ok
		FROM submissions sub
		JOIN contest_questions cq ON cq.contest_id = sub.contest_id
		JOIN questions q ON q.id = cq.question_id
		WHERE sub.student_id = $1`, studentID)
	if err != nil {
		return st, err
	}
	defer rows.Close()
	for rows.Next() {
		var subject, chapter, topic string
		var ok *bool
		if err := rows.Scan(&subject, &chapter, &topic, &ok); err != nil {
			return st, err
		}
		inc := func(m map[string]SubjectStat, k string) {
			v := m[k]
			v.Total++
			if ok != nil && *ok {
				v.Correct++
			}
			m[k] = v
		}
		inc(st.Subjects, subject)
		inc(st.Chapters, chapter)
		inc(st.Topics, subject+" / "+chapter+" / "+topic)
	}
	if err := rows.Err(); err != nil {
		return st, err
	}

	st.Trend, err = collect[TrendPoint](ctx, s.db, `SELECT to_char(date_trunc('month', submitted_at), 'Mon') AS month,
		CASE WHEN sum(total) = 0 THEN 0 ELSE sum(correct)::float / sum(total) END AS accuracy, sum(total)::int AS questions
		FROM submissions WHERE student_id = $1 AND submitted_at > now() - interval '6 months'
		GROUP BY date_trunc('month', submitted_at) ORDER BY date_trunc('month', submitted_at)`, studentID)
	if err != nil {
		return st, err
	}
	_ = s.db.QueryRow(ctx, `SELECT rank FROM (`+leaderboardRanks+`) r WHERE student_id = $1`, studentID).Scan(&st.GlobalRank)
	return st, nil
}

const leaderboardRanks = `SELECT sub.student_id, rank() OVER (ORDER BY sum(sub.correct) DESC, sum(sub.time_seconds) ASC)::int AS rank
	FROM submissions sub JOIN contests c ON c.id = sub.contest_id WHERE c.end_time < now() GROUP BY sub.student_id`

// AdminStudentDetail backs the admin student profile.
type AdminStudentDetail struct {
	Student Student `json:"student"`
	Stats   struct {
		Contests          int     `json:"contests"`
		Questions         int     `json:"questions"`
		Correct           int     `json:"correct"`
		Accuracy          float64 `json:"accuracy"`
		ChaptersCompleted int     `json:"chapters_completed"`
	} `json:"stats"`
	Submissions []SubmissionRow `json:"submissions"`
}

type SubmissionRow struct {
	ContestID    string    `json:"contest_id" db:"contest_id"`
	ContestTitle string    `json:"contest_title" db:"contest_title"`
	Correct      int       `json:"correct" db:"correct"`
	Total        int       `json:"total" db:"total"`
	SubmittedAt  time.Time `json:"submitted_at" db:"submitted_at"`
}

func (s *Store) AdminStudentDetail(ctx context.Context, id string) (AdminStudentDetail, error) {
	var d AdminStudentDetail
	var err error
	if d.Student, err = s.StudentByID(ctx, id); err != nil {
		return d, err
	}
	stats, err := s.StudentStats(ctx, id)
	if err != nil {
		return d, err
	}
	d.Stats.Contests, d.Stats.Questions, d.Stats.Correct, d.Stats.Accuracy, d.Stats.ChaptersCompleted =
		stats.Contests, stats.Questions, stats.Correct, stats.Accuracy, stats.ChaptersCompleted
	d.Submissions, err = collect[SubmissionRow](ctx, s.db, `SELECT sub.contest_id, c.title AS contest_title, sub.correct, sub.total, sub.submitted_at
		FROM submissions sub JOIN contests c ON c.id = sub.contest_id WHERE sub.student_id = $1 ORDER BY sub.submitted_at DESC`, id)
	return d, err
}
