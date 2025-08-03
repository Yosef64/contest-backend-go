package http

import (
	"net/http"
	"strconv"
	"victor-contest-go/internal/domain"
	"victor-contest-go/internal/repository"
	"victor-contest-go/internal/usecase"

	"github.com/gin-gonic/gin"
)

type QuestionHandler struct {
	usecase usecase.QuestionUsecase
	imageRepo *repository.ImageRepository
}

func NewQuestionHandler(u usecase.QuestionUsecase,imgRepo *repository.ImageRepository) *QuestionHandler {
	return &QuestionHandler{usecase: u,imageRepo: imgRepo}
}

func (h *QuestionHandler) RegisterRoutes(rg *gin.RouterGroup) {
	rg.POST("/add", h.AddQuestion)
	rg.PATCH("/:id", h.UpdateQuestion)
	rg.DELETE("/delete/:id", h.DeleteQuestion)
	rg.GET("/", h.GetAllQuestions)
	rg.GET("/:id", h.GetQuestionByID)
}

func (h *QuestionHandler) AddQuestion(c *gin.Context) {
	var question domain.Question
	var err error

	// 1. Parse all form fields
	question.QuestionText = c.PostForm("question_text")
	question.Explanation = c.PostForm("explanation")
	question.Subject = c.PostForm("subject")
	question.Grade = c.PostForm("grade")
	question.Chapter = c.PostForm("chapter")
	question.MultipleChoice = c.PostFormArray("multiple_choice")
	answerStr := c.PostForm("answer")
	question.Answer, err = strconv.Atoi(answerStr)

	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid 'answer' format. Must be an integer."})
		return
	}

	fileHeader, err := c.FormFile("question_image")
	if err != nil && err != http.ErrMissingFile {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Failed to get image from form"})
		return
	}

	if fileHeader != nil {
		file, err := fileHeader.Open()
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to open uploaded file"})
			return
		}
		defer file.Close()

		imgURL, err := h.imageRepo.UploadImage(file,"questions")
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
			return
		}
		question.QuestionImg = imgURL
	}

	id, err := h.usecase.AddQuestion(question)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{"id": id, "message": "Question added successfully"})
}

func (h *QuestionHandler) UpdateQuestion(c *gin.Context) {
	id := c.Param("id")
	var update domain.Question
	if err := c.ShouldBindJSON(&update); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	err := h.usecase.UpdateQuestion(id, update)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"message": "success"})
}

func (h *QuestionHandler) DeleteQuestion(c *gin.Context) {
	id := c.Param("id")
	err := h.usecase.DeleteQuestion(id)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"message": "success"})
}

func (h *QuestionHandler) GetAllQuestions(c *gin.Context) {
	questions, err := h.usecase.GetAllQuestions()
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"questions": questions})
}

func (h *QuestionHandler) GetQuestionByID(c *gin.Context) {
	id := c.Param("id")
	question, err := h.usecase.GetQuestionByID(id)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"question": question})
} 