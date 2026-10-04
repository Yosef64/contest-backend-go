package api

import (
	"errors"
	"fmt"
	"strconv"
	"strings"

	"github.com/gin-gonic/gin"

	"victor-contest-go/internal/httpx"
	"victor-contest-go/internal/store"
)

// Question management for one contest or national exam set.

const maxExamQuestions = 200

// adminContestQuestions returns the exam's questions in order, with answers.
func (s *Server) adminContestQuestions(c *gin.Context) error {
	if _, err := s.store.Contest(c, c.Param("id")); err != nil {
		return storeErr(err, "Exam")
	}
	qs, err := s.store.ContestQuestions(c, c.Param("id"))
	if err != nil {
		return err
	}
	return httpx.OK(c, gin.H{"questions": qs})
}

// importItem is one question in an import. Subject and grade default to the
// exam's; the chapter defaults to "National exam <year>" (or "General").
type importItem struct {
	Text        string   `json:"text"`
	Options     []string `json:"options"`
	AnswerIndex *int     `json:"answer_index"`
	Explanation string   `json:"explanation"`
	Subject     string   `json:"subject"`
	Chapter     string   `json:"chapter"`
	Topic       string   `json:"topic"`
	Grade       string   `json:"grade"`
	ImageURL    string   `json:"image_url"`
}

// adminImportContestQuestions creates questions in the bank and appends them
// to the exam in one transaction: all rows are imported or none are.
func (s *Server) adminImportContestQuestions(c *gin.Context) error {
	var in struct {
		Questions []importItem `json:"questions" binding:"required,min=1,max=200"`
	}
	if err := httpx.Bind(c, &in); err != nil {
		return err
	}
	exam, err := s.store.Contest(c, c.Param("id"))
	if err != nil {
		return storeErr(err, "Exam")
	}
	chapter := "General"
	if exam.ExamYear != nil {
		chapter = fmt.Sprintf("National exam %d", *exam.ExamYear)
	}
	admin := currentAdmin(c).Name
	qs := make([]store.Question, 0, len(in.Questions))
	fields := map[string]string{}
	for i, item := range in.Questions {
		qi := questionInput{
			Text: item.Text, Options: item.Options, AnswerIndex: item.AnswerIndex, Explanation: item.Explanation,
			Subject: or(item.Subject, exam.Subject), Chapter: or(item.Chapter, chapter), Topic: strings.TrimSpace(item.Topic), Grade: or(item.Grade, exam.Grade),
			ImageURL: strings.TrimSpace(item.ImageURL),
		}
		if len(qi.Text) > 2000 || len(qi.Explanation) > 4000 || len(qi.Chapter) > 120 || len(qi.Topic) > 120 || len(qi.Options) > 6 {
			fields["questions["+strconv.Itoa(i)+"]"] = "Too long: text up to 2000 characters, up to 6 options."
			continue
		}
		if qi.ImageURL != "" && !strings.HasPrefix(qi.ImageURL, "https://") && !strings.HasPrefix(qi.ImageURL, "http://") {
			fields["questions["+strconv.Itoa(i)+"].image_url"] = "Use a full image link."
			continue
		}
		q, err := qi.toQuestion("questions[" + strconv.Itoa(i) + "].")
		var apiErr *httpx.Error
		if errors.As(err, &apiErr) {
			for k, v := range apiErr.Fields {
				fields[k] = v
			}
			continue
		}
		if err != nil {
			return err
		}
		q.CreatedBy = admin
		qs = append(qs, q)
	}
	if len(fields) > 0 {
		return httpx.Invalid(fields)
	}
	added, err := s.store.ImportContestQuestions(c, exam.ID, qs, maxExamQuestions)
	switch {
	case errors.Is(err, store.ErrInUse):
		return httpx.Conflict("questions_locked", "Students have already started this exam, so its questions can't change.")
	case errors.Is(err, store.ErrTooMany):
		return httpx.Field("questions", fmt.Sprintf("An exam can have at most %d questions.", maxExamQuestions))
	case err != nil:
		return storeErr(err, "Exam")
	}
	if _, err := s.pool.Exec(c, `INSERT INTO audit_events (admin_id, action, resource) VALUES ($1, 'exam_import', $2)`,
		currentAdmin(c).ID, exam.ID); err != nil {
		return err
	}
	updated, err := s.store.Contest(c, exam.ID)
	if err != nil {
		return err
	}
	return httpx.Created(c, gin.H{"created": added, "contest": updated})
}

func or(v, fallback string) string {
	if strings.TrimSpace(v) == "" {
		return fallback
	}
	return strings.TrimSpace(v)
}
