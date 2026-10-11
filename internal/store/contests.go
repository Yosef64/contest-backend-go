package store

import (
	"context"
	"encoding/json"
	"time"

	"github.com/jackc/pgx/v5"
)

// Questions -------------------------------------------------------------------

const questionCols = `id, text, options, answer_index, explanation, subject, chapter, topic, grade, stream, image_url, created_at, created_by, updated_at, updated_by`

// Questions lists the bank, filtered by subject, grade (or level), stream,
// text/chapter search and the date they were added ([from, until)). A stream
// filter also keeps questions meant for both streams.
func (s *Store) Questions(ctx context.Context, subject, grade, stream, q string, from, until *time.Time) ([]Question, error) {
	return collect[Question](ctx, s.db, `SELECT `+questionCols+` FROM questions
		WHERE ($1 = '' OR subject = $1) AND ($2 = '' OR grade = $2)
		AND ($3 = '' OR text ILIKE '%' || $3 || '%' OR chapter ILIKE '%' || $3 || '%' OR topic ILIKE '%' || $3 || '%')
		AND ($4::timestamptz IS NULL OR created_at >= $4) AND ($5::timestamptz IS NULL OR created_at < $5)
		AND ($6 = '' OR stream IN ('', $6))
		ORDER BY created_at DESC, id`, subject, grade, q, from, until, stream)
}

// CreateQuestion records the author in created_by and updated_by.
func (s *Store) CreateQuestion(ctx context.Context, q Question) (Question, error) {
	out, err := one[Question](ctx, s.db, `INSERT INTO questions (text, options, answer_index, explanation, subject, chapter, topic, grade, image_url, created_by, updated_by, stream)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $10, $11) RETURNING `+questionCols,
		q.Text, q.Options, q.AnswerIndex, q.Explanation, q.Subject, q.Chapter, q.Topic, q.Grade, q.ImageURL, q.CreatedBy, q.Stream)
	return out, classify(err)
}

// CreateQuestions inserts all questions atomically.
func (s *Store) CreateQuestions(ctx context.Context, qs []Question) (int, error) {
	err := pgx.BeginFunc(ctx, s.db, func(tx pgx.Tx) error {
		for _, q := range qs {
			if _, err := tx.Exec(ctx, `INSERT INTO questions (text, options, answer_index, explanation, subject, chapter, topic, grade, image_url, created_by, updated_by, stream)
				VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $10, $11)`, q.Text, q.Options, q.AnswerIndex, q.Explanation, q.Subject, q.Chapter, q.Topic, q.Grade, q.ImageURL, q.CreatedBy, q.Stream); err != nil {
				return err
			}
		}
		return nil
	})
	return len(qs), classify(err)
}

// UpdateQuestion records the editor in updated_by.
func (s *Store) UpdateQuestion(ctx context.Context, id string, q Question) (Question, error) {
	out, err := one[Question](ctx, s.db, `UPDATE questions SET text = $2, options = $3, answer_index = $4, explanation = $5,
		subject = $6, chapter = $7, grade = $8, image_url = $9, updated_by = $10, topic = $11, stream = $12, updated_at = now() WHERE id = $1 RETURNING `+questionCols,
		id, q.Text, q.Options, q.AnswerIndex, q.Explanation, q.Subject, q.Chapter, q.Grade, q.ImageURL, q.UpdatedBy, q.Topic, q.Stream)
	return out, classify(err)
}

// DeleteQuestion fails with ErrInUse if a contest uses the question.
func (s *Store) DeleteQuestion(ctx context.Context, id string) error {
	tag, err := s.db.Exec(ctx, `DELETE FROM questions WHERE id = $1`, id)
	if err != nil {
		return classify(err)
	}
	if tag.RowsAffected() == 0 {
		return ErrNotFound
	}
	return nil
}

// Contests --------------------------------------------------------------------

const contestSelect = `SELECT c.id, c.title, c.description, c.subject, c.grade, c.start_time, c.end_time, c.prize,
	c.kind, c.exam_year, c.stream, c.duration_minutes, c.rules, c.scoring, c.created_by, c.updated_at,
	coalesce((SELECT array_agg(question_id ORDER BY position) FROM contest_questions WHERE contest_id = c.id), '{}') AS question_ids,
	(SELECT count(*)::int FROM registrations WHERE contest_id = c.id) AS registrations,
	(SELECT count(*)::int FROM submissions WHERE contest_id = c.id) AS submissions,
	(SELECT count(*)::int FROM practice_attempts WHERE contest_id = c.id) AS attempts
	FROM contests c`

func withPhase(cs []Contest) []Contest {
	now := time.Now()
	for i := range cs {
		cs[i].Phase = cs[i].PhaseAt(now)
	}
	return cs
}

// Contests lists all contests and national exam sets; grade filters for the
// student app.
func (s *Store) Contests(ctx context.Context, grade string, departments ...string) ([]Contest, error) {
	department := ""
	if len(departments) > 0 {
		department = departments[0]
	}
	cs, err := collect[Contest](ctx, s.db, contestSelect+` WHERE ($1 = '' OR c.grade = $1 OR c.grade = $2)
		ORDER BY c.kind, c.exam_year DESC NULLS LAST, c.start_time DESC`, grade, department)
	return withPhase(cs), err
}

func (s *Store) Contest(ctx context.Context, id string) (Contest, error) {
	c, err := one[Contest](ctx, s.db, contestSelect+` WHERE c.id = $1`, id)
	c.Phase = c.PhaseAt(time.Now())
	return c, err
}

// ContestQuestions returns the contest's questions in order.
func (s *Store) ContestQuestions(ctx context.Context, contestID string) ([]Question, error) {
	return collect[Question](ctx, s.db, `SELECT q.id, q.text, q.options, q.answer_index, q.explanation, q.subject, q.chapter, q.topic, q.grade, q.stream, q.image_url,
		q.created_at, q.created_by, q.updated_at, q.updated_by
		FROM contest_questions cq JOIN questions q ON q.id = cq.question_id WHERE cq.contest_id = $1 ORDER BY cq.position`, contestID)
}

func (s *Store) saveContestQuestions(ctx context.Context, tx pgx.Tx, contestID string, ids []string) error {
	if _, err := tx.Exec(ctx, `DELETE FROM contest_questions WHERE contest_id = $1`, contestID); err != nil {
		return err
	}
	_, err := tx.Exec(ctx, `INSERT INTO contest_questions (contest_id, question_id, position)
		SELECT $1, id, n FROM unnest($2::text[]) WITH ORDINALITY AS t(id, n)`, contestID, ids)
	return classify(err)
}

func (s *Store) CreateContest(ctx context.Context, c Contest) (Contest, error) {
	var id string
	err := pgx.BeginFunc(ctx, s.db, func(tx pgx.Tx) error {
		if err := tx.QueryRow(ctx, `INSERT INTO contests (title, description, subject, grade, start_time, end_time, prize,
			kind, exam_year, stream, duration_minutes, rules, scoring, created_by)
			VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14) RETURNING id`,
			c.Title, c.Description, c.Subject, c.Grade, c.StartTime, c.EndTime, c.Prize,
			c.Kind, c.ExamYear, c.Stream, c.DurationMinutes, c.Rules, c.Scoring, c.CreatedBy).Scan(&id); err != nil {
			return err
		}
		return s.saveContestQuestions(ctx, tx, id, c.QuestionIDs)
	})
	if err != nil {
		return Contest{}, err
	}
	return s.Contest(ctx, id)
}

// UpdateContest replaces details and questions. Questions cannot change once
// anyone has submitted (results would no longer be comparable).
func (s *Store) UpdateContest(ctx context.Context, id string, c Contest) (Contest, error) {
	err := pgx.BeginFunc(ctx, s.db, func(tx pgx.Tx) error {
		tag, err := tx.Exec(ctx, `UPDATE contests SET title = $2, description = $3, subject = $4, grade = $5, start_time = $6, end_time = $7, prize = $8,
			kind = $9, exam_year = $10, stream = $11, duration_minutes = $12, rules = $13, scoring = $14, updated_at = now()
			WHERE id = $1`, id, c.Title, c.Description, c.Subject, c.Grade, c.StartTime, c.EndTime, c.Prize,
			c.Kind, c.ExamYear, c.Stream, c.DurationMinutes, c.Rules, c.Scoring)
		if err != nil {
			return err
		}
		if tag.RowsAffected() == 0 {
			return ErrNotFound
		}
		var current []string
		_ = tx.QueryRow(ctx, `SELECT coalesce(array_agg(question_id ORDER BY position), '{}') FROM contest_questions WHERE contest_id = $1`, id).Scan(&current)
		if equal(current, c.QuestionIDs) {
			return nil
		}
		if locked, err := questionsLocked(ctx, tx, id); err != nil || locked {
			if err == nil {
				err = ErrInUse
			}
			return err
		}
		return s.saveContestQuestions(ctx, tx, id, c.QuestionIDs)
	})
	if err != nil {
		return Contest{}, err
	}
	return s.Contest(ctx, id)
}

// questionsLocked: once a student has submitted, or started a timed national
// exam attempt, the question set must not change under them.
func questionsLocked(ctx context.Context, tx pgx.Tx, contestID string) (bool, error) {
	var locked bool
	err := tx.QueryRow(ctx, `SELECT EXISTS (SELECT 1 FROM submissions WHERE contest_id = $1)
		OR EXISTS (SELECT 1 FROM practice_attempts WHERE contest_id = $1)`, contestID).Scan(&locked)
	return locked, err
}

// ImportContestQuestions adds new questions to the bank and appends them to
// the contest's question list, all or nothing. Returns ErrInUse when the
// questions are locked and ErrNotFound for an unknown contest.
func (s *Store) ImportContestQuestions(ctx context.Context, contestID string, qs []Question, maxTotal int) (int, error) {
	added := 0
	err := pgx.BeginFunc(ctx, s.db, func(tx pgx.Tx) error {
		var exists bool
		if err := tx.QueryRow(ctx, `SELECT EXISTS (SELECT 1 FROM contests WHERE id = $1)`, contestID).Scan(&exists); err != nil {
			return err
		}
		if !exists {
			return ErrNotFound
		}
		if locked, err := questionsLocked(ctx, tx, contestID); err != nil || locked {
			if err == nil {
				err = ErrInUse
			}
			return err
		}
		var count int
		if err := tx.QueryRow(ctx, `SELECT count(*) FROM contest_questions WHERE contest_id = $1`, contestID).Scan(&count); err != nil {
			return err
		}
		if count+len(qs) > maxTotal {
			return ErrTooMany
		}
		for i, q := range qs {
			var id string
			if err := tx.QueryRow(ctx, `INSERT INTO questions (text, options, answer_index, explanation, subject, chapter, topic, grade, image_url, created_by, updated_by, stream)
				VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $10, $11) RETURNING id`,
				q.Text, q.Options, q.AnswerIndex, q.Explanation, q.Subject, q.Chapter, q.Topic, q.Grade, q.ImageURL, q.CreatedBy, q.Stream).Scan(&id); err != nil {
				return err
			}
			if _, err := tx.Exec(ctx, `INSERT INTO contest_questions (contest_id, question_id, position) VALUES ($1, $2, $3)`,
				contestID, id, count+i+1); err != nil {
				return err
			}
			added++
		}
		return nil
	})
	return added, classify(err)
}

func (s *Store) DeleteContest(ctx context.Context, id string) error {
	tag, err := s.db.Exec(ctx, `DELETE FROM contests WHERE id = $1`, id)
	if err == nil && tag.RowsAffected() == 0 {
		return ErrNotFound
	}
	return err
}

// Registrations & submissions -------------------------------------------------

func (s *Store) Register(ctx context.Context, contestID, studentID string, subjects []string, experience string) error {
	_, err := s.db.Exec(ctx, `INSERT INTO registrations (contest_id, student_id, subjects, experience)
		VALUES ($1, $2, $3, $4) ON CONFLICT DO NOTHING`, contestID, studentID, subjects, experience)
	return classify(err)
}

// StudentContestState tells the app whether the student registered and/or
// submitted each contest.
func (s *Store) StudentContestState(ctx context.Context, studentID string) (registered map[string]bool, submitted map[string]Submission, err error) {
	registered = map[string]bool{}
	submitted = map[string]Submission{}
	rows, err := s.db.Query(ctx, `SELECT contest_id FROM registrations WHERE student_id = $1`, studentID)
	if err != nil {
		return
	}
	ids, err := pgx.CollectRows(rows, pgx.RowTo[string])
	if err != nil {
		return
	}
	for _, id := range ids {
		registered[id] = true
	}
	subs, err := s.StudentSubmissions(ctx, studentID)
	for _, sub := range subs {
		submitted[sub.ContestID] = sub
	}
	return
}

func (s *Store) StudentSubmissions(ctx context.Context, studentID string) ([]Submission, error) {
	rows, err := s.db.Query(ctx, `SELECT contest_id, student_id, answers, correct, total, time_seconds, submitted_at
		FROM submissions WHERE student_id = $1 ORDER BY submitted_at DESC`, studentID)
	if err != nil {
		return nil, err
	}
	return pgx.CollectRows(rows, scanSubmission)
}

func (s *Store) Submission(ctx context.Context, contestID, studentID string) (Submission, error) {
	rows, err := s.db.Query(ctx, `SELECT contest_id, student_id, answers, correct, total, time_seconds, submitted_at
		FROM submissions WHERE contest_id = $1 AND student_id = $2`, contestID, studentID)
	if err != nil {
		return Submission{}, err
	}
	return pgx.CollectExactlyOneRow(rows, scanSubmission)
}

func scanSubmission(row pgx.CollectableRow) (Submission, error) {
	var sub Submission
	var answers []byte
	err := row.Scan(&sub.ContestID, &sub.StudentID, &answers, &sub.Correct, &sub.Total, &sub.TimeSeconds, &sub.SubmittedAt)
	if err == nil {
		err = json.Unmarshal(answers, &sub.Answers)
	}
	return sub, err
}

// CreateSubmission stores a server-scored submission; a second submission
// for the same contest returns ErrDuplicate.
func (s *Store) CreateSubmission(ctx context.Context, sub Submission) error {
	answers, err := json.Marshal(sub.Answers)
	if err != nil {
		return err
	}
	_, err = s.db.Exec(ctx, `INSERT INTO submissions (contest_id, student_id, answers, correct, total, time_seconds)
		VALUES ($1, $2, $3, $4, $5, $6)`, sub.ContestID, sub.StudentID, answers, sub.Correct, sub.Total, sub.TimeSeconds)
	return classify(err)
}

// Standings ranks a contest by correct answers, then time.
func (s *Store) Standings(ctx context.Context, contestID string) ([]StandingRow, error) {
	return collect[StandingRow](ctx, s.db, `SELECT
		rank() OVER (ORDER BY sub.correct DESC, sub.time_seconds ASC)::int AS rank,
		st.id AS student_id, st.name, st.photo_url, sub.correct, sub.total,
		CASE WHEN sub.total = 0 THEN 0 ELSE round(100.0 * sub.correct / sub.total)::int END AS score,
		sub.time_seconds
		FROM submissions sub JOIN students st ON st.id = sub.student_id
		WHERE sub.contest_id = $1 ORDER BY rank, st.name`, contestID)
}

// Leaderboard is the all-time ranking across finished contests.
func (s *Store) Leaderboard(ctx context.Context, limit int) ([]StandingRow, error) {
	return collect[StandingRow](ctx, s.db, `WITH totals AS (
			SELECT sub.student_id, sum(sub.correct)::int AS correct, sum(sub.total)::int AS total, sum(sub.time_seconds)::int AS time_seconds
			FROM submissions sub JOIN contests c ON c.id = sub.contest_id
			WHERE c.end_time < now() AND c.kind = 'contest' GROUP BY sub.student_id
		)
		SELECT rank() OVER (ORDER BY t.correct DESC, t.time_seconds ASC)::int AS rank,
			st.id AS student_id, st.name, st.photo_url, t.correct, t.total,
			CASE WHEN t.total = 0 THEN 0 ELSE round(100.0 * t.correct / t.total)::int END AS score, t.time_seconds
		FROM totals t JOIN students st ON st.id = t.student_id
		ORDER BY rank, st.name LIMIT $1`, limit)
}

func equal(a, b []string) bool {
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
