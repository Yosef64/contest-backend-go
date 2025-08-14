package http

import (
	"net/http"
	"victor-contest-go/internal/domain"
	"victor-contest-go/internal/usecase"

	"github.com/gin-gonic/gin"
)

type NotificationHandler struct {
	usecase usecase.NotificationUsecase
}

func NewNotificationHandler(u usecase.NotificationUsecase) *NotificationHandler {
	return &NotificationHandler{usecase: u}
}

func (h *NotificationHandler) RegisterRoutes(rg *gin.RouterGroup) {
	rg.POST("/", h.AddNotification)
	rg.PUT("/:id", h.UpdateNotification)
	rg.DELETE("/:id", h.DeleteNotification)
	rg.PATCH("/:id/read", h.MarkAsRead)
	rg.GET("/", h.GetNotificationsByRecipient)
	rg.GET("/:id", h.GetNotificationsByRecipient)
	rg.GET("/recipient/:recipient_id", h.GetNotificationsByRecipient)
	rg.GET("/admin/:admin_email", h.GetNotificationsByAdminEmail)
	rg.POST("/contest-announce", h.ContestAnnounce)
}

func (h *NotificationHandler) AddNotification(c *gin.Context) {
	var notification domain.Notification
	if err := c.ShouldBindJSON(&notification); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	id, err := h.usecase.AddNotification(notification)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"id": id})
}

func (h *NotificationHandler) UpdateNotification(c *gin.Context) {
	id := c.Param("id")
	var update domain.Notification
	if err := c.ShouldBindJSON(&update); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	err := h.usecase.UpdateNotification(id, update)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"message": "success"})
}

func (h *NotificationHandler) DeleteNotification(c *gin.Context) {
	id := c.Param("id")
	err := h.usecase.DeleteNotification(id)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"message": "success"})
}

func (h *NotificationHandler) MarkAsRead(c *gin.Context) {
	id := c.Param("id")
	err := h.usecase.MarkNotificationAsRead(id)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"message": "Notification marked as read"})
}

func (h *NotificationHandler) GetNotificationByID(c *gin.Context) {
	id := c.Param("id")

	notification, err := h.usecase.GetNotificationByID(id)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"notification": notification})
}

func (h *NotificationHandler) GetAllNotifications(c *gin.Context) {
	notifications, err := h.usecase.GetAllNotifications()
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"notifications": notifications})
}

func (h *NotificationHandler) GetNotificationsByRecipient(c *gin.Context) {
	recipientID := c.Param("recipient_id")
	notifications, err := h.usecase.GetNotificationsByRecipient(recipientID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{"notifications": notifications})
}

func (h *NotificationHandler) GetNotificationsByAdminEmail(c *gin.Context) {
	adminEmail := c.Param("admin_email")
	notifications, err := h.usecase.GetNotificationsByRecipient(adminEmail)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{"notifications": notifications})
}

func (h *NotificationHandler) ContestAnnounce(c *gin.Context) {
	var contest domain.Contest
	if err := c.ShouldBindJSON(&contest); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	if err := h.usecase.AnnounceContest(contest); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"status": "ok"})
}
