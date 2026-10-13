package media

import (
	"context"
	"fmt"
	"io"
	"mime/multipart"
	"net/http"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"time"

	"github.com/cloudinary/cloudinary-go/v2/api/uploader"
)

// AvatarFolder holds one profile picture per student, named by student ID:
// a new picture replaces the previous one instead of adding another file,
// and the picture's URL never changes (AvatarURL).
const AvatarFolder = "victory/avatars"

var unsafeID = regexp.MustCompile(`[^A-Za-z0-9_-]`)

func avatarName(studentID string) string { return unsafeID.ReplaceAllString(studentID, "_") }

// AvatarURL is the fixed address of a student's profile picture in
// Cloudinary (always JPEG, no version), or "" with local storage.
func (s *Store) AvatarURL(studentID string) string {
	if s.cld == nil {
		return ""
	}
	return fmt.Sprintf("https://res.cloudinary.com/%s/image/upload/%s/%s.jpg", s.cld.Config.Cloud.CloudName, AvatarFolder, avatarName(studentID))
}

// SaveAvatar stores a student's profile picture, replacing their previous
// one. maxBytes caps the size.
func (s *Store) SaveAvatar(ctx context.Context, fh *multipart.FileHeader, studentID string, maxBytes int64) (string, error) {
	if fh.Size > maxBytes {
		return "", fmt.Errorf("file is too large (max %d MB)", maxBytes>>20)
	}
	f, err := fh.Open()
	if err != nil {
		return "", err
	}
	defer f.Close()
	head := make([]byte, 512)
	n, _ := io.ReadFull(f, head)
	mime := strings.SplitN(http.DetectContentType(head[:n]), ";", 2)[0]
	if allowed[mime] != "image" {
		return "", ErrUnsupported
	}
	if _, err := f.Seek(0, io.SeekStart); err != nil {
		return "", err
	}
	name := avatarName(studentID)

	if s.cld != nil {
		yes := true
		// Stored as JPEG so the address is the same whatever was uploaded;
		// Invalidate clears the old picture from Cloudinary's CDN.
		res, err := s.cld.Upload.Upload(ctx, f, uploader.UploadParams{
			PublicID: AvatarFolder + "/" + name, ResourceType: "image", Format: "jpg",
			Overwrite: &yes, Invalidate: &yes,
		})
		if err != nil {
			return "", err
		}
		if res.Error.Message != "" {
			return "", fmt.Errorf("cloudinary: %s", res.Error.Message)
		}
		return s.AvatarURL(studentID), nil
	}

	s.removeLocalAvatar(name)
	file := "avatar-" + name + extensions[mime]
	out, err := os.Create(filepath.Join(s.dir, file))
	if err != nil {
		return "", err
	}
	defer out.Close()
	if _, err := io.Copy(out, f); err != nil {
		return "", err
	}
	return fmt.Sprintf("%s/uploads/%s?v=%d", s.publicBase, file, time.Now().Unix()), nil
}

// DeleteAvatar removes a student's profile picture from storage.
func (s *Store) DeleteAvatar(ctx context.Context, studentID string) error {
	name := avatarName(studentID)
	if s.cld == nil {
		s.removeLocalAvatar(name)
		return nil
	}
	yes := true
	_, err := s.cld.Upload.Destroy(ctx, uploader.DestroyParams{PublicID: AvatarFolder + "/" + name, ResourceType: "image", Invalidate: &yes})
	return err
}

func (s *Store) removeLocalAvatar(name string) {
	for _, ext := range extensions {
		_ = os.Remove(filepath.Join(s.dir, "avatar-"+name+ext))
	}
}

// LegacyImageID is the Cloudinary public ID of an image this store
// uploaded to the lessons folder (where profile pictures went before
// AvatarFolder), or "" for any other URL.
func (s *Store) LegacyImageID(url string) string {
	if s.cld == nil {
		return ""
	}
	prefix := fmt.Sprintf("https://res.cloudinary.com/%s/image/upload/", s.cld.Config.Cloud.CloudName)
	rest, ok := strings.CutPrefix(url, prefix)
	if !ok {
		return ""
	}
	// Skip the version segment ("v1791896045/").
	if i := strings.Index(rest, "/"); i > 1 && rest[0] == 'v' && strings.Trim(rest[1:i], "0123456789") == "" {
		rest = rest[i+1:]
	}
	if !strings.HasPrefix(rest, Folder+"/") || strings.ContainsAny(rest, "?#") {
		return ""
	}
	if i := strings.LastIndex(rest, "."); i > 0 {
		rest = rest[:i]
	}
	return rest
}

// DeleteImage removes an image by its Cloudinary public ID.
func (s *Store) DeleteImage(ctx context.Context, publicID string) error {
	if s.cld == nil || publicID == "" {
		return nil
	}
	yes := true
	_, err := s.cld.Upload.Destroy(ctx, uploader.DestroyParams{PublicID: publicID, ResourceType: "image", Invalidate: &yes})
	return err
}
