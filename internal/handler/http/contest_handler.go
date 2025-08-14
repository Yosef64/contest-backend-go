package http

import (
	"fmt"
	"log"
	"net/http"
	"reflect"
	"victor-contest-go/internal/domain"
	"victor-contest-go/internal/usecase"

	"github.com/gin-gonic/gin"
)

type ContestHandler struct {
	usecase             usecase.ContestUsecase
	notificationService *usecase.NotificationService
}

func NewContestHandler(u usecase.ContestUsecase, notificationService *usecase.NotificationService) *ContestHandler {
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
	rg.POST("/announce/:id", h.AnnounceContest)
	rg.POST("/clone/:id", h.CloneContest)
}

func (h *ContestHandler) AddContest(c *gin.Context) {
	var contest domain.Contest
	if err := c.ShouldBindJSON(&contest); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	log.Printf("contest %s", contest)
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
		fmt.Printf("WARNING: Questions were lost during update preparation! Restoring from current contest.\n")
		update.Questions = currentContest.Contest.Questions
	}

	// Final safety check: if the update contains questions field but it's empty,
	// and we had questions before, preserve the original questions
	if rawData["questions"] != nil {
		if questionsArray, ok := rawData["questions"].([]interface{}); ok {
			if len(questionsArray) == 0 && len(currentContest.Contest.Questions) > 0 {
				fmt.Printf("WARNING: Empty questions array received in update, preserving original questions.\n")
				update.Questions = currentContest.Contest.Questions
			}
		}
	}

	fmt.Printf("Final update struct - Questions count: %d, Questions: %+v\n", len(update.Questions), update.Questions)

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

	// Parse form data to get the announcement message
	message := c.PostForm("message")
	if message == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Announcement message is required"})
		return
	}

	// Handle optional file upload
	file, err := c.FormFile("file")
	if err != nil && err != http.ErrMissingFile {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid file upload: " + err.Error()})
		return
	}

	// Get the contest data using the ID from the URL
	contest, err := h.usecase.GetContestByID(id)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to get contest: " + err.Error()})
		return
	}

	if contest == nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "Contest not found"})
		return
	}

	// Log the announcement details
	log.Printf("Announcing contest %s with message: %s, file: %v", id, message, file != nil)

	// Send notifications to all students about the new contest
	if h.notificationService != nil {
		go func() {
			err := h.notificationService.SendContestAnnouncementNotification(contest.Contest)
			if err != nil {
				log.Printf("Failed to send contest announcement notifications: %v", err)
			}
		}()
	}

	c.JSON(http.StatusOK, gin.H{"message": "Contest announced successfully"})
}

func (h *ContestHandler) CloneContest(c *gin.Context) {
	id := c.Param("id")

	// Parse the clone request data
	var cloneRequest struct {
		Title       string `json:"title" binding:"required"`
		Description string `json:"description" binding:"required"`
	}

	if err := c.ShouldBindJSON(&cloneRequest); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid clone request: " + err.Error()})
		return
	}

	// Get the original contest
	originalContest, err := h.usecase.GetContestByID(id)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to get original contest: " + err.Error()})
		return
	}

	if originalContest == nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "Original contest not found"})
		return
	}

	// Create new contest with cloned data
	clonedContest := domain.Contest{
		Title:       cloneRequest.Title,
		Description: cloneRequest.Description,
		StartTime:   originalContest.Contest.StartTime,
		EndTime:     originalContest.Contest.EndTime,
		Subject:     originalContest.Contest.Subject,
		Grade:       originalContest.Contest.Grade,
		Prize:       originalContest.Contest.Prize,
		Status:      "draft", // New contests start as drafts
		Type:        originalContest.Contest.Type,
		Questions:   originalContest.Contest.Questions, // Clone all questions
	}

	// Add the cloned contest
	newContestID, err := h.usecase.AddContest(clonedContest)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to clone contest: " + err.Error()})
		return
	}

	log.Printf("Contest %s cloned to new contest %s", id, newContestID)

	c.JSON(http.StatusOK, gin.H{
		"message":        "Contest cloned successfully",
		"new_contest_id": newContestID,
	})
}
