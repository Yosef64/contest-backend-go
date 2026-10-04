package store

import "context"

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
