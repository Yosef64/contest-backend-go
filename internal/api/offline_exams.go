package api

import (
	"errors"
	"time"

	"github.com/gin-gonic/gin"

	"victor-contest-go/internal/httpx"
	"victor-contest-go/internal/store"
)

// Offline past national exams. The app downloads a national exam set with
// its answer key while online, lets the student sit it without a connection
// (timing and scoring on the device), and uploads the attempt later. The
// server re-scores every upload; the device's score is only a preview.
//
// Only national practice sets qualify: they are past public papers whose
// answers are shown right after submitting anyway. Live contests and exams
// in exam mode are never sent with answers.

type offlineQuestion struct {
	ID          string   `json:"id"`
	Text        string   `json:"text"`
	Options     []string `json:"options"`
	AnswerIndex int      `json:"answer_index"`
	Explanation string   `json:"explanation"`
	Subject     string   `json:"subject"`
	Chapter     string   `json:"chapter"`
	Topic       string   `json:"topic"`
	Grade       string   `json:"grade"`
	ImageURL    string   `json:"image_url"`
	// Aligned with Options; empty when no option has a picture.
	OptionImages        []string `json:"option_images"`
	ExplanationImageURL string   `json:"explanation_image_url"`
}

// offlineContest checks the student may take this exam offline.
func (s *Server) offlineContest(c *gin.Context, co store.Contest, st store.Student) error {
	if !co.IsNational() {
		return httpx.Conflict("offline_unavailable", "Only past national exams can be taken offline.")
	}
	if err := s.checkLearningAccess(c, st, "contest", co.ID); err != nil {
		return err
	}
	if co.Stream != "" {
		academic, err := s.academicProfile(c, st.ID)
		if err != nil {
			return err
		}
		if mine, _ := academic["stream"].(string); mine != "" && mine != co.Stream {
			return httpx.Forbidden("This exam set is for the " + co.Stream + " science stream.")
		}
	}
	var examMode bool
	if err := s.pool.QueryRow(c, "SELECT EXISTS(SELECT 1 FROM exam_mode_settings WHERE contest_id=$1 AND enabled)", co.ID).Scan(&examMode); err != nil {
		return err
	}
	if examMode {
		return httpx.Conflict("offline_unavailable", "This exam runs in exam mode, so it must be taken online.")
	}
	return nil
}

// appOfflinePaper: GET /contests/:id/offline — the full paper with answers.
func (s *Server) appOfflinePaper(c *gin.Context) error {
	st, err := s.me(c)
	if err != nil {
		return err
	}
	co, err := s.store.Contest(c, c.Param("id"))
	if err != nil {
		return storeErr(err, "Exam")
	}
	if err = s.offlineContest(c, co, st); err != nil {
		return err
	}
	if len(co.QuestionIDs) == 0 {
		return httpx.Conflict("not_ready", "This exam isn't ready yet.")
	}
	qs, err := s.store.ContestQuestions(c, co.ID)
	if err != nil {
		return err
	}
	out := make([]offlineQuestion, len(qs))
	for i, q := range qs {
		out[i] = offlineQuestion{q.ID, q.Text, q.Options, q.AnswerIndex, q.Explanation, q.Subject, q.Chapter, q.Topic, q.Grade, q.ImageURL,
			nonNilStrings(q.OptionImages), q.ExplanationImageURL}
	}
	return httpx.OK(c, gin.H{
		"contest_id":       co.ID,
		"duration_minutes": co.DurationMinutes,
		"available_until":  co.EndTime,
		"updated_at":       co.UpdatedAt,
		"questions":        out,
		// Answers come back in the original option order.
		"shuffle_questions": co.ShuffleQuestions,
		"shuffle_options":   co.ShuffleOptions,
	})
}

// appOfflineSubmit: POST /contests/:id/offline-submit — an attempt sat on the
// device. Times are the device's; they are clamped to what is possible.
func (s *Server) appOfflineSubmit(c *gin.Context) error {
	var in struct {
		Answers     map[string]int `json:"answers"`
		TimeSeconds int            `json:"time_seconds" binding:"min=0"`
		StartedAt   time.Time      `json:"started_at" binding:"required"`
		FinishedAt  time.Time      `json:"finished_at" binding:"required"`
	}
	if err := httpx.Bind(c, &in); err != nil {
		return err
	}
	st, co, err := s.contestForPlay(c)
	if err != nil {
		return err
	}
	if err = s.offlineContest(c, co, st); err != nil {
		return err
	}
	now := s.now()
	if in.StartedAt.After(now.Add(5*time.Minute)) || in.StartedAt.Before(co.StartTime.Add(-5*time.Minute)) {
		return httpx.Field("started_at", "The start time of this attempt is not valid.")
	}
	finished := in.FinishedAt
	if finished.Before(in.StartedAt) {
		finished = in.StartedAt
	}
	if finished.After(now) {
		finished = now
	}
	if finished.After(co.EndTime.Add(submitGrace)) {
		return httpx.Conflict("contest_ended", "This exam closed before the attempt was finished.")
	}
	limit := time.Duration(*co.DurationMinutes) * time.Minute
	maxSeconds := int(limit.Seconds())
	if elapsed := int(finished.Sub(in.StartedAt).Seconds()) + 60; maxSeconds > elapsed {
		maxSeconds = elapsed
	}
	if in.TimeSeconds > maxSeconds {
		in.TimeSeconds = maxSeconds
	}
	// Record the attempt like an online start, so the exam's questions lock
	// and the "started" count includes it.
	if _, err = s.pool.Exec(c, `INSERT INTO practice_attempts(contest_id, student_id, started_at, ends_at) VALUES($1, $2, $3, $4)
		ON CONFLICT (contest_id, student_id) DO NOTHING`, co.ID, st.ID, in.StartedAt, in.StartedAt.Add(limit)); err != nil {
		return err
	}
	qs, err := s.store.ContestQuestions(c, co.ID)
	if err != nil {
		return err
	}
	answers, correct := scoreAnswers(qs, in.Answers)
	sub := store.Submission{ContestID: co.ID, StudentID: st.ID, Answers: answers, Correct: correct, Total: len(qs), TimeSeconds: in.TimeSeconds, SubmittedAt: finished}
	if err := s.store.CreateSubmission(c, sub); err != nil {
		if errors.Is(err, store.ErrDuplicate) {
			return httpx.Conflict("already_submitted", "You have already submitted this exam.")
		}
		return err
	}
	return httpx.Created(c, gin.H{"submission": summarize(sub, true)})
}

// scoreAnswers keeps answers for known questions and options and counts the
// correct ones. Scores are always computed on the server.
func scoreAnswers(qs []store.Question, given map[string]int) (map[string]int, int) {
	answers := map[string]int{}
	correct := 0
	for _, q := range qs {
		a, ok := given[q.ID]
		if !ok || a < 0 || a >= len(q.Options) {
			continue
		}
		answers[q.ID] = a
		if a == q.AnswerIndex {
			correct++
		}
	}
	return answers, correct
}

func nonNilStrings(ss []string) []string {
	if ss == nil {
		return []string{}
	}
	return ss
}
