package store

import (
	"context"
	"strings"
)

const mediaCols = `id, url, kind, content_type, file_name, size_bytes, uploader_name, created_at`

// RecordUpload stores metadata for an uploaded file.
func (s *Store) RecordUpload(ctx context.Context, m MediaUpload, adminID string) (MediaUpload, error) {
	return one[MediaUpload](ctx, s.db, `INSERT INTO media_uploads (url, kind, content_type, file_name, size_bytes, uploaded_by, uploader_name)
		VALUES ($1, $2, $3, $4, $5, NULLIF($6, ''), $7) RETURNING `+mediaCols,
		m.URL, m.Kind, m.ContentType, m.FileName, m.SizeBytes, adminID, m.UploaderName)
}

// MediaUploads lists uploads newest first, optionally filtered by kind.
func (s *Store) MediaUploads(ctx context.Context, kind string, limit int) ([]MediaUpload, error) {
	return collect[MediaUpload](ctx, s.db, `SELECT `+mediaCols+` FROM media_uploads
		WHERE $1 = '' OR kind = $1 ORDER BY created_at DESC LIMIT $2`, kind, limit)
}

// MediaUploadByID returns one upload.
func (s *Store) MediaUploadByID(ctx context.Context, id string) (MediaUpload, error) {
	return one[MediaUpload](ctx, s.db, `SELECT `+mediaCols+` FROM media_uploads WHERE id = $1`, id)
}

// DeleteMediaUpload removes an upload from the list.
func (s *Store) DeleteMediaUpload(ctx context.Context, id string) error {
	_, err := s.db.Exec(ctx, `DELETE FROM media_uploads WHERE id = $1`, id)
	return err
}

// MediaUse is a place that shows an uploaded file.
type MediaUse struct {
	Kind  string `json:"kind" db:"kind"`
	Title string `json:"title" db:"title"`
}

// MediaUses lists content that refers to a file, found by the unique part
// of its name (so other sizes or formats of the same file count too).
func (s *Store) MediaUses(ctx context.Context, fileKey string) ([]MediaUse, error) {
	pattern := "%" + strings.NewReplacer(`\`, `\`, `%`, `\%`, `_`, `\_`).Replace(fileKey) + "%"
	return collect[MediaUse](ctx, s.db, `
		SELECT 'Chapter' AS kind, title FROM chapters WHERE html LIKE $1
		UNION ALL SELECT 'Training', title FROM trainings WHERE html LIKE $1
		UNION ALL SELECT 'Question', left(regexp_replace(text, '<[^>]+>', '', 'g'), 80) FROM questions
			WHERE text LIKE $1 OR explanation LIKE $1 OR image_url LIKE $1 OR array_to_string(options, ' ') LIKE $1
				OR explanation_image_url LIKE $1 OR array_to_string(option_images, ' ') LIKE $1
		UNION ALL SELECT 'Announcement', title FROM announcements WHERE body LIKE $1
		UNION ALL SELECT 'Site setting', key FROM site_settings WHERE value LIKE $1
		LIMIT 20`, pattern)
}
