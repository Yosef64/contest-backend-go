package media

import (
	"context"
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/cloudinary/cloudinary-go/v2/api"
	"github.com/cloudinary/cloudinary-go/v2/api/uploader"
)

// Folder holds every lesson and question upload in Cloudinary.
const Folder = "victory/lessons"

// Direct is a signed upload the admin's browser sends straight to
// Cloudinary. Large files can't pass through the API (Lambda caps a request
// at 6 MB), so bulk uploads and videos go this way.
type Direct struct {
	UploadURL string            `json:"upload_url"`
	Fields    map[string]string `json:"fields"`
}

// directTypes are the types accepted for direct uploads (the browser's
// declared type; Cloudinary checks the content against allowed_formats).
var directTypes = map[string]string{
	"image/png":       "image",
	"image/jpeg":      "image",
	"image/webp":      "image",
	"image/gif":       "image",
	"video/mp4":       "video",
	"video/webm":      "video",
	"video/quicktime": "video", // iPhone recordings, served as MP4
}

var directFormats = map[string]string{
	"image": "png,jpg,jpeg,webp,gif",
	"video": "mp4,webm,mov",
}

// Check returns the kind ("image" or "video") of a file about to be
// uploaded, or an error for an unsupported type or a file that is too big.
func Check(contentType string, size int64) (string, error) {
	kind, ok := directTypes[contentType]
	if !ok {
		return "", ErrUnsupported
	}
	if (kind == "image" && size > MaxImageBytes) || (kind == "video" && size > MaxVideoBytes) {
		return "", fmt.Errorf("file is too large (max %d MB for %ss)", map[string]int{"image": MaxImageBytes >> 20, "video": MaxVideoBytes >> 20}[kind], kind)
	}
	return kind, nil
}

// SignDirect signs a direct upload of the given kind. It returns nil when
// files are stored on local disk (development): upload through the API then.
func (s *Store) SignDirect(kind string) (*Direct, error) {
	if s.cld == nil {
		return nil, nil
	}
	cloud := s.cld.Config.Cloud
	params := url.Values{
		"folder":          {Folder},
		"timestamp":       {strconv.FormatInt(time.Now().Unix(), 10)},
		"allowed_formats": {directFormats[kind]},
	}
	sig, err := api.SignParameters(params, cloud.APISecret)
	if err != nil {
		return nil, err
	}
	fields := map[string]string{"api_key": cloud.APIKey, "signature": sig}
	for k := range params {
		fields[k] = params.Get(k)
	}
	return &Direct{
		UploadURL: fmt.Sprintf("https://api.cloudinary.com/v1_1/%s/%s/upload", cloud.CloudName, kind),
		Fields:    fields,
	}, nil
}

// DirectURL checks that a finished direct upload is ours (this cloud, the
// lessons folder, the expected kind) and returns the URL to serve: videos
// always as MP4, which Cloudinary converts to on request.
func (s *Store) DirectURL(raw, kind string) (string, bool) {
	if s.cld == nil {
		return "", false
	}
	prefix := fmt.Sprintf("https://res.cloudinary.com/%s/%s/upload/", s.cld.Config.Cloud.CloudName, kind)
	if !strings.HasPrefix(raw, prefix) || !strings.Contains(raw, "/"+Folder+"/") || strings.ContainsAny(raw, "?#\"<> ") {
		return "", false
	}
	if kind == "video" {
		if i := strings.LastIndex(raw, "."); i > len(prefix) && strings.EqualFold(raw[i:], ".mov") {
			raw = raw[:i] + ".mp4"
		}
	}
	return raw, true
}

// FileKey is the unique part of an uploaded file's name (its random ID),
// used to find content that shows the file. "" if the URL isn't ours.
func FileKey(url string) string {
	if i := strings.IndexAny(url, "?#"); i >= 0 {
		url = url[:i]
	}
	name := url[strings.LastIndex(url, "/")+1:]
	if i := strings.LastIndex(name, "."); i > 0 {
		name = name[:i]
	}
	if len(name) < 8 {
		return "" // too short to identify one file safely
	}
	return name
}

// DeleteUpload removes an uploaded lesson file from storage. A file that is
// already gone counts as deleted.
func (s *Store) DeleteUpload(ctx context.Context, url, kind string) error {
	if s.cld == nil {
		prefix := s.publicBase + "/uploads/"
		if name, ok := strings.CutPrefix(url, prefix); ok && !strings.ContainsAny(name, `/\`) {
			if err := os.Remove(filepath.Join(s.dir, name)); err != nil && !os.IsNotExist(err) {
				return err
			}
		}
		return nil
	}
	prefix := fmt.Sprintf("https://res.cloudinary.com/%s/%s/upload/", s.cld.Config.Cloud.CloudName, kind)
	rest, ok := strings.CutPrefix(url, prefix)
	if !ok {
		return nil // not stored in this Cloudinary account
	}
	if i := strings.Index(rest, "/"); i > 1 && rest[0] == 'v' && strings.Trim(rest[1:i], "0123456789") == "" {
		rest = rest[i+1:]
	}
	if i := strings.LastIndex(rest, "."); i > 0 {
		rest = rest[:i]
	}
	yes := true
	_, err := s.cld.Upload.Destroy(ctx, uploader.DestroyParams{PublicID: rest, ResourceType: kind, Invalidate: &yes})
	return err
}
