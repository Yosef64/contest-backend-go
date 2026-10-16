package api

import (
	"github.com/gin-gonic/gin"

	"victor-contest-go/internal/httpx"
)

// GET /students/:id/profile — what other students may see about a
// student: name, picture, level, school and region, and their study record
// (streak, badges, exams, rank). Never contact details, age, gender or
// answers.
func (s *Server) appStudentProfile(c *gin.Context) error {
	if _, err := s.me(c); err != nil {
		return err
	}
	st, err := s.store.StudentByID(c, c.Param("id"))
	if err != nil {
		return storeErr(err, "Student")
	}
	if st.IsSuspended {
		return httpx.NotFound("Student")
	}
	academic, err := s.academicProfile(c, st.ID)
	if err != nil {
		return err
	}
	stats, err := s.store.StudentStats(c, st.ID)
	if err != nil {
		return err
	}
	a, err := s.achievements(c, st.ID)
	if err != nil {
		return err
	}
	return httpx.OK(c, gin.H{
		"student": gin.H{
			"id":         st.ID,
			"name":       st.Name,
			"photo_url":  st.PhotoURL,
			"grade":      st.Grade,
			"department": academic["department"],
			"stream":     academic["stream"],
			"school":     st.School,
			"region":     st.Region,
			"joined":     st.CreatedAt,
		},
		"stats": gin.H{
			"exams":              stats.Contests,
			"accuracy":           stats.Accuracy,
			"global_rank":        stats.GlobalRank,
			"chapters_completed": stats.ChaptersCompleted,
		},
		"streak":      a["streak"],
		"best_streak": a["best_streak"],
		"badges":      a["badges"],
	})
}
