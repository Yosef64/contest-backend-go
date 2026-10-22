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
	Growth               Growth       `json:"growth"`
}

// GrowthPoint is one day or month: students who joined, by stream (grades
// 9-10 have none), and the money taken.
type GrowthPoint struct {
	Period   string  `json:"period" db:"period"`
	Natural  int     `json:"natural" db:"natural"`
	Social   int     `json:"social" db:"social"`
	NoStream int     `json:"no_stream" db:"no_stream"`
	Revenue  float64 `json:"revenue" db:"revenue"`
}

// Growth: the last 30 days and the last 12 months, oldest first.
type Growth struct {
	Daily   []GrowthPoint `json:"daily"`
	Monthly []GrowthPoint `json:"monthly"`
}

// growthQuery fills %[1]s (the step: day or month) over the series from
// %[2]s. Times are bucketed in Addis Ababa time.
const growthQuery = `WITH price AS (
		SELECT
			coalesce((SELECT CASE WHEN value ~ '^[0-9]+(\.[0-9]+)?$' THEN value::numeric END FROM site_settings WHERE key = 'price_academic'), 0) AS academic,
			coalesce((SELECT CASE WHEN value ~ '^[0-9]+(\.[0-9]+)?$' THEN value::numeric END FROM site_settings WHERE key = 'price_training'), 0) AS training
	), paid AS (
		SELECT coalesce(verified_at, created_at) AS at, amount FROM chapa_payments WHERE status = 'success'
		UNION ALL
		SELECT e.created_at, CASE e.module WHEN 'academic' THEN p.academic ELSE p.training END
		FROM enrollment_requests e CROSS JOIN price p
		WHERE e.status = 'approved' AND e.reference NOT LIKE 'Chapa %%'
	), series AS (
		SELECT generate_series(%[2]s, date_trunc('%[1]s', now() AT TIME ZONE 'Africa/Addis_Ababa'), interval '1 %[1]s') AS p
	)
	SELECT to_char(series.p, '%[3]s') AS period,
		count(st.id) FILTER (WHERE ap.stream = 'natural')::int AS natural,
		count(st.id) FILTER (WHERE ap.stream = 'social')::int AS social,
		count(st.id) FILTER (WHERE coalesce(ap.stream, '') = '')::int AS no_stream,
		coalesce((SELECT sum(amount) FROM paid WHERE date_trunc('%[1]s', paid.at AT TIME ZONE 'Africa/Addis_Ababa') = series.p), 0)::float8 AS revenue
	FROM series
	LEFT JOIN students st ON date_trunc('%[1]s', st.created_at AT TIME ZONE 'Africa/Addis_Ababa') = series.p
	LEFT JOIN academic_profiles ap ON ap.student_id = st.id
	GROUP BY series.p ORDER BY series.p`

// monthly counts for the last six months, including empty months.
const monthly = `SELECT to_char(m, 'Mon') AS month, count(t.ts)::int AS count
	FROM generate_series(date_trunc('month', now()) - interval '5 months', date_trunc('month', now()), interval '1 month') m
	LEFT JOIN (%s) t(ts) ON date_trunc('month', t.ts) = m
	GROUP BY m ORDER BY m`

// Overview gathers dashboard numbers. The independent queries run in
// parallel so latency is one round trip, not eight. Student and exam
// figures are for "paid" or "free" students (anything else: everyone):
// students by their plan now, exam entries by the plan when they were made.
func (s *Store) Overview(ctx context.Context, plan string) (Overview, error) {
	var o Overview
	students := "TRUE"
	switch plan {
	case "paid":
		students = PaidNow("s.id")
	case "free":
		students = "NOT " + PaidNow("s.id")
	}
	g, ctx := errgroup.WithContext(ctx)
	g.Go(func() error {
		t := &o.Totals
		return s.db.QueryRow(ctx, `SELECT
			(SELECT count(*) FROM students s WHERE `+students+`),
			(SELECT count(*) FROM students s WHERE s.created_at > now() - interval '30 days' AND `+students+`),
			(SELECT count(*) FROM contests WHERE kind = 'contest' AND start_time <= now() AND end_time > now()),
			(SELECT count(*) FROM contests WHERE kind = 'contest' AND start_time > now()),
			(SELECT count(*) FROM courses),
			(SELECT count(*) FROM chapters WHERE status = 'published'),
			(SELECT count(*) FROM chapters WHERE status = 'draft'),
			(SELECT count(*) FROM registrations r JOIN students s ON s.id = r.student_id WHERE r.registered_at > now() - interval '30 days' AND `+students+`)`).Scan(
			&t.Students, &t.NewStudents30d, &t.ActiveContests, &t.UpcomingContests, &t.Courses, &t.ChaptersPublished, &t.ChaptersDraft, &t.Registrations30d)
	})
	g.Go(func() (err error) {
		o.SignupsByMonth, err = collect[MonthCount](ctx, s.db, fmt.Sprintf(monthly, `SELECT s.created_at FROM students s WHERE `+students))
		return
	})
	g.Go(func() (err error) {
		o.ParticipationByMonth, err = collect[MonthCount](ctx, s.db, fmt.Sprintf(monthly, `SELECT sub.submitted_at FROM submissions sub WHERE TRUE`+planFilter("sub", plan)))
		return
	})
	g.Go(func() (err error) {
		o.StudentsByGrade, err = collect[GradeCount](ctx, s.db, `SELECT g AS grade, count(s.id)::int AS count
			FROM unnest(ARRAY['9','10','11','12']) g LEFT JOIN students s ON s.grade = g AND `+students+` GROUP BY g ORDER BY g::int`)
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
		o.RecentStudents, err = collect[Student](ctx, s.db, `SELECT `+studentCols+` FROM students s WHERE `+students+` ORDER BY created_at DESC LIMIT 6`)
		return
	})
	g.Go(func() error {
		e := &o.Earnings
		e.Currency = "ETB"
		err := s.db.QueryRow(ctx, earningsQuery).Scan(&e.Online, &e.OnlineCount, &e.Manual, &e.ManualCount, &e.ThisMonth, &e.LastMonth)
		e.Total = e.Online + e.Manual
		return err
	})
	g.Go(func() (err error) {
		o.Growth.Daily, err = collect[GrowthPoint](ctx, s.db, fmt.Sprintf(growthQuery, "day",
			`date_trunc('day', now() AT TIME ZONE 'Africa/Addis_Ababa') - interval '29 days'`, "YYYY-MM-DD"))
		return
	})
	g.Go(func() (err error) {
		o.Growth.Monthly, err = collect[GrowthPoint](ctx, s.db, fmt.Sprintf(growthQuery, "month",
			`date_trunc('month', now() AT TIME ZONE 'Africa/Addis_Ababa') - interval '11 months'`, "YYYY-MM"))
		return
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
	// Questions left unanswered.
	Skipped int `json:"skipped"`
	// Chapters by subject (Chapters mixes subjects; kept for older apps).
	ChaptersBySubject map[string]map[string]SubjectStat `json:"chapters_by_subject"`
	Learning          Learning                          `json:"learning"`
}

// Learning is what the student has read and finished: courses of their
// level, chapters and topics, and trainings.
type Learning struct {
	CoursesDone    int               `json:"courses_done"`
	CoursesTotal   int               `json:"courses_total"`
	ChaptersDone   int               `json:"chapters_done"`
	ChaptersTotal  int               `json:"chapters_total"`
	TopicsDone     int               `json:"topics_done"`
	TrainingsDone  int               `json:"trainings_done"`
	TrainingsTotal int               `json:"trainings_total"`
	Subjects       []SubjectLearning `json:"subjects"`
}

type SubjectLearning struct {
	Subject       string `json:"subject"`
	ChaptersDone  int    `json:"chapters_done"`
	ChaptersTotal int    `json:"chapters_total"`
}

type TrendPoint struct {
	Month     string  `json:"month" db:"month"`
	Accuracy  float64 `json:"accuracy" db:"accuracy"`
	Questions int     `json:"questions" db:"questions"`
}

// StudentStats is computed from the student's submissions.
func (s *Store) StudentStats(ctx context.Context, studentID string) (StudentStats, error) {
	st := StudentStats{Subjects: map[string]SubjectStat{}, Chapters: map[string]SubjectStat{}, Topics: map[string]SubjectStat{},
		Trend: []TrendPoint{}, ChaptersBySubject: map[string]map[string]SubjectStat{}, Learning: Learning{Subjects: []SubjectLearning{}}}
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
		if st.ChaptersBySubject[subject] == nil {
			st.ChaptersBySubject[subject] = map[string]SubjectStat{}
		}
		inc(st.ChaptersBySubject[subject], chapter)
		if ok == nil {
			st.Skipped++
		}
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
	if st.Learning, err = s.studentLearning(ctx, studentID); err != nil {
		return st, err
	}
	// Ranked on the board of the student's current plan (paid results for a
	// student on a plan, free results otherwise).
	_ = s.db.QueryRow(ctx, `SELECT rank FROM (SELECT sub.student_id, rank() OVER (ORDER BY sum(sub.correct) DESC, sum(sub.time_seconds) ASC)::int AS rank
		FROM submissions sub JOIN contests c ON c.id = sub.contest_id
		WHERE c.end_time < now() AND `+PaidAt("sub.student_id", "sub.submitted_at")+` = `+PaidNow("$1")+`
		GROUP BY sub.student_id) r WHERE student_id = $1`, studentID).Scan(&st.GlobalRank)
	return st, nil
}

// AdminStudentDetail backs the admin student profile.
type AdminStudentDetail struct {
	Student Student `json:"student"`
	// The same statistics the student sees in the app.
	Stats       StudentStats    `json:"stats"`
	Submissions []SubmissionRow `json:"submissions"`
}

type SubmissionRow struct {
	ContestID    string    `json:"contest_id" db:"contest_id"`
	ContestTitle string    `json:"contest_title" db:"contest_title"`
	Correct      int       `json:"correct" db:"correct"`
	Total        int       `json:"total" db:"total"`
	TimeSeconds  int       `json:"time_seconds" db:"time_seconds"`
	Subject      string    `json:"subject" db:"subject"`
	SubmittedAt  time.Time `json:"submitted_at" db:"submitted_at"`
}

func (s *Store) AdminStudentDetail(ctx context.Context, id string) (AdminStudentDetail, error) {
	var d AdminStudentDetail
	var err error
	if d.Student, err = s.StudentByID(ctx, id); err != nil {
		return d, err
	}
	if d.Stats, err = s.StudentStats(ctx, id); err != nil {
		return d, err
	}
	d.Submissions, err = collect[SubmissionRow](ctx, s.db, `SELECT sub.contest_id, c.title AS contest_title, sub.correct, sub.total, sub.time_seconds, c.subject, sub.submitted_at
		FROM submissions sub JOIN contests c ON c.id = sub.contest_id WHERE sub.student_id = $1 ORDER BY sub.submitted_at DESC`, id)
	return d, err
}

// studentLearning counts what the student finished among the published
// courses of their level, and the trainings.
func (s *Store) studentLearning(ctx context.Context, studentID string) (Learning, error) {
	l := Learning{Subjects: []SubjectLearning{}}
	rows, err := s.db.Query(ctx, `SELECT c.subject, count(ch.id)::int, count(p.chapter_id)::int
		FROM courses c
		JOIN chapters ch ON ch.course_id = c.id AND ch.status = 'published'
		LEFT JOIN chapter_progress p ON p.chapter_id = ch.id AND p.student_id = $1
		WHERE c.grade = (SELECT grade FROM students WHERE id = $1)
		GROUP BY c.id, c.subject ORDER BY c.subject`, studentID)
	if err != nil {
		return l, err
	}
	bySubject := map[string]int{} // subject -> index in l.Subjects
	for rows.Next() {
		var subject string
		var total, done int
		if err := rows.Scan(&subject, &total, &done); err != nil {
			rows.Close()
			return l, err
		}
		l.CoursesTotal++
		if done == total {
			l.CoursesDone++
		}
		l.ChaptersTotal += total
		l.ChaptersDone += done
		i, seen := bySubject[subject]
		if !seen {
			i = len(l.Subjects)
			bySubject[subject] = i
			l.Subjects = append(l.Subjects, SubjectLearning{Subject: subject})
		}
		l.Subjects[i].ChaptersTotal += total
		l.Subjects[i].ChaptersDone += done
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return l, err
	}
	err = s.db.QueryRow(ctx, `SELECT
			coalesce((SELECT sum(cardinality(topics)) FROM topic_progress WHERE student_id = $1), 0)::int,
			(SELECT count(*) FROM trainings WHERE status = 'published')::int,
			(SELECT count(*) FROM training_progress tp JOIN trainings t ON t.id = tp.training_id AND t.status = 'published'
				WHERE tp.student_id = $1)::int`, studentID).Scan(&l.TopicsDone, &l.TrainingsTotal, &l.TrainingsDone)
	return l, err
}
