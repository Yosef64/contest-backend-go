package http

import (
	"fmt"
	"net/http"
	"time"
	"victor-contest-go/internal/domain"
	"victor-contest-go/internal/usecase"

	"github.com/gin-gonic/gin"
)

// FeedbackQuestionHandler handles feedback question operations
type FeedbackQuestionHandler struct {
	usecase             usecase.FeedbackQuestionUsecase
	notificationService usecase.NotificationUsecase
}

func NewFeedbackQuestionHandler(u usecase.FeedbackQuestionUsecase, notificationService usecase.NotificationUsecase) *FeedbackQuestionHandler {
	return &FeedbackQuestionHandler{
		usecase:             u,
		notificationService: notificationService,
	}
}

func (h *FeedbackQuestionHandler) RegisterRoutes(rg *gin.RouterGroup) {
	rg.POST("/", h.AddFeedbackQuestion)
	rg.PUT("/:id", h.UpdateFeedbackQuestion)
	rg.DELETE("/:id", h.DeleteFeedbackQuestion)
	rg.GET("/", h.GetAllFeedbackQuestions)
	rg.GET("/active", h.GetActiveFeedbackQuestions)
	rg.GET("/:id", h.GetFeedbackQuestionByID)
	rg.GET("/admin/:admin_id", h.GetFeedbackQuestionsByAdmin)
}

func (h *FeedbackQuestionHandler) AddFeedbackQuestion(c *gin.Context) {
	var question domain.FeedbackQuestion
	if err := c.ShouldBindJSON(&question); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	id, err := h.usecase.AddFeedbackQuestion(question)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	// Send notifications to all students about the new feedback question
	if h.notificationService != nil {
		go func() {
			message := "Feadback questions are added. so everybody fill all the questions"
			title := "New feedback question"
			recepientId := "all"
			Type := "feedback_question"
			h.notificationService.SendNotification(title, message, Type, recepientId)

		}()
	}

	c.JSON(http.StatusOK, gin.H{"id": id})
}

func (h *FeedbackQuestionHandler) UpdateFeedbackQuestion(c *gin.Context) {
	id := c.Param("id")
	var update domain.FeedbackQuestion
	if err := c.ShouldBindJSON(&update); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	err := h.usecase.UpdateFeedbackQuestion(id, update)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"message": "success"})
}

func (h *FeedbackQuestionHandler) DeleteFeedbackQuestion(c *gin.Context) {
	id := c.Param("id")
	err := h.usecase.DeleteFeedbackQuestion(id)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"message": "success"})
}

func (h *FeedbackQuestionHandler) GetFeedbackQuestionByID(c *gin.Context) {
	id := c.Param("id")
	question, err := h.usecase.GetFeedbackQuestionByID(id)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"question": question})
}

func (h *FeedbackQuestionHandler) GetAllFeedbackQuestions(c *gin.Context) {
	questions, err := h.usecase.GetAllFeedbackQuestions()
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"questions": questions})
}

func (h *FeedbackQuestionHandler) GetActiveFeedbackQuestions(c *gin.Context) {
	questions, err := h.usecase.GetActiveFeedbackQuestions()
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"questions": questions})
}

func (h *FeedbackQuestionHandler) GetFeedbackQuestionsByAdmin(c *gin.Context) {
	adminID := c.Param("admin_id")
	questions, err := h.usecase.GetFeedbackQuestionsByAdmin(adminID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"questions": questions})
}

// PollOptionHandler handles poll option operations
type PollOptionHandler struct {
	usecase usecase.PollOptionUsecase
}

func NewPollOptionHandler(u usecase.PollOptionUsecase) *PollOptionHandler {
	return &PollOptionHandler{usecase: u}
}

func (h *PollOptionHandler) RegisterRoutes(rg *gin.RouterGroup) {
	rg.POST("/", h.AddPollOption)
	rg.PUT("/:id", h.UpdatePollOption)
	rg.DELETE("/:id", h.DeletePollOption)
	rg.GET("/", h.GetAllPollOptions)
	rg.GET("/:id", h.GetPollOptionByID)
	rg.GET("/score/:score", h.GetPollOptionByScore)
}

func (h *PollOptionHandler) AddPollOption(c *gin.Context) {
	var option domain.PollOption
	if err := c.ShouldBindJSON(&option); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	id, err := h.usecase.AddPollOption(option)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"id": id})
}

func (h *PollOptionHandler) UpdatePollOption(c *gin.Context) {
	id := c.Param("id")
	var update domain.PollOption
	if err := c.ShouldBindJSON(&update); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	err := h.usecase.UpdatePollOption(id, update)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"message": "success"})
}

func (h *PollOptionHandler) DeletePollOption(c *gin.Context) {
	id := c.Param("id")
	err := h.usecase.DeletePollOption(id)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"message": "success"})
}

func (h *PollOptionHandler) GetPollOptionByID(c *gin.Context) {
	id := c.Param("id")
	option, err := h.usecase.GetPollOptionByID(id)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"option": option})
}

func (h *PollOptionHandler) GetAllPollOptions(c *gin.Context) {
	options, err := h.usecase.GetAllPollOptions()
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"options": options})
}

func (h *PollOptionHandler) GetPollOptionByScore(c *gin.Context) {
	score := c.Param("score")
	// Convert score to int - you might want to add validation here
	var scoreInt int
	if _, err := fmt.Sscanf(score, "%d", &scoreInt); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid score format"})
		return
	}
	option, err := h.usecase.GetPollOptionByScore(scoreInt)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"option": option})
}

// FeedbackResponseHandler handles feedback response operations
type FeedbackResponseHandler struct {
	usecase             usecase.FeedbackResponseUsecase
	notificationService usecase.NotificationUsecase
}

func NewFeedbackResponseHandler(u usecase.FeedbackResponseUsecase, notificationService usecase.NotificationUsecase) *FeedbackResponseHandler {
	return &FeedbackResponseHandler{usecase: u, notificationService: notificationService}
}

func (h *FeedbackResponseHandler) RegisterRoutes(rg *gin.RouterGroup) {
	rg.POST("/", h.AddFeedbackResponse)
	rg.PUT("/:id", h.UpdateFeedbackResponse)
	rg.DELETE("/:id", h.DeleteFeedbackResponse)
	rg.DELETE("/response-only/:id", h.DeleteFeedbackResponseOnly)
	rg.GET("/", h.GetAllFeedbackResponses)
	rg.GET("/student/:student_id", h.GetFeedbackResponsesByStudent)
	rg.GET("/question/:question_id", h.GetFeedbackResponsesByQuestion)
	rg.GET("/analytics", h.GetFeedbackAnalytics)
	rg.GET("/test", h.TestEndpoint)
	rg.DELETE("/contact/:phone_number", h.DeleteContactByPhoneNumber)
	rg.GET("/:id", h.GetFeedbackResponseByID)
}

func (h *FeedbackResponseHandler) AddFeedbackResponse(c *gin.Context) {
	var response domain.FeedbackResponse
	if err := c.ShouldBindJSON(&response); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	id, err := h.usecase.AddFeedbackResponse(response)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	go func() {
		if h.notificationService != nil {
			message := fmt.Sprintf("%s sent a feedback response", response.StudentName)
			Type := "feedback_response"
			reciepientId := response.StudentID
			h.notificationService.SendNotification("New Feedback response", message, Type, reciepientId)
		}
	}()

	c.JSON(http.StatusOK, gin.H{"id": id})
}

func (h *FeedbackResponseHandler) UpdateFeedbackResponse(c *gin.Context) {
	id := c.Param("id")
	var update domain.FeedbackResponse
	if err := c.ShouldBindJSON(&update); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	err := h.usecase.UpdateFeedbackResponse(id, update)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"message": "success"})
}

func (h *FeedbackResponseHandler) DeleteFeedbackResponse(c *gin.Context) {
	id := c.Param("id")
	err := h.usecase.DeleteFeedbackResponse(id)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"message": "success"})
}

func (h *FeedbackResponseHandler) DeleteFeedbackResponseOnly(c *gin.Context) {
	id := c.Param("id")
	err := h.usecase.DeleteFeedbackResponseOnly(id)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"message": "success"})
}

func (h *FeedbackResponseHandler) GetFeedbackResponseByID(c *gin.Context) {
	id := c.Param("id")
	response, err := h.usecase.GetFeedbackResponseByID(id)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"response": response})
}

func (h *FeedbackResponseHandler) GetAllFeedbackResponses(c *gin.Context) {
	responses, err := h.usecase.GetAllFeedbackResponses()
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"responses": responses})
}

func (h *FeedbackResponseHandler) GetFeedbackResponsesByStudent(c *gin.Context) {
	studentID := c.Param("student_id")
	responses, err := h.usecase.GetFeedbackResponsesByStudent(studentID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"responses": responses})
}

func (h *FeedbackResponseHandler) GetFeedbackResponsesByQuestion(c *gin.Context) {
	questionID := c.Param("question_id")
	responses, err := h.usecase.GetFeedbackResponsesByQuestion(questionID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"responses": responses})
}

func (h *FeedbackResponseHandler) GetFeedbackAnalytics(c *gin.Context) {
	// Get query parameters
	timeRange := c.Query("range")
	if timeRange == "" {
		timeRange = "all"
	}
	adminID := c.Query("admin_id")

	filter := domain.AnalyticsFilter{
		TimeRange: timeRange,
		AdminID:   adminID,
	}

	analytics, err := h.usecase.GetFeedbackAnalytics(filter)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, analytics)
}

func (h *FeedbackResponseHandler) TestEndpoint(c *gin.Context) {
	c.JSON(http.StatusOK, gin.H{"message": "Test endpoint working", "timestamp": time.Now().Unix()})
}

func (h *FeedbackResponseHandler) DeleteContactByPhoneNumber(c *gin.Context) {
	phoneNumber := c.Param("phone_number")
	if phoneNumber == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "phone number is required"})
		return
	}

	err := h.usecase.DeleteContactByPhoneNumber(phoneNumber)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{"message": "Contact deleted successfully"})
}
