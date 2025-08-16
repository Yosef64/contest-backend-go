package http

import (
	"net/http"
	"strconv"
	"victor-contest-go/internal/domain"
	"victor-contest-go/internal/usecase"

	"github.com/gin-gonic/gin"
)

type PageViewHandler struct {
	usecase usecase.PageViewUsecase
}

func NewPageViewHandler(u usecase.PageViewUsecase) *PageViewHandler {
	return &PageViewHandler{usecase: u}
}

func (h *PageViewHandler) RegisterRoutes(rg *gin.RouterGroup) {
	rg.POST("/track", h.TrackPageView)
	rg.GET("/stats", h.GetPageViewStats)
}

func (h *PageViewHandler) TrackPageView(c *gin.Context) {
	var pageView domain.PageView
	if err := c.ShouldBindJSON(&pageView); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	// Get additional data from request
	pageView.IPAddress = c.ClientIP()
	pageView.UserAgent = c.GetHeader("User-Agent")
	pageView.Referrer = c.GetHeader("Referer")

	err := h.usecase.TrackPageView(pageView)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{"message": "Page view tracked successfully"})
}

func (h *PageViewHandler) GetPageViewStats(c *gin.Context) {
	// Get days parameter, default to 30
	daysStr := c.DefaultQuery("days", "30")
	days, err := strconv.Atoi(daysStr)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid days parameter"})
		return
	}

	stats, err := h.usecase.GetPageViewStats(days)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{"stats": stats})
}
