package http

import (
	"log"
	"net/http"
	"victor-contest-go/internal/domain"
	"victor-contest-go/internal/usecase"

	"github.com/gin-gonic/gin"
)

type ContestRegistrationHandler struct {
	usecase usecase.ContestRegistrationUsecase
}

func NewContestRegistrationHandler(u usecase.ContestRegistrationUsecase) *ContestRegistrationHandler {
	return &ContestRegistrationHandler{usecase: u}
}

func (h *ContestRegistrationHandler) RegisterRoutes(rg *gin.RouterGroup) {
	rg.POST("/", h.AddContestRegistration)
	rg.PUT("/:id", h.UpdateContestRegistration)
	rg.DELETE("/:id", h.DeleteContestRegistration)
	rg.GET("/check/:student_id/:contest_id", h.IsStudentRegisteredForContest)
	rg.GET("/isActive/:contest_id/:student_id", h.CheckStudentActiveInContest)
}

func (h *ContestRegistrationHandler) AddContestRegistration(c *gin.Context) {
	var registration domain.ContestRegistrationDto
	if err := c.ShouldBindJSON(&registration); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	log.Printf("student_id : %s, contest: %s",registration.StudentID,registration.ContestID)
	id, err := h.usecase.AddContestRegistration(registration)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{"id": id})
}

func (h *ContestRegistrationHandler) UpdateContestRegistration(c *gin.Context) {
	id := c.Param("id")
	var update domain.ContestRegistration
	if err := c.ShouldBindJSON(&update); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	err := h.usecase.UpdateContestRegistration(id, update)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"message": "success"})
}

func (h *ContestRegistrationHandler) DeleteContestRegistration(c *gin.Context) {
	id := c.Param("id")
	err := h.usecase.DeleteContestRegistration(id)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"message": "success"})
}

func (h *ContestRegistrationHandler) IsStudentRegisteredForContest(c *gin.Context) {
	studentID := c.Param("student_id")
	contestID := c.Param("contest_id")
	isRegistered, err := h.usecase.CheckRegistrationsByContestAndStudent(contestID, studentID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"is_registered": isRegistered})
}
func (h *ContestRegistrationHandler) CheckStudentActiveInContest(c *gin.Context) {
	conId, studId := c.Param("contest_id"), c.Param("student_id")
	isActive, err := h.usecase.CheckStudentActiveInContest(conId, studId)
	if err != nil {
		c.JSON(http.StatusConflict, gin.H{"errors": "The user is active"})
		return
	}
	c.JSON(http.StatusOK, gin.H{"ok": isActive})
}
