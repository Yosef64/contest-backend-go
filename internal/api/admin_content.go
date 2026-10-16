package api

import (
	"errors"
	"net/http"
	"strconv"
	"strings"

	"github.com/gin-gonic/gin"

	"victor-contest-go/internal/content"
	"victor-contest-go/internal/httpx"
	"victor-contest-go/internal/media"
	"victor-contest-go/internal/store"
)

type courseInput struct {
	Grade       string `json:"grade" binding:"required"`
	Subject     string `json:"subject" binding:"required"`
	Title       string `json:"title" binding:"required,max=120"`
	Description string `json:"description" binding:"max=600"`
}

func (in *courseInput) toCourse() (store.Course, error) {
	grade := questionLevel(in.Grade)
	if grade == "" {
		return store.Course{}, httpx.Field("grade", "Choose grade 9, 10, 11, 12, remedial or freshman.")
	}
	if !subjectOK(in.Subject, true) {
		return store.Course{}, httpx.Field("subject", "Choose a supported subject.")
	}
	return store.Course{Grade: grade, Subject: in.Subject, Title: strings.TrimSpace(in.Title), Description: strings.TrimSpace(in.Description)}, nil
}

func (s *Server) adminCourses(c *gin.Context) error {
	grade := ""
	if g := c.Query("grade"); g != "" {
		if grade = questionLevel(g); grade == "" {
			return httpx.BadRequest("Unknown grade.")
		}
	}
	list, err := s.store.Courses(c, grade, false)
	if err != nil {
		return err
	}
	return httpx.OK(c, gin.H{"courses": list})
}

func (s *Server) adminCreateCourse(c *gin.Context) error {
	var in courseInput
	if err := httpx.Bind(c, &in); err != nil {
		return err
	}
	course, err := in.toCourse()
	if err != nil {
		return err
	}
	out, err := s.store.CreateCourse(c, course)
	if err != nil {
		return storeErr(err, "Course")
	}
	return httpx.Created(c, gin.H{"course": out})
}

func (s *Server) adminCourse(c *gin.Context) error {
	course, err := s.store.Course(c, c.Param("id"))
	if err != nil {
		return storeErr(err, "Course")
	}
	chapters, err := s.store.Chapters(c, course.ID, false)
	if err != nil {
		return err
	}
	return httpx.OK(c, gin.H{"course": course, "chapters": chapters})
}

func (s *Server) adminUpdateCourse(c *gin.Context) error {
	var in courseInput
	if err := httpx.Bind(c, &in); err != nil {
		return err
	}
	course, err := in.toCourse()
	if err != nil {
		return err
	}
	out, err := s.store.UpdateCourse(c, c.Param("id"), course)
	if err != nil {
		return storeErr(err, "Course")
	}
	return httpx.OK(c, gin.H{"course": out})
}

func (s *Server) adminDeleteCourse(c *gin.Context) error {
	return orNoContent(c, storeErr(s.store.DeleteCourse(c, c.Param("id")), "Course"))
}

func (s *Server) adminCreateChapter(c *gin.Context) error {
	var in struct {
		Title string `json:"title" binding:"required,max=160"`
	}
	if err := httpx.Bind(c, &in); err != nil {
		return err
	}
	if _, err := s.store.Course(c, c.Param("id")); err != nil {
		return storeErr(err, "Course")
	}
	ch, err := s.store.CreateChapter(c, c.Param("id"), strings.TrimSpace(in.Title), currentAdmin(c).Name)
	if err != nil {
		return err
	}
	return httpx.Created(c, gin.H{"chapter": ch})
}

func (s *Server) adminReorderChapters(c *gin.Context) error {
	var in struct {
		IDs []string `json:"ids" binding:"required,min=1"`
	}
	if err := httpx.Bind(c, &in); err != nil {
		return err
	}
	if err := s.store.ReorderChapters(c, c.Param("id"), in.IDs); err != nil {
		if errors.Is(err, store.ErrNotFound) {
			return httpx.BadRequest("The list must contain every chapter of this course exactly once.")
		}
		return err
	}
	return httpx.NoContent(c)
}

func (s *Server) adminChapter(c *gin.Context) error {
	ch, err := s.store.Chapter(c, c.Param("id"))
	if err != nil {
		return storeErr(err, "Chapter")
	}
	return httpx.OK(c, gin.H{"chapter": ch})
}

const maxChapterHTML = 512 << 10

func (s *Server) adminSaveChapter(c *gin.Context) error {
	var in struct {
		Title       *string `json:"title" binding:"omitempty,max=160"`
		Summary     *string `json:"summary" binding:"omitempty,max=600"`
		Minutes     *int    `json:"minutes" binding:"omitempty,min=1,max=240"`
		HTML        *string `json:"html"`
		Status      *string `json:"status" binding:"omitempty,oneof=draft published"`
		BaseVersion int     `json:"base_version" binding:"required,min=1"`
		Snapshot    bool    `json:"snapshot"`
	}
	if err := httpx.Bind(c, &in); err != nil {
		return err
	}
	admin := currentAdmin(c)
	cur, err := s.store.Chapter(c, c.Param("id"))
	if err != nil {
		return storeErr(err, "Chapter")
	}
	if cur.Lock != nil && cur.Lock.AdminID != admin.ID {
		return httpx.NewError(http.StatusLocked, "locked", cur.Lock.AdminName+" is editing this chapter.")
	}
	u := store.ChapterUpdate{
		Title: cur.Title, Summary: cur.Summary, HTML: cur.HTML, Status: cur.Status, Minutes: cur.Minutes,
		BaseVersion: in.BaseVersion, Snapshot: in.Snapshot, Author: admin.Name,
	}
	if in.Title != nil {
		if u.Title = strings.TrimSpace(*in.Title); u.Title == "" {
			return httpx.Field("title", "This field is required.")
		}
	}
	if in.Summary != nil {
		u.Summary = strings.TrimSpace(*in.Summary)
	}
	if in.Minutes != nil {
		u.Minutes = *in.Minutes
	}
	if in.Status != nil {
		u.Status = *in.Status
		u.Snapshot = u.Snapshot || u.Status != cur.Status
	}
	if in.HTML != nil {
		if len(*in.HTML) > maxChapterHTML {
			return httpx.Field("html", "This chapter is too long. Split it into two chapters.")
		}
		u.HTML = content.SanitizeHTML(*in.HTML)
	}
	ch, err := s.store.SaveChapter(c, cur.ID, u)
	if err != nil {
		return storeErr(err, "Chapter")
	}
	return httpx.OK(c, gin.H{"chapter": ch})
}

func (s *Server) adminDeleteChapter(c *gin.Context) error {
	return orNoContent(c, storeErr(s.store.DeleteChapter(c, c.Param("id")), "Chapter"))
}

func (s *Server) adminLockChapter(c *gin.Context) error {
	admin := currentAdmin(c)
	lock, ok, err := s.store.AcquireLock(c, c.Param("id"), admin.ID, admin.Name, lockTTL)
	if err != nil {
		return storeErr(err, "Chapter")
	}
	if !ok {
		// The dashboard shows the message as the holder's name.
		return httpx.NewError(http.StatusLocked, "locked", lock.AdminName)
	}
	return httpx.OK(c, gin.H{"lock": lock})
}

func (s *Server) adminUnlockChapter(c *gin.Context) error {
	return orNoContent(c, s.store.ReleaseLock(c, c.Param("id"), currentAdmin(c).ID))
}

func (s *Server) adminVersions(c *gin.Context) error {
	list, err := s.store.ChapterVersions(c, c.Param("id"))
	if err != nil {
		return err
	}
	return httpx.OK(c, gin.H{"versions": list})
}

func versionParam(c *gin.Context) (int, error) {
	v, err := strconv.Atoi(c.Param("v"))
	if err != nil || v < 1 {
		return 0, httpx.BadRequest("Invalid version number.")
	}
	return v, nil
}

func (s *Server) adminVersion(c *gin.Context) error {
	v, err := versionParam(c)
	if err != nil {
		return err
	}
	ver, err := s.store.ChapterVersion(c, c.Param("id"), v)
	if err != nil {
		return storeErr(err, "Version")
	}
	return httpx.OK(c, gin.H{"version": ver})
}

func (s *Server) adminRestoreVersion(c *gin.Context) error {
	v, err := versionParam(c)
	if err != nil {
		return err
	}
	ch, err := s.store.RestoreVersion(c, c.Param("id"), v, currentAdmin(c).Name)
	if err != nil {
		return storeErr(err, "Version")
	}
	return httpx.OK(c, gin.H{"chapter": ch})
}

func (s *Server) adminComments(c *gin.Context) error {
	list, err := s.store.ChapterComments(c, c.Param("id"))
	if err != nil {
		return err
	}
	return httpx.OK(c, gin.H{"comments": list})
}

func (s *Server) adminAddComment(c *gin.Context) error {
	var in struct {
		Body  string `json:"body" binding:"required,max=2000"`
		Quote string `json:"quote" binding:"max=500"`
	}
	if err := httpx.Bind(c, &in); err != nil {
		return err
	}
	admin := currentAdmin(c)
	cm, err := s.store.AddChapterComment(c, c.Param("id"), admin.ID, admin.Name, strings.TrimSpace(in.Body), strings.TrimSpace(in.Quote))
	if err != nil {
		return storeErr(err, "Chapter")
	}
	return httpx.Created(c, gin.H{"comment": cm})
}

func (s *Server) adminResolveComment(c *gin.Context) error {
	var in struct {
		Resolved bool `json:"resolved"`
	}
	if err := httpx.Bind(c, &in); err != nil {
		return err
	}
	return orNoContent(c, storeErr(s.store.SetCommentResolved(c, c.Param("id"), in.Resolved), "Comment"))
}

func (s *Server) adminUpload(c *gin.Context) error {
	fh, err := c.FormFile("file")
	if err != nil {
		return httpx.Field("file", "Choose a file to upload.")
	}
	saved, err := s.media.Store(c, fh)
	if errors.Is(err, media.ErrUnsupported) || (err != nil && strings.HasPrefix(err.Error(), "file is too large")) {
		return httpx.Field("file", err.Error())
	}
	if err != nil {
		return err
	}
	admin := currentAdmin(c)
	name := uploadFileName(fh.Filename)
	upload, err := s.store.RecordUpload(c, store.MediaUpload{
		URL: saved.URL, Kind: saved.Kind, ContentType: saved.ContentType, FileName: name, SizeBytes: fh.Size, UploaderName: admin.Name,
	}, admin.ID)
	if err != nil {
		return err
	}
	return httpx.Created(c, gin.H{"url": saved.URL, "kind": saved.Kind, "upload": upload})
}

// uploadFileName keeps the base name of a client file name, for display.
func uploadFileName(name string) string {
	if i := strings.LastIndexAny(name, `/\`); i >= 0 {
		name = name[i+1:]
	}
	if name = strings.TrimSpace(name); len(name) > 255 {
		name = name[:255]
	}
	return name
}

// adminSignUpload starts a direct browser-to-Cloudinary upload (bulk
// uploads and large videos). mode "server" means: POST the file to /media.
func (s *Server) adminSignUpload(c *gin.Context) error {
	var in struct {
		ContentType string `json:"content_type" binding:"required"`
		SizeBytes   int64  `json:"size_bytes" binding:"required,min=1"`
	}
	if err := httpx.Bind(c, &in); err != nil {
		return err
	}
	kind, err := media.Check(in.ContentType, in.SizeBytes)
	if err != nil {
		return httpx.Field("file", err.Error())
	}
	direct, err := s.media.SignDirect(kind)
	if err != nil {
		return err
	}
	if direct == nil {
		return httpx.OK(c, gin.H{"mode": "server", "kind": kind})
	}
	return httpx.OK(c, gin.H{"mode": "direct", "kind": kind, "upload_url": direct.UploadURL, "fields": direct.Fields})
}

// adminRegisterUpload records a finished direct upload in the uploads list.
func (s *Server) adminRegisterUpload(c *gin.Context) error {
	var in struct {
		URL         string `json:"url" binding:"required,max=1000"`
		ContentType string `json:"content_type" binding:"required"`
		FileName    string `json:"file_name" binding:"max=1000"`
		SizeBytes   int64  `json:"size_bytes" binding:"required,min=1"`
	}
	if err := httpx.Bind(c, &in); err != nil {
		return err
	}
	kind, err := media.Check(in.ContentType, in.SizeBytes)
	if err != nil {
		return httpx.Field("file", err.Error())
	}
	url, ok := s.media.DirectURL(in.URL, kind)
	if !ok {
		return httpx.Field("url", "This file wasn't uploaded to Victory's media storage.")
	}
	contentType := in.ContentType
	if kind == "video" && strings.HasSuffix(url, ".mp4") {
		contentType = "video/mp4"
	}
	admin := currentAdmin(c)
	upload, err := s.store.RecordUpload(c, store.MediaUpload{
		URL: url, Kind: kind, ContentType: contentType, FileName: uploadFileName(in.FileName), SizeBytes: in.SizeBytes, UploaderName: admin.Name,
	}, admin.ID)
	if err != nil {
		return err
	}
	return httpx.Created(c, gin.H{"url": url, "kind": kind, "upload": upload})
}

func (s *Server) adminMedia(c *gin.Context) error {
	kind := c.Query("kind")
	if kind != "" && kind != "image" && kind != "video" {
		return httpx.BadRequest("Invalid kind.")
	}
	list, err := s.store.MediaUploads(c, kind, 200)
	if err != nil {
		return err
	}
	return httpx.OK(c, gin.H{"uploads": list})
}

// mediaUses lists the content that shows an uploaded file.
func (s *Server) mediaUses(c *gin.Context, u store.MediaUpload) ([]store.MediaUse, error) {
	key := media.FileKey(u.URL)
	if key == "" {
		return []store.MediaUse{}, nil
	}
	return s.store.MediaUses(c, key)
}

func (s *Server) adminMediaUses(c *gin.Context) error {
	u, err := s.store.MediaUploadByID(c, c.Param("id"))
	if err != nil {
		return storeErr(err, "Upload")
	}
	uses, err := s.mediaUses(c, u)
	if err != nil {
		return err
	}
	return httpx.OK(c, gin.H{"uses": uses})
}

// adminDeleteMedia deletes an upload from storage and from the list. A file
// still shown in lessons, trainings or questions is kept unless ?force=1.
func (s *Server) adminDeleteMedia(c *gin.Context) error {
	u, err := s.store.MediaUploadByID(c, c.Param("id"))
	if err != nil {
		return storeErr(err, "Upload")
	}
	if c.Query("force") != "1" {
		uses, err := s.mediaUses(c, u)
		if err != nil {
			return err
		}
		if len(uses) > 0 {
			return httpx.Conflict("media_in_use", "This file is still used in your content.")
		}
	}
	if err := s.media.DeleteUpload(c, u.URL, u.Kind); err != nil {
		return err
	}
	return orNoContent(c, s.store.DeleteMediaUpload(c, u.ID))
}

func orNoContent(c *gin.Context, err error) error {
	if err != nil {
		return err
	}
	return httpx.NoContent(c)
}
