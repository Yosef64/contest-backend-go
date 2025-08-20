package http

import (
	"fmt"
	"net/http"
	"strconv"
	"victor-contest-go/internal/domain"
	"victor-contest-go/internal/usecase"

	"github.com/gin-gonic/gin"
)

type StudentHandler struct {
	usecase             usecase.StudentUsecase
	notificationService usecase.NotificationUsecase
}

func NewStudentHandler(u usecase.StudentUsecase, notificationService usecase.NotificationUsecase) *StudentHandler {
	return &StudentHandler{usecase: u, notificationService: notificationService}
}

func (h *StudentHandler) RegisterRoutes(rg *gin.RouterGroup) {
	rg.POST("/", h.AddStudent)
	rg.PUT("/:id", h.UpdateStudent)
	rg.DELETE("/:id", h.DeleteStudent)
	rg.GET("/", h.GetStudents)
	rg.GET("/paid", h.GetPaidStudents)
	rg.GET("/quickstat/:id", h.GetQuickStat)
	rg.GET("/rank", h.GetStudentRankings)
	rg.GET("/rank/:contest_id", h.GetStudentRankingsByContest)
	rg.GET("/:id", h.GetStudentByID)
	rg.GET("/grades-and-schools", h.GetGradesAndSchools)
	rg.GET("/profile/:id", h.GetUserProfile)
	rg.GET("/profile-admin/:student_id", h.GetUserStatForAdmin)
}

func (h *StudentHandler) AddStudent(c *gin.Context) {
	var student domain.Student
	if err := c.ShouldBindJSON(&student); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	err := h.usecase.AddStudent(student)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	go func() {
		if h.notificationService != nil {
			h.notificationService.SendStudentRegistrationNotification(student)
		}
	}()
	student.IsPremium = false
	c.JSON(http.StatusOK, gin.H{"student": student})
}

func (h *StudentHandler) UpdateStudent(c *gin.Context) {
	id := c.Param("id")
	var student domain.Student
	if err := c.ShouldBindJSON(&student); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	// Set the ID from the URL parameter
	student.ID = id

	err := h.usecase.UpdateStudent(student)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"message": "ok"})
}

func (h *StudentHandler) GetStudents(c *gin.Context) {
	students, err := h.usecase.GetStudents()
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"students": students})
}

func (h *StudentHandler) GetPaidStudents(c *gin.Context) {
	students, err := h.usecase.GetPaidStudents()
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"students": students})
}

func (h *StudentHandler) GetQuickStat(c *gin.Context) {
	id := c.Param("id")
	stat, err := h.usecase.GetQuickStat(id)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"stat": stat})
}

func (h *StudentHandler) GetStudentRankings(c *gin.Context) {
	rankings, err := h.usecase.GetStudentRankings()
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"rankings": rankings})
}

func (h *StudentHandler) GetStudentRankingsByContest(c *gin.Context) {
	contestID := c.Param("contest_id")
	rankings, err := h.usecase.GetStudentRankingsByContest(contestID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"rankings": rankings})
}

func (h *StudentHandler) GetStudentByID(c *gin.Context) {
	id := c.Param("id")
	student, err := h.usecase.GetStudentByID(id)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	if student != nil && student.Badge == nil {
		student.Badge = make([]string, 0)
	}
	c.JSON(http.StatusOK, gin.H{"student": student})
}

func (h *StudentHandler) GetGradesAndSchools(c *gin.Context) {
	data, err := h.usecase.GetGradesAndSchools()
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, data)
}

func (h *StudentHandler) GetUserProfile(c *gin.Context) {
	id := c.Param("id")
	profile, err := h.usecase.GetUserProfile(id)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"user": profile})
}
func (r *StudentHandler) GetUserStatForAdmin(c *gin.Context) {
	studId := c.Param("student_id")
	profile, err := r.usecase.GetUserStatForAdmin(studId)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "something went wrong"})
		return
	}
	c.JSON(http.StatusOK, gin.H{"profile": profile})

}

func (h *StudentHandler) DeleteStudent(c *gin.Context) {
	id := c.Param("id")
	if id == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Student ID is required"})
		return
	}

	var student *domain.Student
	var err error

	// Check if the ID looks like a Telegram ID (numeric) or a UUID
	// If it's numeric, try to find by Telegram ID first
	if _, err := strconv.Atoi(id); err == nil {
		// It's numeric, try to find by Telegram ID
		fmt.Printf("Looking up student by Telegram ID: %s\n", id)
		student, err = h.usecase.GetStudentByTelegramID(id)
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to check if student exists: " + err.Error()})
			return
		}
		if student == nil {
			fmt.Printf("Student not found by Telegram ID, trying by regular ID: %s\n", id)
			// Try by regular ID as fallback
			student, err = h.usecase.GetStudentByID(id)
			if err != nil {
				c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to check if student exists: " + err.Error()})
				return
			}
		} else {
			fmt.Printf("Found student by Telegram ID: %s, database ID: %s\n", id, student.ID)
		}
	} else {
		// It's not numeric, try by regular ID
		fmt.Printf("Looking up student by regular ID: %s\n", id)
		student, err = h.usecase.GetStudentByID(id)
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to check if student exists: " + err.Error()})
			return
		}
	}

	if student == nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "Student not found"})
		return
	}

	// Delete the student using the actual database ID
	fmt.Printf("Deleting student with database ID: %s\n", student.ID)
	err = h.usecase.DeleteStudent(student.ID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to delete student: " + err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"message": "Student deleted successfully"})
}
