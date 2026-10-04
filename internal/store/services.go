package store

import (
	"context"
	"encoding/json"
)

// ServiceRows returns JSON records without exposing password hashes or auth subjects.
func (s *Store) ServiceRows(ctx context.Context, query string, args ...any) ([]map[string]any, error) {
	rows, err := s.db.Query(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []map[string]any{}
	for rows.Next() {
		var raw []byte
		if err := rows.Scan(&raw); err != nil {
			return nil, err
		}
		var item map[string]any
		if err := json.Unmarshal(raw, &item); err != nil {
			return nil, err
		}
		out = append(out, item)
	}
	return out, rows.Err()
}
func (s *Store) ServiceExec(ctx context.Context, query string, args ...any) error {
	tag, err := s.db.Exec(ctx, query, args...)
	if err != nil {
		return classify(err)
	}
	if tag.RowsAffected() == 0 {
		return ErrNotFound
	}
	return nil
}

// PrivateStandings ranks every submission of a contest, then hides other
// students' rows for school contests (each student sees only their own row,
// but with their true rank). Ranking must happen before that filter.
func (s *Store) PrivateStandings(ctx context.Context, id, studentID string) ([]StandingRow, error) {
	return collect[StandingRow](ctx, s.db, `WITH ranked AS (
		SELECT rank() OVER (ORDER BY sub.correct DESC, sub.time_seconds ASC)::int AS rank,
			st.id AS student_id, st.name, sub.correct, sub.total,
			CASE WHEN sub.total = 0 THEN 0 ELSE round(sub.correct::numeric / sub.total * 100)::int END AS score,
			sub.time_seconds
		FROM submissions sub JOIN students st ON st.id = sub.student_id
		WHERE sub.contest_id = $1)
	SELECT * FROM ranked
	WHERE NOT EXISTS (SELECT 1 FROM learning_policies WHERE resource_type = 'contest' AND resource_id = $1 AND school_id IS NOT NULL)
		OR student_id = $2
	ORDER BY rank, name`, id, studentID)
}
// PrivateLeaderboard ranks finished, non-school contests within one level
// ("9", "10", "11" for high school by grade, or "entrance", "remedial",
// "freshman" by department) and stream ("" for every stream). It returns the
// top 50 plus the caller's own row.
func (s *Store) PrivateLeaderboard(ctx context.Context, studentID, level, stream string) ([]StandingRow, error) {
	return collect[StandingRow](ctx, s.db, `WITH t AS (
			SELECT sub.student_id, sum(sub.correct)::int AS correct, sum(sub.total)::int AS total, sum(sub.time_seconds)::int AS time_seconds
			FROM submissions sub JOIN contests co ON co.id = sub.contest_id
			WHERE co.end_time < now() AND co.kind = 'contest'
				AND NOT EXISTS (SELECT 1 FROM learning_policies WHERE resource_type = 'contest' AND resource_id = co.id AND school_id IS NOT NULL)
			GROUP BY sub.student_id),
		ranked AS (
			SELECT rank() OVER (ORDER BY t.correct DESC, t.time_seconds ASC)::int AS rank,
				st.id AS student_id, st.name, st.photo_url, t.correct, t.total,
				CASE WHEN t.total = 0 THEN 0 ELSE round(t.correct::numeric / t.total * 100)::int END AS score, t.time_seconds
			FROM t JOIN students st ON st.id = t.student_id
			LEFT JOIN academic_profiles p ON p.student_id = st.id
			WHERE CASE WHEN $2 IN ('9', '10', '11') THEN coalesce(p.department, '') = '' AND st.grade = $2
				ELSE p.department = $2 END
				AND ($3 = '' OR p.stream = $3))
		SELECT * FROM ranked WHERE rank <= 50 OR student_id = $1 ORDER BY rank, name`, studentID, level, stream)
}
