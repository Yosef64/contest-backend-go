package http

import (
	"fmt"
	"net/http"
	"reflect"
	"strings"
	"time"
	"victor-contest-go/internal/domain"
	"victor-contest-go/internal/usecase"

	"github.com/gin-gonic/gin"
)

type ContestHandler struct {
	usecase             usecase.ContestUsecase
	notificationService usecase.NotificationUsecase
}

func NewContestHandler(u usecase.ContestUsecase, notificationService usecase.NotificationUsecase) *ContestHandler {
	return &ContestHandler{
		usecase:             u,
		notificationService: notificationService,
	}
}

func (h *ContestHandler) RegisterRoutes(rg *gin.RouterGroup) {
	rg.POST("/add", h.AddContest)
	rg.PATCH("/:id", h.UpdateContest)
	rg.GET("/", h.GetAllContests)
	rg.GET("/:id", h.GetContestByID)
	rg.DELETE("/delete/:id", h.DeleteContest)
	rg.POST("/clone/:id", h.CloneContest)
	rg.POST("/announce/:id", h.AnnounceContest)
}

func (h *ContestHandler) AddContest(c *gin.Context) {
	var contest domain.Contest
	if err := c.ShouldBindJSON(&contest); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	id, err := h.usecase.AddContest(contest)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"id": id})
}

func (h *ContestHandler) UpdateContest(c *gin.Context) {
	id := c.Param("id")

	// Parse the raw JSON to handle partial updates
	var rawData map[string]interface{}
	if err := c.ShouldBindJSON(&rawData); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid JSON format: " + err.Error()})
		return
	}

	// Get the current contest to merge with updates
	currentContest, err := h.usecase.GetContestByID(id)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to get current contest: " + err.Error()})
		return
	}
	if currentContest == nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "Contest not found"})
		return
	}

	// Create update struct with only the fields that are provided
	update := domain.Contest{
		ID: id, // Always set the ID
	}

	// Define allowed fields for update
	allowedFields := map[string]string{
		"title":       "Title",
		"description": "Description",
		"start_time":  "StartTime",
		"end_time":    "EndTime",
		"subject":     "Subject",
		"grade":       "Grade",
		"prize":       "Prize",
		"status":      "Status",
		"type":        "Type",
	}

	// Process only the fields that are provided in the update
	for jsonField, structField := range allowedFields {
		if value, exists := rawData[jsonField]; exists && value != nil {
			// Convert interface{} to string safely
			if strValue, ok := value.(string); ok && strValue != "" {
				// Use reflection to set the field value
				reflect.ValueOf(&update).Elem().FieldByName(structField).SetString(strValue)
			}
		}
	}

	// Validate that at least one field was updated
	if reflect.DeepEqual(update, domain.Contest{ID: id}) {
		c.JSON(http.StatusBadRequest, gin.H{"error": "No valid fields provided for update"})
		return
	}

	// Preserve existing fields that weren't updated
	if update.Title == "" {
		update.Title = currentContest.Contest.Title
	}
	if update.Description == "" {
		update.Description = currentContest.Contest.Description
	}
	if update.StartTime == "" {
		update.StartTime = currentContest.Contest.StartTime
	}
	if update.EndTime == "" {
		update.EndTime = currentContest.Contest.EndTime
	}
	if update.Subject == "" {
		update.Subject = currentContest.Contest.Subject
	}
	if update.Grade == "" {
		update.Grade = currentContest.Contest.Grade
	}
	if update.Prize == "" {
		update.Prize = currentContest.Contest.Prize
	}
	if update.Status == "" {
		update.Status = currentContest.Contest.Status
	}
	if update.Type == "" {
		update.Type = currentContest.Contest.Type
	}

	// Preserve questions
	update.Questions = currentContest.Contest.Questions

	// Additional safety check: ensure questions are never empty if they existed before
	if len(currentContest.Contest.Questions) > 0 && len(update.Questions) == 0 {
		update.Questions = currentContest.Contest.Questions
	}

	if rawData["questions"] != nil {
		if questionsArray, ok := rawData["questions"].([]interface{}); ok {
			if len(questionsArray) == 0 && len(currentContest.Contest.Questions) > 0 {
				update.Questions = currentContest.Contest.Questions
			}
		}
	}

	// Perform the update
	err = h.usecase.UpdateContest(id, update)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to update contest: " + err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{"message": "Contest updated successfully"})
}

func (h *ContestHandler) GetAllContests(c *gin.Context) {
	contests, err := h.usecase.GetAllContests()
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"contests": contests})
}

func (h *ContestHandler) GetContestByID(c *gin.Context) {
	id := c.Param("id")
	contest, err := h.usecase.GetContestByID(id)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"contest": contest})
}

func (h *ContestHandler) DeleteContest(c *gin.Context) {
	id := c.Param("id")
	err := h.usecase.DeleteContest(id)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"message": "success"})
}

func (h *ContestHandler) AnnounceContest(c *gin.Context) {
	id := c.Param("id")

	// Parse the announce request data - handle both JSON and form data
	var announceRequest struct {
		Message string `json:"message" form:"message"`
		File    string `json:"file" form:"file"` // File path or URL if file was uploaded
	}
	// Try to bind JSON first, then form data if JSON fails
	if err := c.ShouldBindJSON(&announceRequest); err != nil {
		// If JSON binding fails, try form data
		if err := c.ShouldBind(&announceRequest); err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid request format. Expected JSON or form data: " + err.Error()})
			return
		}
	}

	// Validate required fields
	if announceRequest.Message == "" || strings.TrimSpace(announceRequest.Message) == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Message is required for contest announcement and cannot be empty or whitespace"})
		return
	}

	// Get the contest to announce
	contest, err := h.usecase.GetContestByID(id)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to get contest: " + err.Error()})
		return
	}
	if contest == nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "Contest not found"})
		return
	}

	// Create announcement data
	announcementData := map[string]interface{}{
		"contest_id":    id,
		"contest_title": contest.Contest.Title,
		"message":       announceRequest.Message,
		"file":          announceRequest.File,
		"announced_at":  time.Now().Format(time.RFC3339),
	}

	// Send notifications to all students
	if h.notificationService != nil {
		message := fmt.Sprintf("New contest is announce for grade %s", contest.Grade)
		title := "New contest added"
		recepientId := "all"
		Type := "contest_announcement"
		h.notificationService.SendNotification(title, message, Type, recepientId)
	}

	c.JSON(http.StatusOK, gin.H{
		"message":      "Contest announced successfully",
		"announcement": announcementData,
	})
}

func (h *ContestHandler) CloneContest(c *gin.Context) {
	id := c.Param("id")

	// Parse the clone request data
	var cloneRequest struct {
		Title       string `json:"title"`
		Description string `json:"description"`
	}

	if err := c.ShouldBindJSON(&cloneRequest); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid JSON format: " + err.Error()})
		return
	}

	// Validate required fields
	if cloneRequest.Title == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Title is required for cloned contest"})
		return
	}

	// Clone the contest using the usecase
	clonedContestID, err := h.usecase.CloneContest(id, cloneRequest.Title, cloneRequest.Description)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to clone contest: " + err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"message":         "Contest cloned successfully",
		"clonedContestId": clonedContestID,
	})
}
