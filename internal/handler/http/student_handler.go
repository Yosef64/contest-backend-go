package http

import (
	"net/http"
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
	var student domain.Student
	if err := c.ShouldBindJSON(&student); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
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
