package api

import (
	"errors"
	"strings"

	"github.com/gin-gonic/gin"

	"victor-contest-go/internal/httpx"
	"victor-contest-go/internal/media"
)

// maxPhotoBytes keeps profile pictures small; the app resizes before upload.
const maxPhotoBytes = 3 << 20

// putMyPhoto uploads a new profile picture (multipart field "file").
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
	url, kind, err := s.media.Save(c, fh)
	if errors.Is(err, media.ErrUnsupported) || (err == nil && kind != "image") {
		return httpx.Field("file", "Use a PNG, JPEG, WebP or GIF picture.")
	}
	if err != nil {
		if strings.HasPrefix(err.Error(), "file is too large") {
			return httpx.Field("file", err.Error())
		}
		return err
	}
	updated, err := s.store.SetStudentPhoto(c, st.ID, url)
	if err != nil {
		return err
	}
	return httpx.OK(c, gin.H{"student": updated})
}

// deleteMyPhoto removes the profile picture (initials are shown instead).
func (s *Server) deleteMyPhoto(c *gin.Context) error {
	st, err := s.me(c)
	if err != nil {
		return err
	}
	updated, err := s.store.SetStudentPhoto(c, st.ID, "")
	if err != nil {
		return err
	}
	return httpx.OK(c, gin.H{"student": updated})
}
