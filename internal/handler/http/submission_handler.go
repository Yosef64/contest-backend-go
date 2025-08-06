package http

import (
	"net/http"
	"victor-contest-go/internal/domain"
	"victor-contest-go/internal/usecase"

	"github.com/gin-gonic/gin"
)

type SubmissionHandler struct {
	usecase usecase.SubmissionUsecase
}

func NewSubmissionHandler(u usecase.SubmissionUsecase) *SubmissionHandler {
	return &SubmissionHandler{usecase: u}
}

func (h *SubmissionHandler) RegisterRoutes(rg *gin.RouterGroup) {
	rg.POST("/", h.AddSubmission)
	rg.GET("/", h.GetAllSubmissions)
	rg.GET("/contest/:contest_id", h.GetSubmissionsByContest)
	rg.GET("/student/:student_id", h.GetSubmissionsByStudent)
	rg.GET("/leaderboard", h.GetLeaderboardByTimeFrame)
	rg.GET("/rank/:conId", h.GetRankForContest)
	rg.GET("/:id", h.GetSubmissionByID)
	rg.GET("/editorial/:student_id", h.GetStudentEditorial)
	rg.GET("/statistics-profile/:student_id", h.GetStudentProfileStatistics)
	rg.GET("/statistics/:student_id", h.GetStudentStatisctis)
}

func (h *SubmissionHandler) AddSubmission(c *gin.Context) {
	var submission domain.SubmissionDto
	if err := c.ShouldBindJSON(&submission); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	id, err := h.usecase.AddSubmission(submission)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"id": id})
}

func (h *SubmissionHandler) GetAllSubmissions(c *gin.Context) {
	submissions, err := h.usecase.GetAllSubmissions()
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"submissions": submissions})
}

func (h *SubmissionHandler) GetStudentStatisctis(c *gin.Context) {
	userId := c.Param("student_id")
	userStat, err := h.usecase.GetStudentStatistics(userId)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"statistics": userStat})
}

func (h *SubmissionHandler) GetStudentProfileStatistics(c *gin.Context) {
	userId := c.Param("student_id")
	stat, err := h.usecase.GetStudentProfileStatistics(userId)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"stat": stat})
}

func (h *SubmissionHandler) GetSubmissionByID(c *gin.Context) {
	id := c.Param("id")
	submission, err := h.usecase.GetSubmissionByID(id)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"submission": submission})
}

func (h *SubmissionHandler) GetSubmissionsByContest(c *gin.Context) {
	contestID := c.Param("contest_id")
	submissions, err := h.usecase.GetSubmissionsByContest(contestID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"submissions": submissions})
}

func (h *SubmissionHandler) GetSubmissionsByStudent(c *gin.Context) {
	studentID := c.Param("student_id")
	submissions, err := h.usecase.GetSubmissionsByStudent(studentID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"submissions": submissions})
}
func (h *SubmissionHandler) GetLeaderboardByTimeFrame(c *gin.Context) {
	timeFrame := c.Query("timeFrame")
	leaderboard, err := h.usecase.GetLeaderboardByTimeFrame(timeFrame)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"leaderboard": leaderboard})
}
func (h *SubmissionHandler) GetRankForContest(c *gin.Context) {
	conId := c.Param("conId")

	rankings, err := h.usecase.GetRankingsForContest(conId)
	if err != nil {
		c.JSON(http.StatusNoContent, gin.H{"error": "Not found"})
	}

	c.JSON(http.StatusOK, gin.H{"rankings": rankings})

}
func (h *SubmissionHandler) GetStudentEditorial(c *gin.Context) {
	studId, conId := c.Param("student_id"), c.Query("contest_id")
	if conId == "" {
		c.JSON(http.StatusNotFound, gin.H{"message": "please specify the contest Id"})
		return
	}
	editorial, err := h.usecase.GetStudentEditorial(conId, studId)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"message": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"editorial": editorial})
}
