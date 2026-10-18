package api

import (
	"context"
	"errors"
	"io"

	"github.com/gin-gonic/gin"

	"victor-contest-go/internal/docx"
	"victor-contest-go/internal/httpx"
)

// maxDocx keeps a Word upload within what the API can receive in one
// request (AWS Lambda takes about 6 MB, and the file grows on the way).
const maxDocx = 4 << 20

// POST /admin/services/chapters/convert (multipart "file": a .docx) — the
// lessons in a Word chapter, to preview before importing them with
// /services/chapters/import. Its pictures are stored now and shown in place.
func (s *Server) adminConvertChapters(c *gin.Context) error {
	fh, err := c.FormFile("file")
	if err != nil {
		return httpx.Field("file", "Choose a Word file (.docx).")
	}
	if fh.Size > maxDocx {
		return httpx.Field("file", "This Word file is larger than 4 MB. Make its pictures smaller in Word "+
			"(Picture Format > Compress Pictures), or split it into two files.")
	}
	f, err := fh.Open()
	if err != nil {
		return err
	}
	defer f.Close()
	data, err := io.ReadAll(io.LimitReader(f, maxDocx+1))
	if err != nil {
		return err
	}
	res, err := docx.Convert(c, data, docx.Options{
		SavePicture: func(ctx context.Context, b []byte, _ string) (string, error) {
			saved, err := s.media.StoreBytes(ctx, b)
			if err != nil {
				return "", err
			}
			if saved.Kind != "image" {
				return "", errors.New("not a picture")
			}
			return saved.URL, nil
		},
	})
	if errors.Is(err, docx.ErrNotDocx) {
		return httpx.Field("file", "This isn't a Word (.docx) file. For an older .doc file, open it in Word and "+
			"save it as .docx first.")
	}
	if err != nil {
		return err
	}
	return httpx.OK(c, res)
}
