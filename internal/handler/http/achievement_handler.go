package http

import (
	"net/http"
	"victor-contest-go/internal/domain"
	"victor-contest-go/internal/usecase"

	"github.com/gin-gonic/gin"
)

type AchievementHandler struct {
	usecase usecase.AchievementUsecase
}

func NewAchievementHandler(u usecase.AchievementUsecase) *AchievementHandler {
	return &AchievementHandler{usecase: u}
}

func (h *AchievementHandler) RegisterRoutes(rg *gin.RouterGroup) {
	rg.POST("/", h.AddAchievement)
	rg.PUT("/:id", h.UpdateAchievement)
	rg.DELETE("/:id", h.DeleteAchievement)
	rg.GET("/", h.GetAllAchievements)
	rg.GET("/:id", h.GetAchievementByID)
	rg.GET("/student/:student_id", h.GetAchievementsByStudent)
}

func (h *AchievementHandler) AddAchievement(c *gin.Context) {
	var achievement domain.Achievement
	if err := c.ShouldBindJSON(&achievement); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	id, err := h.usecase.AddAchievement(achievement)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"id": id})
}

func (h *AchievementHandler) UpdateAchievement(c *gin.Context) {
	id := c.Param("id")
	var update domain.Achievement
	if err := c.ShouldBindJSON(&update); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	err := h.usecase.UpdateAchievement(id, update)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"message": "success"})
}

func (h *AchievementHandler) DeleteAchievement(c *gin.Context) {
	id := c.Param("id")
	err := h.usecase.DeleteAchievement(id)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"message": "success"})
}

func (h *AchievementHandler) GetAchievementByID(c *gin.Context) {
	id := c.Param("id")
	achievement, err := h.usecase.GetAchievementByID(id)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"achievement": achievement})
}

func (h *AchievementHandler) GetAllAchievements(c *gin.Context) {
	achievements, err := h.usecase.GetAllAchievements()
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"achievements": achievements})
}

func (h *AchievementHandler) GetAchievementsByStudent(c *gin.Context) {
	studentID := c.Param("student_id")
	achievements, err := h.usecase.GetAchievementsByStudent(studentID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"achievements": achievements})
} 