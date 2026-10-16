// Package media stores uploaded lesson images and videos: in Cloudinary when
// CLOUDINARY_URL is set, otherwise on local disk (development).
package media

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"mime/multipart"
	"net/http"
	"os"
	"path/filepath"
	"strings"

	"github.com/cloudinary/cloudinary-go/v2"
	"github.com/cloudinary/cloudinary-go/v2/api/uploader"
)

const (
	MaxImageBytes = 8 << 20
	MaxVideoBytes = 100 << 20
)

var allowed = map[string]string{
	"image/png":  "image",
	"image/jpeg": "image",
	"image/webp": "image",
	"image/gif":  "image",
	"video/mp4":  "video",
	"video/webm": "video",
}

// extensions maps each allowed type to the extension it is stored under.
var extensions = map[string]string{
	"image/png":  ".png",
	"image/jpeg": ".jpg",
	"image/webp": ".webp",
	"image/gif":  ".gif",
	"video/mp4":  ".mp4",
	"video/webm": ".webm",
}

var ErrUnsupported = errors.New("only PNG, JPEG, WebP, GIF images and MP4/WebM videos are allowed")

type Store struct {
	cld        *cloudinary.Cloudinary
	dir        string
	publicBase string
}

func New(cloudinaryURL, uploadDir, publicBase string) (*Store, error) {
	s := &Store{dir: uploadDir, publicBase: publicBase}
	if cloudinaryURL != "" {
		cld, err := cloudinary.NewFromURL(cloudinaryURL)
		if err != nil {
			return nil, fmt.Errorf("cloudinary: %w", err)
		}
		s.cld = cld
	} else if err := os.MkdirAll(uploadDir, 0o755); err != nil {
		return nil, err
	}
	return s, nil
}

// LocalDir is the directory to serve at /uploads, or "" when using Cloudinary.
func (s *Store) LocalDir() string {
	if s.cld != nil {
		return ""
	}
	return s.dir
}

// Saved describes a stored file.
type Saved struct {
	URL         string
	Kind        string // "image" or "video"
	ContentType string // detected from the file's bytes
}

// Save validates and stores the file, returning its public URL and kind.
func (s *Store) Save(ctx context.Context, fh *multipart.FileHeader) (url, kind string, err error) {
	saved, err := s.Store(ctx, fh)
	return saved.URL, saved.Kind, err
}

// Store validates and stores the file.
func (s *Store) Store(ctx context.Context, fh *multipart.FileHeader) (Saved, error) {
	f, err := fh.Open()
	if err != nil {
		return Saved{}, err
	}
	defer f.Close()

	head := make([]byte, 512)
	n, _ := io.ReadFull(f, head)
	mime := strings.SplitN(http.DetectContentType(head[:n]), ";", 2)[0]
	kind, ok := allowed[mime]
	if !ok {
		return Saved{}, ErrUnsupported
	}
	if (kind == "image" && fh.Size > MaxImageBytes) || (kind == "video" && fh.Size > MaxVideoBytes) {
		return Saved{}, fmt.Errorf("file is too large (max %d MB for %ss)", map[string]int{"image": MaxImageBytes >> 20, "video": MaxVideoBytes >> 20}[kind], kind)
	}
	if _, err := f.Seek(0, io.SeekStart); err != nil {
		return Saved{}, err
	}

	if s.cld != nil {
		res, err := s.cld.Upload.Upload(ctx, f, uploader.UploadParams{Folder: Folder, ResourceType: kind})
		if err != nil {
			return Saved{}, err
		}
		return Saved{res.SecureURL, kind, mime}, nil
	}

	// The extension comes from the detected type, never the client's file
	// name: files are served by extension, so "photo.html" must not become
	// an HTML page.
	ext := extensions[mime]
	buf := make([]byte, 12)
	_, _ = rand.Read(buf)
	name := hex.EncodeToString(buf) + ext
	out, err := os.Create(filepath.Join(s.dir, name))
	if err != nil {
		return Saved{}, err
	}
	defer out.Close()
	if _, err := io.Copy(out, f); err != nil {
		return Saved{}, err
	}
	return Saved{s.publicBase + "/uploads/" + name, kind, mime}, nil
}
