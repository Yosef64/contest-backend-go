package api

import (
	"errors"
	"strconv"
	"strings"
	"time"

	"github.com/gin-gonic/gin"

	"victor-contest-go/internal/content"
	"victor-contest-go/internal/httpx"
	"victor-contest-go/internal/store"
)

type questionInput struct {
	Text        string   `json:"text" binding:"required,max=2000"`
	Options     []string `json:"options" binding:"required,min=2,max=6"`
	AnswerIndex *int     `json:"answer_index" binding:"required"`
	Explanation string   `json:"explanation" binding:"max=4000"`
	Subject     string   `json:"subject" binding:"required"`
	Chapter     string   `json:"chapter" binding:"required,max=120"`
	// Optional finer level under the chapter, used in school reports.
	Topic    string `json:"topic" binding:"max=120"`
	Grade string `json:"grade" binding:"required"`
	// "natural" or "social" for grade 11 and up; empty means both streams.
	Stream   string `json:"stream"`
	ImageURL string `json:"image_url" binding:"omitempty,url"`
}

// questionLevel normalizes a question's grade (9-12) or university level
// (remedial, freshman); empty when it isn't one.
func questionLevel(g string) string {
	if l := strings.ToLower(strings.TrimSpace(g)); l == "remedial" || l == "freshman" {
		return l
	}
	return NormalizeGrade(g)
}

// questionStream checks a stream for the level: grades 9 and 10 have none,
// higher levels take natural, social or empty (both).
func questionStream(level, stream string) (string, bool) {
	stream = strings.ToLower(strings.TrimSpace(stream))
	switch {
	case level == "9" || level == "10":
		return "", true
	case stream == "" || stream == "natural" || stream == "social":
		return stream, true
	}
	return "", false
}

func (in *questionInput) toQuestion(prefix string) (store.Question, error) {
	opts := trimAll(in.Options)
	fields := map[string]string{}
	if len(opts) < 2 || len(opts) != len(in.Options) {
		fields[prefix+"options"] = "Fill in at least two options and remove empty ones."
	}
	if in.AnswerIndex == nil || *in.AnswerIndex < 0 || *in.AnswerIndex >= len(in.Options) {
		fields[prefix+"answer_index"] = "Choose the correct option."
	}
	grade := questionLevel(in.Grade)
	if grade == "" {
		fields[prefix+"grade"] = "Choose grade 9, 10, 11, 12, remedial or freshman."
	}
	stream, ok := questionStream(grade, in.Stream)
	if !ok {
		fields[prefix+"stream"] = "Choose Natural, Social or both streams."
	}
	if !subjects[in.Subject] || in.Subject == "Mixed" {
		fields[prefix+"subject"] = "Choose a supported subject."
	}
	if strings.TrimSpace(in.Text) == "" {
		fields[prefix+"text"] = "This field is required."
	}
	if len(fields) > 0 {
		return store.Question{}, httpx.Invalid(fields)
	}
	return store.Question{
		Text: content.SanitizeText(strings.TrimSpace(in.Text)), Options: opts, AnswerIndex: *in.AnswerIndex, Explanation: content.SanitizeText(strings.TrimSpace(in.Explanation)),
		Subject: in.Subject, Chapter: strings.TrimSpace(in.Chapter), Topic: strings.TrimSpace(in.Topic), Grade: grade, Stream: stream, ImageURL: in.ImageURL,
	}, nil
}

func (s *Server) adminQuestions(c *gin.Context) error {
	grade := ""
	if g := c.Query("grade"); g != "" {
		grade = questionLevel(g)
	}
	stream, _ := questionStream(grade, c.Query("stream"))
	from, until, err := reportDateRange(c.Query("from"), c.Query("to"))
	if err != nil {
		return err
	}
	list, err := s.store.Questions(c, c.Query("subject"), grade, stream, strings.TrimSpace(c.Query("q")), from, until)
	if err != nil {
		return err
	}
	return httpx.OK(c, gin.H{"questions": list})
}

func (s *Server) adminCreateQuestion(c *gin.Context) error {
	var in questionInput
	if err := httpx.Bind(c, &in); err != nil {
		return err
	}
	q, err := in.toQuestion("")
	if err != nil {
		return err
	}
	q.CreatedBy = currentAdmin(c).Name
	out, err := s.store.CreateQuestion(c, q)
	if err != nil {
		return err
	}
	return httpx.Created(c, gin.H{"question": out})
}

func (s *Server) adminBulkQuestions(c *gin.Context) error {
	var in struct {
		Questions []questionInput `json:"questions" binding:"required,min=1,max=500,dive"`
	}
	if err := httpx.Bind(c, &in); err != nil {
		return err
	}
	qs := make([]store.Question, 0, len(in.Questions))
	for i := range in.Questions {
		q, err := in.Questions[i].toQuestion("questions[" + strconv.Itoa(i) + "].")
		if err != nil {
			return err
		}
		q.CreatedBy = currentAdmin(c).Name
		qs = append(qs, q)
	}
	n, err := s.store.CreateQuestions(c, qs)
	if err != nil {
		return err
	}
	return httpx.Created(c, gin.H{"created": n})
}

func (s *Server) adminUpdateQuestion(c *gin.Context) error {
	var in questionInput
	if err := httpx.Bind(c, &in); err != nil {
		return err
	}
	q, err := in.toQuestion("")
	if err != nil {
		return err
	}
	q.UpdatedBy = currentAdmin(c).Name
	out, err := s.store.UpdateQuestion(c, c.Param("id"), q)
	if err != nil {
		return storeErr(err, "Question")
	}
	return httpx.OK(c, gin.H{"question": out})
}

func (s *Server) adminDeleteQuestion(c *gin.Context) error {
	err := s.store.DeleteQuestion(c, c.Param("id"))
	if errors.Is(err, store.ErrInUse) {
		return httpx.Conflict("in_use", "This question is used in an exam. Remove it from the exam first.")
	}
	return orNoContent(c, storeErr(err, "Question"))
}

type contestInput struct {
	Title       string    `json:"title" binding:"required,max=160"`
	Description string    `json:"description" binding:"max=2000"`
	Subject     string    `json:"subject" binding:"required"`
	Grade       string    `json:"grade" binding:"required"`
	StartTime   time.Time `json:"start_time" binding:"required"`
	EndTime     time.Time `json:"end_time" binding:"required,gtfield=StartTime"`
	Prize       string    `json:"prize" binding:"max=300"`
	// At least one for contests; national exam sets may start empty and get
	// their questions by import.
	QuestionIDs []string `json:"question_ids" binding:"max=200"`
	Rules       string   `json:"rules" binding:"max=4000"`
	Scoring     string   `json:"scoring" binding:"max=2000"`
	// "national" marks a past national entrance exam set: students take it
	// as timed practice any time between start_time and end_time.
	Kind            string `json:"kind" binding:"omitempty,oneof=contest national"`
	ExamYear        *int   `json:"exam_year" binding:"omitempty,min=1990,max=2100"`
	Stream          string `json:"stream" binding:"omitempty,oneof=natural social"`
	DurationMinutes *int   `json:"duration_minutes" binding:"omitempty,min=1,max=600"`
}

func (in *contestInput) toContest() (store.Contest, error) {
	grade := NormalizeGrade(in.Grade)
	if in.Grade == "remedial" || in.Grade == "freshman" {
		grade = in.Grade
	}
	national := in.Kind == "national"
	switch {
	case grade == "":
		return store.Contest{}, httpx.Field("grade", "Choose grade 9, 10, 11, 12, remedial or freshman.")
	case !subjects[in.Subject]:
		return store.Contest{}, httpx.Field("subject", "Choose a supported subject.")
	case !national && in.EndTime.Sub(in.StartTime) > 24*time.Hour:
		return store.Contest{}, httpx.Field("end_time", "An exam can last at most 24 hours.")
	case !national && len(in.QuestionIDs) == 0:
		return store.Contest{}, httpx.Field("question_ids", "Choose at least one question.")
	case national && in.ExamYear == nil:
		return store.Contest{}, httpx.Field("exam_year", "Enter the year of the national exam.")
	case national && in.DurationMinutes == nil:
		return store.Contest{}, httpx.Field("duration_minutes", "Enter the time allowed in minutes.")
	}
	kind, year, stream, duration := "contest", (*int)(nil), in.Stream, (*int)(nil)
	if !national && (grade == "9" || grade == "10") {
		stream = ""
	}
	if national {
		kind, year, stream, duration = "national", in.ExamYear, in.Stream, in.DurationMinutes
	}
	seen := map[string]bool{}
	for _, id := range in.QuestionIDs {
		if seen[id] {
			return store.Contest{}, httpx.Field("question_ids", "Each question can be used once.")
		}
		seen[id] = true
	}
	return store.Contest{
		Title: strings.TrimSpace(in.Title), Description: strings.TrimSpace(in.Description), Subject: in.Subject, Grade: grade,
		StartTime: in.StartTime.UTC(), EndTime: in.EndTime.UTC(), Prize: strings.TrimSpace(in.Prize), QuestionIDs: in.QuestionIDs,
		Rules: strings.TrimSpace(in.Rules), Scoring: strings.TrimSpace(in.Scoring),
		Kind: kind, ExamYear: year, Stream: stream, DurationMinutes: duration,
	}, nil
}

func (s *Server) adminContests(c *gin.Context) error {
	list, err := s.store.Contests(c, "")
	if err != nil {
		return err
	}
	return httpx.OK(c, gin.H{"contests": list})
}

func (s *Server) adminContest(c *gin.Context) error {
	contest, err := s.store.Contest(c, c.Param("id"))
	if err != nil {
		return storeErr(err, "Exam")
	}
	qs, err := s.store.ContestQuestions(c, contest.ID)
	if err != nil {
		return err
	}
	return httpx.OK(c, gin.H{"contest": contest, "questions": qs})
}

func (s *Server) adminCreateContest(c *gin.Context) error {
	var in contestInput
	if err := httpx.Bind(c, &in); err != nil {
		return err
	}
	contest, err := in.toContest()
	if err != nil {
		return err
	}
	contest.CreatedBy = currentAdmin(c).Name
	out, err := s.store.CreateContest(c, contest)
	if errors.Is(err, store.ErrInUse) {
		return httpx.Field("question_ids", "One of the selected questions no longer exists.")
	}
	if err != nil {
		return err
	}
	return httpx.Created(c, gin.H{"contest": out})
}

func (s *Server) adminUpdateContest(c *gin.Context) error {
	var in contestInput
	if err := httpx.Bind(c, &in); err != nil {
		return err
	}
	contest, err := in.toContest()
	if err != nil {
		return err
	}
	out, err := s.store.UpdateContest(c, c.Param("id"), contest)
	if errors.Is(err, store.ErrInUse) {
		return httpx.Field("question_ids", "Students have already submitted, so the questions can't change.")
	}
	if err != nil {
		return storeErr(err, "Exam")
	}
	return httpx.OK(c, gin.H{"contest": out})
}

func (s *Server) adminDeleteContest(c *gin.Context) error {
	return orNoContent(c, storeErr(s.store.DeleteContest(c, c.Param("id")), "Exam"))
}

func (s *Server) adminStandings(c *gin.Context) error {
	rows, err := s.store.Standings(c, c.Param("id"))
	if err != nil {
		return err
	}
	return httpx.OK(c, gin.H{"standings": rows})
}

// People ----------------------------------------------------------------------

func (s *Server) adminStudents(c *gin.Context) error {
	grade := ""
	if g := c.Query("grade"); g != "" {
		grade = NormalizeGrade(g)
	}
	department, stream := c.Query("department"), c.Query("stream")
	if department != "" && department != "none" && department != "entrance" && department != "remedial" && department != "freshman" {
		return httpx.BadRequest("Invalid department.")
	}
	if stream != "" && stream != "natural" && stream != "social" {
		return httpx.BadRequest("Invalid stream.")
	}
	school := c.Query("school_id")
	if id, manager := managerSchool(c); manager {
		school = id
	}
	page, err := s.store.Students(c, strings.TrimSpace(c.Query("q")), grade, department, stream, school, c.Query("cursor"), 20)
	if err != nil {
		return err
	}
	return httpx.OK(c, page)
}

func (s *Server) adminStudent(c *gin.Context) error {
	d, err := s.store.AdminStudentDetail(c, c.Param("id"))
	if err != nil {
		return storeErr(err, "Student")
	}
	academic, err := s.academicProfile(c, d.Student.ID)
	if err != nil {
		return err
	}
	if school, manager := managerSchool(c); manager {
		if member, _ := academic["school_id"].(*string); member == nil || *member != school {
			return httpx.NotFound("Student")
		}
	}
	return httpx.OK(c, gin.H{"student": d.Student, "stats": d.Stats, "submissions": d.Submissions, "academic": academic})
}

func (s *Server) adminPatchStudent(c *gin.Context) error {
	var in struct {
		IsSuspended *bool `json:"is_suspended" binding:"required"`
	}
	if err := httpx.Bind(c, &in); err != nil {
		return err
	}
	return orNoContent(c, storeErr(s.store.SetStudentSuspended(c, c.Param("id"), *in.IsSuspended), "Student"))
}

func (s *Server) adminDeleteStudent(c *gin.Context) error {
	return orNoContent(c, storeErr(s.store.DeleteStudent(c, c.Param("id")), "Student"))
}

func (s *Server) adminAdmins(c *gin.Context) error {
	list, err := s.store.Admins(c)
	if err != nil {
		return err
	}
	return httpx.OK(c, gin.H{"admins": list})
}

func (s *Server) adminPatchAdmin(c *gin.Context) error {
	var in struct {
		IsApproved *bool `json:"is_approved" binding:"required"`
	}
	if err := httpx.Bind(c, &in); err != nil {
		return err
	}
	if c.Param("id") == currentAdmin(c).ID {
		return httpx.BadRequest("You can't change your own access.")
	}
	return orNoContent(c, storeErr(s.store.SetAdminApproved(c, c.Param("id"), *in.IsApproved), "Admin"))
}

func (s *Server) adminDeleteAdmin(c *gin.Context) error {
	if c.Param("id") == currentAdmin(c).ID {
		return httpx.BadRequest("You can't remove your own account.")
	}
	return orNoContent(c, storeErr(s.store.DeleteAdmin(c, c.Param("id")), "Admin"))
}
