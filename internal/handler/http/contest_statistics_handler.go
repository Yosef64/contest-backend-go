package http

import (
	"log"
	"net/http"
	"strconv"
	"victor-contest-go/internal/domain"
	"victor-contest-go/internal/usecase"

	"github.com/gin-gonic/gin"
)

type ContestStatisticsHandler struct {
	usecase usecase.ContestStatisticsUsecase
}

func NewContestStatisticsHandler(u usecase.ContestStatisticsUsecase) *ContestStatisticsHandler {
	return &ContestStatisticsHandler{usecase: u}
}

func (h *ContestStatisticsHandler) RegisterRoutes(rg *gin.RouterGroup) {
	rg.GET("/contest/:contest_id/statistics", h.GetContestStatistics)
	rg.GET("/contest/:contest_id/statistics/summary", h.GetContestSummary)
	rg.GET("/contest/:contest_id/statistics/students", h.GetStudentPerformances)
}

// GetContestStatistics handles GET /contest/:contest_id/statistics
func (h *ContestStatisticsHandler) GetContestStatistics(c *gin.Context) {
	contestID := c.Param("contest_id")
	if contestID == "" {
		c.JSON(http.StatusBadRequest, gin.H{
			"success": false,
			"message": "contest_id is required",
		})
		return
	}

	// Parse query parameters for filters
	filters := domain.StatisticsFilters{
		Gender: c.Query("gender"),
		City:   c.Query("city"),
		School: c.Query("school"),
		Grade:  c.Query("grade"),
	}

	// Get statistics
	stats, err := h.usecase.GetContestStatistics(contestID, filters)
	if err != nil {
		log.Printf("Error getting contest statistics: %v", err)
		c.JSON(http.StatusInternalServerError, gin.H{
			"success": false,
			"message": "Failed to get contest statistics",
			"error":   err.Error(),
		})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"data":    stats,
		"message": "Contest statistics retrieved successfully",
	})
}

// GetContestSummary handles GET /contest/:contest_id/statistics/summary
func (h *ContestStatisticsHandler) GetContestSummary(c *gin.Context) {
	contestID := c.Param("contest_id")
	if contestID == "" {
		c.JSON(http.StatusBadRequest, gin.H{
			"success": false,
			"message": "contest_id is required",
		})
		return
	}

	// Get summary statistics (no filters)
	stats, err := h.usecase.GetContestSummary(contestID)
	if err != nil {
		log.Printf("Error getting contest summary: %v", err)
		c.JSON(http.StatusInternalServerError, gin.H{
			"success": false,
			"message": "Failed to get contest summary",
			"error":   err.Error(),
		})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"data":    stats,
		"message": "Contest summary retrieved successfully",
	})
}

// GetStudentPerformances handles GET /contest/:contest_id/statistics/students
func (h *ContestStatisticsHandler) GetStudentPerformances(c *gin.Context) {
	contestID := c.Param("contest_id")
	if contestID == "" {
		c.JSON(http.StatusBadRequest, gin.H{
			"success": false,
			"message": "contest_id is required",
		})
		return
	}

	// Parse query parameters for filters
	filters := domain.StatisticsFilters{
		Gender: c.Query("gender"),
		City:   c.Query("city"),
		School: c.Query("school"),
		Grade:  c.Query("grade"),
	}

	// Parse pagination parameters
	page, err := strconv.Atoi(c.DefaultQuery("page", "1"))
	if err != nil || page < 1 {
		page = 1
	}

	pageSize, err := strconv.Atoi(c.DefaultQuery("page_size", "20"))
	if err != nil || pageSize < 1 || pageSize > 100 {
		pageSize = 20
	}

	// Get student performances
	performances, err := h.usecase.GetStudentPerformancesByContest(contestID, filters, page, pageSize)
	if err != nil {
		log.Printf("Error getting student performances: %v", err)
		c.JSON(http.StatusInternalServerError, gin.H{
			"success": false,
			"message": "Failed to get student performances",
			"error":   err.Error(),
		})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"data":    performances,
		"message": "Student performances retrieved successfully",
	})
}
