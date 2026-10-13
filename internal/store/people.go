package store

import (
	"context"
	"encoding/base64"
	"fmt"
	"strconv"
	"strings"

	"github.com/jackc/pgx/v5"
)

// Admins ----------------------------------------------------------------------

const adminCols = `id, name, email, password_hash, is_approved, is_owner, permission_role, school_id, created_at`

func (s *Store) AdminByEmail(ctx context.Context, email string) (Admin, error) {
	return one[Admin](ctx, s.db, `SELECT `+adminCols+` FROM admins WHERE lower(email) = lower($1)`, email)
}

func (s *Store) AdminByID(ctx context.Context, id string) (Admin, error) {
	return one[Admin](ctx, s.db, `SELECT `+adminCols+` FROM admins WHERE id = $1`, id)
}

func (s *Store) Admins(ctx context.Context) ([]Admin, error) {
	return collect[Admin](ctx, s.db, `SELECT `+adminCols+` FROM admins ORDER BY is_owner DESC, created_at`)
}

func (s *Store) CountAdmins(ctx context.Context) (int, error) {
	var n int
	err := s.db.QueryRow(ctx, `SELECT count(*) FROM admins`).Scan(&n)
	return n, err
}

func (s *Store) CreateAdmin(ctx context.Context, a Admin) (Admin, error) {
	out, err := one[Admin](ctx, s.db, `INSERT INTO admins (name, email, password_hash, is_approved, is_owner)
		VALUES ($1, $2, $3, $4, $5) RETURNING `+adminCols, a.Name, strings.TrimSpace(a.Email), a.PasswordHash, a.IsApproved, a.IsOwner)
	return out, classify(err)
}

func (s *Store) SetAdminApproved(ctx context.Context, id string, approved bool) error {
	tag, err := s.db.Exec(ctx, `UPDATE admins SET is_approved = $2 WHERE id = $1 AND NOT is_owner`, id, approved)
	if err == nil && tag.RowsAffected() == 0 {
		return ErrNotFound
	}
	return err
}

func (s *Store) DeleteAdmin(ctx context.Context, id string) error {
	tag, err := s.db.Exec(ctx, `DELETE FROM admins WHERE id = $1 AND NOT is_owner`, id)
	if err == nil && tag.RowsAffected() == 0 {
		return ErrNotFound
	}
	return err
}

// Students --------------------------------------------------------------------

const studentCols = `id, auth_provider, auth_subject, name, phone_number, gender, age, school, grade, city, region, photo_url, photo_custom, is_suspended, created_at, updated_at`

func (s *Store) StudentByID(ctx context.Context, id string) (Student, error) {
	return one[Student](ctx, s.db, `SELECT `+studentCols+` FROM students WHERE id = $1`, id)
}

func (s *Store) StudentByIdentity(ctx context.Context, provider, subject string) (Student, error) {
	return one[Student](ctx, s.db, `SELECT `+studentCols+` FROM students WHERE auth_provider = $1 AND auth_subject = $2`, provider, subject)
}

// UpsertStudentProfile creates the student on first sign-up or updates the
// editable profile fields.
func (s *Store) UpsertStudentProfile(ctx context.Context, st Student) (Student, error) {
	out, err := one[Student](ctx, s.db, `INSERT INTO students
		(id, auth_provider, auth_subject, name, phone_number, gender, age, school, grade, city, region, photo_url)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12)
		ON CONFLICT (id) DO UPDATE SET
			name = EXCLUDED.name, phone_number = EXCLUDED.phone_number, gender = EXCLUDED.gender,
			age = EXCLUDED.age, school = EXCLUDED.school, grade = EXCLUDED.grade, city = EXCLUDED.city,
			region = EXCLUDED.region, updated_at = now()
		RETURNING `+studentCols,
		st.ID, st.AuthProvider, st.AuthSubject, st.Name, st.PhoneNumber, st.Gender, st.Age, st.School, st.Grade, st.City, st.Region, st.PhotoURL)
	return out, classify(err)
}

// SetStudentPhoto replaces the student's profile picture URL ("" removes it).
// SetStudentPhoto sets the picture the student chose (or "" for none).
func (s *Store) SetStudentPhoto(ctx context.Context, id, url string) (Student, error) {
	return one[Student](ctx, s.db, `UPDATE students SET photo_url = $2, photo_custom = true, updated_at = now() WHERE id = $1 RETURNING `+studentCols, id, url)
}

// SyncSignInPhoto updates the picture from the sign-in account, unless the
// student chose their own.
func (s *Store) SyncSignInPhoto(ctx context.Context, id, url string) (Student, error) {
	return one[Student](ctx, s.db, `UPDATE students SET photo_url = $2, updated_at = now() WHERE id = $1 AND NOT photo_custom RETURNING `+studentCols, id, url)
}

// DeleteStudent removes the student and, by cascade, their registrations,
// submissions and progress. A password account's login is removed too, so
// the email or phone can be used again.
func (s *Store) DeleteStudent(ctx context.Context, id string) error {
	return pgx.BeginFunc(ctx, s.db, func(tx pgx.Tx) error {
		tag, err := tx.Exec(ctx, `DELETE FROM students WHERE id = $1`, id)
		if err != nil {
			return err
		}
		if tag.RowsAffected() == 0 {
			return ErrNotFound
		}
		_, err = tx.Exec(ctx, `DELETE FROM student_credentials WHERE student_id = $1`, id)
		return err
	})
}

func (s *Store) SetStudentSuspended(ctx context.Context, id string, suspended bool) error {
	tag, err := s.db.Exec(ctx, `UPDATE students SET is_suspended = $2, updated_at = now() WHERE id = $1`, id, suspended)
	if err == nil && tag.RowsAffected() == 0 {
		return ErrNotFound
	}
	return err
}

// Students lists students newest first with keyset pagination. The cursor
// is an opaque base64 of "created_at|id".
// Students lists students newest first. department ("entrance", "remedial",
// "freshman") and stream ("natural", "social") filter on the academic profile,
// as does school (a partner-school id).
func (s *Store) Students(ctx context.Context, q, grade, department, stream, school, cursor string, limit int) (Page[Student], error) {
	where := []string{"true"}
	args := []any{}
	add := func(cond string, v any) {
		args = append(args, v)
		where = append(where, fmt.Sprintf(cond, len(args)))
	}
	if q != "" {
		add("(name ILIKE '%%' || $%[1]d || '%%' OR school ILIKE '%%' || $%[1]d || '%%')", q)
	}
	if grade != "" {
		add("grade = $%d", grade)
	}
	if department == "none" {
		add("id NOT IN (SELECT student_id FROM academic_profiles WHERE department <> $%d)", "")
	} else if department != "" {
		add("id IN (SELECT student_id FROM academic_profiles WHERE department = $%d)", department)
	}
	if stream != "" {
		add("id IN (SELECT student_id FROM academic_profiles WHERE stream = $%d)", stream)
	}
	if school != "" {
		add("id IN (SELECT student_id FROM academic_profiles WHERE school_id = $%d)", school)
	}
	filter := strings.Join(where, " AND ")

	var total int
	if err := s.db.QueryRow(ctx, `SELECT count(*) FROM students WHERE `+filter, args...).Scan(&total); err != nil {
		return Page[Student]{}, err
	}
	if cursor != "" {
		raw, err := base64.RawURLEncoding.DecodeString(cursor)
		parts := strings.SplitN(string(raw), "|", 2)
		if err == nil && len(parts) == 2 {
			args = append(args, parts[0], parts[1])
			filter += fmt.Sprintf(" AND (created_at, id) < ($%d::timestamptz, $%d)", len(args)-1, len(args))
		}
	}
	args = append(args, limit+1)
	items, err := collect[Student](ctx, s.db, `SELECT `+studentCols+` FROM students WHERE `+filter+
		` ORDER BY created_at DESC, id DESC LIMIT $`+strconv.Itoa(len(args)), args...)
	if err != nil {
		return Page[Student]{}, err
	}
	page := Page[Student]{Items: items, Total: total}
	if len(items) > limit {
		last := items[limit-1]
		next := base64.RawURLEncoding.EncodeToString([]byte(last.CreatedAt.Format("2006-01-02T15:04:05.999999999Z07:00") + "|" + last.ID))
		page.Items = items[:limit]
		page.NextCursor = &next
	}
	return page, nil
}
