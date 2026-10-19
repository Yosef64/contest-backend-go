package store

import (
	"context"
	"strconv"
	"time"

	"github.com/jackc/pgx/v5"
)

const courseSelect = `SELECT c.id, c.grade, c.subject, c.title, c.description, c.updated_at,
	count(ch.id)::int AS chapter_count,
	count(ch.id) FILTER (WHERE ch.status = 'published')::int AS published_count
	FROM courses c LEFT JOIN chapters ch ON ch.course_id = c.id`

// Courses for a grade. publishedOnly hides courses with no published chapters
// (student view).
func (s *Store) Courses(ctx context.Context, grade string, publishedOnly bool) ([]Course, error) {
	having := ""
	if publishedOnly {
		having = ` HAVING count(ch.id) FILTER (WHERE ch.status = 'published') > 0`
	}
	return collect[Course](ctx, s.db, courseSelect+` WHERE ($1 = '' OR c.grade = $1) GROUP BY c.id`+having+` ORDER BY `+levelOrder("c.grade")+`, c.position, c.subject`, grade)
}

func (s *Store) Course(ctx context.Context, id string) (Course, error) {
	return one[Course](ctx, s.db, courseSelect+` WHERE c.id = $1 GROUP BY c.id`, id)
}

func (s *Store) CreateCourse(ctx context.Context, c Course) (Course, error) {
	var id string
	err := s.db.QueryRow(ctx, `INSERT INTO courses (grade, subject, title, description, position)
		VALUES ($1, $2, $3, $4, (SELECT coalesce(max(position), 0) + 1 FROM courses WHERE grade = $1)) RETURNING id`,
		c.Grade, c.Subject, c.Title, c.Description).Scan(&id)
	if err != nil {
		return Course{}, classify(err)
	}
	return s.Course(ctx, id)
}

func (s *Store) UpdateCourse(ctx context.Context, id string, c Course) (Course, error) {
	tag, err := s.db.Exec(ctx, `UPDATE courses SET grade = $2, subject = $3, title = $4, description = $5, updated_at = now() WHERE id = $1`,
		id, c.Grade, c.Subject, c.Title, c.Description)
	if err != nil {
		return Course{}, err
	}
	if tag.RowsAffected() == 0 {
		return Course{}, ErrNotFound
	}
	return s.Course(ctx, id)
}

func (s *Store) DeleteCourse(ctx context.Context, id string) error {
	tag, err := s.db.Exec(ctx, `DELETE FROM courses WHERE id = $1`, id)
	if err == nil && tag.RowsAffected() == 0 {
		return ErrNotFound
	}
	return err
}

// Chapters -------------------------------------------------------------------

const chapterSummaryCols = `id, course_id, position, title, summary, minutes, status, version, updated_at, updated_by`

func (s *Store) Chapters(ctx context.Context, courseID string, publishedOnly bool) ([]ChapterSummary, error) {
	return collect[ChapterSummary](ctx, s.db, `SELECT `+chapterSummaryCols+` FROM chapters
		WHERE course_id = $1 AND (NOT $2 OR status = 'published') ORDER BY position`, courseID, publishedOnly)
}

func (s *Store) Chapter(ctx context.Context, id string) (Chapter, error) {
	return scanChapter(s.db.QueryRow(ctx, `SELECT `+chapterSummaryCols+`, html, lock_admin_id, lock_admin_name, lock_expires_at FROM chapters WHERE id = $1`, id))
}

func scanChapter(row pgx.Row) (Chapter, error) {
	var ch Chapter
	var lockID, lockName *string
	var lockUntil *time.Time
	err := row.Scan(&ch.ID, &ch.CourseID, &ch.Number, &ch.Title, &ch.Summary, &ch.Minutes, &ch.Status, &ch.Version, &ch.UpdatedAt, &ch.UpdatedBy,
		&ch.HTML, &lockID, &lockName, &lockUntil)
	if err != nil {
		return ch, err
	}
	if lockID != nil && lockUntil != nil && lockUntil.After(time.Now()) {
		ch.Lock = &ChapterLock{AdminID: *lockID, AdminName: *lockName, ExpiresAt: *lockUntil}
	}
	return ch, nil
}

func (s *Store) CreateChapter(ctx context.Context, courseID, title, author string) (ChapterSummary, error) {
	var out ChapterSummary
	err := pgx.BeginFunc(ctx, s.db, func(tx pgx.Tx) error {
		var err error
		out, err = one[ChapterSummary](ctx, tx, `INSERT INTO chapters (course_id, position, title, html, updated_by)
			VALUES ($1, (SELECT coalesce(max(position), 0) + 1 FROM chapters WHERE course_id = $1), $2, '<p></p>', $3)
			RETURNING `+chapterSummaryCols, courseID, title, author)
		if err != nil {
			return classify(err)
		}
		_, err = tx.Exec(ctx, `INSERT INTO chapter_versions (chapter_id, version, title, html, author) VALUES ($1, 1, $2, '<p></p>', $3)`, out.ID, title, author)
		if err != nil {
			return err
		}
		_, err = tx.Exec(ctx, `UPDATE courses SET updated_at = now() WHERE id = $1`, courseID)
		return err
	})
	return out, err
}

// ReorderChapters sets positions in the given order. Every chapter of the
// course must be listed exactly once.
func (s *Store) ReorderChapters(ctx context.Context, courseID string, ids []string) error {
	return pgx.BeginFunc(ctx, s.db, func(tx pgx.Tx) error {
		var count int
		if err := tx.QueryRow(ctx, `SELECT count(*) FROM chapters WHERE course_id = $1 AND id = ANY($2)`, courseID, ids).Scan(&count); err != nil {
			return err
		}
		var total int
		if err := tx.QueryRow(ctx, `SELECT count(*) FROM chapters WHERE course_id = $1`, courseID).Scan(&total); err != nil {
			return err
		}
		if count != len(ids) || total != len(ids) {
			return ErrNotFound
		}
		_, err := tx.Exec(ctx, `UPDATE chapters c SET position = o.n FROM unnest($2::text[]) WITH ORDINALITY AS o(id, n)
			WHERE c.id = o.id AND c.course_id = $1`, courseID, ids)
		return err
	})
}

type ChapterUpdate struct {
	Title, Summary, HTML, Status string
	Minutes                      int
	BaseVersion                  int
	Snapshot                     bool
	Author                       string
}

var ErrVersionConflict = &versionConflict{}

type versionConflict struct{}

func (*versionConflict) Error() string { return "version conflict" }

// SaveChapter applies an edit if BaseVersion still matches (optimistic
// concurrency) and optionally records a restorable version.
// saveChapterSQL applies a save in one round trip: the version-checked
// update, the optional snapshot (keeping the 50 most recent), and the
// course's updated_at. Saves are on the editor's critical path and the
// database may be far away, so this must not be split into several queries.
const saveChapterSQL = `WITH upd AS (
	UPDATE chapters SET title = $3, summary = $4, minutes = $5, html = $6, status = $7,
		version = version + 1, updated_at = now(), updated_by = $8
	WHERE id = $1 AND version = $2
	RETURNING *
), snap AS (
	INSERT INTO chapter_versions (chapter_id, version, title, html, author)
	SELECT id, version, title, html, $8 FROM upd WHERE $9
), prune AS (
	DELETE FROM chapter_versions WHERE $9 AND chapter_id = $1 AND EXISTS (SELECT 1 FROM upd) AND version NOT IN
		(SELECT version FROM chapter_versions WHERE chapter_id = $1 ORDER BY version DESC LIMIT 49)
), touch AS (
	UPDATE courses SET updated_at = now() WHERE id = (SELECT course_id FROM upd)
)
SELECT ` + chapterSummaryCols + `, html, lock_admin_id, lock_admin_name, lock_expires_at FROM upd`

// SaveChapter stores a new version of the chapter if BaseVersion is still
// current, otherwise it returns ErrVersionConflict.
func (s *Store) SaveChapter(ctx context.Context, id string, u ChapterUpdate) (Chapter, error) {
	ch, err := scanChapter(s.db.QueryRow(ctx, saveChapterSQL,
		id, u.BaseVersion, u.Title, u.Summary, u.Minutes, u.HTML, u.Status, u.Author, u.Snapshot))
	if err != pgx.ErrNoRows {
		return ch, err
	}
	var exists bool
	if err := s.db.QueryRow(ctx, `SELECT EXISTS (SELECT 1 FROM chapters WHERE id = $1)`, id).Scan(&exists); err != nil {
		return Chapter{}, err
	}
	if exists {
		return Chapter{}, ErrVersionConflict
	}
	return Chapter{}, ErrNotFound
}

func (s *Store) DeleteChapter(ctx context.Context, id string) error {
	return pgx.BeginFunc(ctx, s.db, func(tx pgx.Tx) error {
		var courseID string
		if err := tx.QueryRow(ctx, `DELETE FROM chapters WHERE id = $1 RETURNING course_id`, id).Scan(&courseID); err != nil {
			return err
		}
		// Close the gap in numbering.
		_, err := tx.Exec(ctx, `UPDATE chapters c SET position = o.n FROM
			(SELECT id, row_number() OVER (ORDER BY position) AS n FROM chapters WHERE course_id = $1) o
			WHERE c.id = o.id`, courseID)
		return err
	})
}

// AcquireLock takes or refreshes the editing lock. It returns the current
// holder when someone else holds an unexpired lock.
func (s *Store) AcquireLock(ctx context.Context, chapterID, adminID, adminName string, ttl time.Duration) (ChapterLock, bool, error) {
	var lock ChapterLock
	err := s.db.QueryRow(ctx, `UPDATE chapters SET lock_admin_id = $2, lock_admin_name = $3, lock_expires_at = now() + $4::interval
		WHERE id = $1 AND (lock_admin_id IS NULL OR lock_admin_id = $2 OR lock_expires_at < now())
		RETURNING lock_admin_id, lock_admin_name, lock_expires_at`,
		chapterID, adminID, adminName, ttl.String()).Scan(&lock.AdminID, &lock.AdminName, &lock.ExpiresAt)
	if err == nil {
		return lock, true, nil
	}
	if err != pgx.ErrNoRows {
		return lock, false, err
	}
	err = s.db.QueryRow(ctx, `SELECT lock_admin_id, lock_admin_name, lock_expires_at FROM chapters WHERE id = $1`, chapterID).
		Scan(&lock.AdminID, &lock.AdminName, &lock.ExpiresAt)
	return lock, false, err
}

func (s *Store) ReleaseLock(ctx context.Context, chapterID, adminID string) error {
	_, err := s.db.Exec(ctx, `UPDATE chapters SET lock_admin_id = NULL, lock_admin_name = NULL, lock_expires_at = NULL
		WHERE id = $1 AND lock_admin_id = $2`, chapterID, adminID)
	return err
}

func (s *Store) ChapterVersions(ctx context.Context, chapterID string) ([]ChapterVersion, error) {
	return collect[ChapterVersion](ctx, s.db, `SELECT version, title, created_at, author, '' AS html FROM chapter_versions
		WHERE chapter_id = $1 ORDER BY version DESC`, chapterID)
}

func (s *Store) ChapterVersion(ctx context.Context, chapterID string, version int) (ChapterVersion, error) {
	return one[ChapterVersion](ctx, s.db, `SELECT version, title, created_at, author, html FROM chapter_versions
		WHERE chapter_id = $1 AND version = $2`, chapterID, version)
}

// RestoreVersion copies an old version's content into the chapter as a new
// version (history is never rewritten).
func (s *Store) RestoreVersion(ctx context.Context, chapterID string, version int, author string) (Chapter, error) {
	v, err := s.ChapterVersion(ctx, chapterID, version)
	if err != nil {
		return Chapter{}, err
	}
	cur, err := s.Chapter(ctx, chapterID)
	if err != nil {
		return Chapter{}, err
	}
	return s.SaveChapter(ctx, chapterID, ChapterUpdate{
		Title: v.Title, Summary: cur.Summary, HTML: v.HTML, Status: cur.Status, Minutes: cur.Minutes,
		BaseVersion: cur.Version, Snapshot: true, Author: author + " (restored v" + strconv.Itoa(version) + ")",
	})
}

func (s *Store) ChapterComments(ctx context.Context, chapterID string) ([]ChapterComment, error) {
	return collect[ChapterComment](ctx, s.db, `SELECT id, chapter_id, author_name, body, quote, resolved, created_at
		FROM chapter_comments WHERE chapter_id = $1 ORDER BY created_at`, chapterID)
}

func (s *Store) AddChapterComment(ctx context.Context, chapterID, authorID, authorName, body, quote string) (ChapterComment, error) {
	out, err := one[ChapterComment](ctx, s.db, `INSERT INTO chapter_comments (chapter_id, author_id, author_name, body, quote)
		VALUES ($1, $2, $3, $4, $5) RETURNING id, chapter_id, author_name, body, quote, resolved, created_at`,
		chapterID, authorID, authorName, body, quote)
	return out, classify(err)
}

func (s *Store) SetCommentResolved(ctx context.Context, id string, resolved bool) error {
	tag, err := s.db.Exec(ctx, `UPDATE chapter_comments SET resolved = $2 WHERE id = $1`, id, resolved)
	if err == nil && tag.RowsAffected() == 0 {
		return ErrNotFound
	}
	return err
}

// Progress -------------------------------------------------------------------

func (s *Store) CompletedChapters(ctx context.Context, studentID string) ([]string, error) {
	rows, err := s.db.Query(ctx, `SELECT chapter_id FROM chapter_progress WHERE student_id = $1`, studentID)
	if err != nil {
		return nil, err
	}
	ids, err := pgx.CollectRows(rows, pgx.RowTo[string])
	if ids == nil {
		ids = []string{}
	}
	return ids, err
}

func (s *Store) SetChapterCompleted(ctx context.Context, studentID, chapterID string, completed bool) error {
	if !completed {
		_, err := s.db.Exec(ctx, `DELETE FROM chapter_progress WHERE student_id = $1 AND chapter_id = $2`, studentID, chapterID)
		return err
	}
	_, err := s.db.Exec(ctx, `INSERT INTO chapter_progress (student_id, chapter_id)
		SELECT $1, id FROM chapters WHERE id = $2 AND status = 'published'
		ON CONFLICT DO NOTHING`, studentID, chapterID)
	return classify(err)
}

// levelOrder sorts grades 9-12 first, then remedial and freshman.
func levelOrder(col string) string {
	return "CASE " + col + " WHEN 'remedial' THEN 13 WHEN 'freshman' THEN 14 ELSE " + col + "::int END"
}

// TopicProgress is the finished topics of one chapter.
type TopicProgress struct {
	Done  []string `json:"done"`
	Total int      `json:"total"`
}

// TopicsDone maps chapter ID to the student's finished topics.
func (s *Store) TopicsDone(ctx context.Context, studentID string) (map[string]TopicProgress, error) {
	rows, err := s.db.Query(ctx, `SELECT chapter_id, topics, total FROM topic_progress WHERE student_id = $1`, studentID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := map[string]TopicProgress{}
	for rows.Next() {
		var id string
		var p TopicProgress
		if err := rows.Scan(&id, &p.Done, &p.Total); err != nil {
			return nil, err
		}
		out[id] = p
	}
	return out, rows.Err()
}

// FinishTopic records a finished topic of a published chapter.
func (s *Store) FinishTopic(ctx context.Context, studentID, chapterID, topic string, total int) error {
	tag, err := s.db.Exec(ctx, `INSERT INTO topic_progress (student_id, chapter_id, topics, total)
		SELECT $1, id, ARRAY[$3::text], $4 FROM chapters WHERE id = $2 AND status = 'published'
		ON CONFLICT (student_id, chapter_id) DO UPDATE SET
			topics = CASE WHEN $3 = ANY (topic_progress.topics) THEN topic_progress.topics
				ELSE topic_progress.topics || $3::text END,
			total = $4, updated_at = now()`, studentID, chapterID, topic, total)
	if err == nil && tag.RowsAffected() == 0 {
		return ErrNotFound
	}
	return classify(err)
}
