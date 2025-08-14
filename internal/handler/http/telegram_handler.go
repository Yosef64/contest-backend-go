package http

import (
	"net/http"
	"victor-contest-go/internal/usecase"

	"github.com/gin-gonic/gin"
	tgbotapi "github.com/go-telegram-bot-api/telegram-bot-api/v5"
)


type telegramHandler struct {
	usecase usecase.TelegramUsecase
	
}

func(t *telegramHandler) RegisterRoutes(rg *gin.RouterGroup){
	rg.POST("/webhook",t.Updater)
}
// Updater implements TelegramHandler.
func (t *telegramHandler) Updater(c *gin.Context) {
	var update tgbotapi.Update
	if err := c.ShouldBindJSON(&update);err != nil {
		c.JSON(http.StatusBadRequest,gin.H{"error":err.Error()})
		return
	}
	err := t.usecase.TakeUpdate(update)
	if err != nil {
		c.JSON(http.StatusInternalServerError,gin.H{"error":err.Error()})
		return
	}
	c.JSON(http.StatusOK,gin.H{"status":"ok"})
}

func NewTelegramHandler(usecase usecase.TelegramUsecase) *telegramHandler {
	return &telegramHandler{usecase: usecase}
}
