package store

import (
	"context"
)

// ContestParticipants returns up to `limit` registered student names per
// contest, newest registrations first, for the avatar stacks in the app.
func (s *Store) ContestParticipants(ctx context.Context, limit int) (map[string][]string, error) {
	rows, err := s.db.Query(ctx, `SELECT contest_id, name FROM (
			SELECT r.contest_id, st.name, row_number() OVER (PARTITION BY r.contest_id ORDER BY r.registered_at DESC) AS n
			FROM registrations r JOIN students st ON st.id = r.student_id
		) t WHERE n <= $1`, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := map[string][]string{}
	for rows.Next() {
		var contestID, name string
		if err := rows.Scan(&contestID, &name); err != nil {
			return nil, err
		}
		out[contestID] = append(out[contestID], name)
	}
	return out, rows.Err()
}

// StudentRank returns the student's row on the all-time leaderboard, if any.
func (s *Store) StudentRank(ctx context.Context, studentID string) (*StandingRow, error) {
	rows, err := s.Leaderboard(ctx, 100000)
	if err != nil {
		return nil, err
	}
	for i := range rows {
		if rows[i].StudentID == studentID {
			return &rows[i], nil
		}
	}
	return nil, nil
}
