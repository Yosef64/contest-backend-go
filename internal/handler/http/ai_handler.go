package http

import (
	"net/http"
	"victor-contest-go/internal/domain"
	"victor-contest-go/internal/usecase"

	"github.com/gin-gonic/gin"
)

type AiHandler struct{
	usecase usecase.AiUsecase
}

func NewAiHandler(usecase usecase.AiUsecase) *AiHandler{
	return &AiHandler{usecase: usecase}
}
func (h *AiHandler) RegisterRoutes(rg *gin.RouterGroup) {
	rg.POST("/practice", h.Practice)
	rg.POST("/getRecommendation",h.GetRecommendation)
	
}

func(h *AiHandler) Practice(c *gin.Context){
	var setting domain.AiPracticeSetting
	if err := c.ShouldBindJSON(&setting);err != nil{
		c.JSON(http.StatusBadRequest,gin.H{"error":err.Error()})
		return
	}
	questions,err := h.usecase.PracticeWithAi(setting)
	if err != nil {
		c.JSON(http.StatusInternalServerError,gin.H{"error":err.Error()})
		return
	}
	c.JSON(http.StatusOK,gin.H{"questions":questions})
	return
}
func (h *AiHandler) GetRecommendation(c *gin.Context){
	var recommendationInput domain.RecommendationInput
	if err := c.ShouldBindJSON(&recommendationInput);err != nil{
		c.JSON(http.StatusBadRequest,gin.H{"error":err.Error()})
		return
	}
	recommendation,err := h.usecase.GenerateRecommendations(recommendationInput)
	if err != nil {
		c.JSON(http.StatusInternalServerError,gin.H{"error":err.Error()})
		return
	}
	c.JSON(http.StatusOK,gin.H{"recommendation":recommendation})
	return
}
