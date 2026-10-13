package api

import (
	"errors"
	"log/slog"
	"strings"

	"github.com/gin-gonic/gin"

	"victor-contest-go/internal/httpx"
	"victor-contest-go/internal/media"
)

// maxPhotoBytes keeps profile pictures small; the app resizes before upload.
const maxPhotoBytes = 3 << 20

// putMyPhoto uploads a new profile picture (multipart field "file"). Each
// student has one picture at a fixed URL: a new one replaces the old.
func (s *Server) putMyPhoto(c *gin.Context) error {
	st, err := s.me(c)
	if err != nil {
		return err
	}
	fh, err := c.FormFile("file")
	if err != nil {
		return httpx.Field("file", "Choose a picture to upload.")
	}
	if fh.Size > maxPhotoBytes {
		return httpx.Field("file", "Choose a picture under 3 MB.")
	}
	url, err := s.media.SaveAvatar(c, fh, st.ID, maxPhotoBytes)
	if errors.Is(err, media.ErrUnsupported) {
		return httpx.Field("file", "Use a PNG, JPEG, WebP or GIF picture.")
	}
	if err != nil {
		if strings.HasPrefix(err.Error(), "file is too large") {
			return httpx.Field("file", "Choose a picture under 3 MB.")
		}
		return err
	}
	updated, err := s.store.SetStudentPhoto(c, st.ID, url)
	if err != nil {
		return err
	}
	if st.PhotoURL != url {
		s.deleteLegacyPhoto(c, st.PhotoURL)
	}
	return httpx.OK(c, gin.H{"student": updated})
}

// deleteMyPhoto removes the profile picture (initials are shown instead)
// and deletes the file.
func (s *Server) deleteMyPhoto(c *gin.Context) error {
	st, err := s.me(c)
	if err != nil {
		return err
	}
	updated, err := s.store.SetStudentPhoto(c, st.ID, "")
	if err != nil {
		return err
	}
	if err := s.media.DeleteAvatar(c, st.ID); err != nil {
		slog.Error("delete profile picture", "student", st.ID, "err", err)
	}
	s.deleteLegacyPhoto(c, st.PhotoURL)
	return httpx.OK(c, gin.H{"student": updated})
}

// deleteLegacyPhoto deletes a picture uploaded before profile pictures had
// a fixed place (it sits in the lessons folder), unless an admin also
// listed it in Uploads.
func (s *Server) deleteLegacyPhoto(c *gin.Context, url string) {
	id := s.media.LegacyImageID(url)
	if id == "" {
		return
	}
	var listed bool
	if err := s.pool.QueryRow(c, `SELECT EXISTS (SELECT 1 FROM media_uploads WHERE url = $1)`, url).Scan(&listed); err != nil || listed {
		return
	}
	if err := s.media.DeleteImage(c, id); err != nil {
		slog.Error("delete old profile picture", "public_id", id, "err", err)
	}
}
