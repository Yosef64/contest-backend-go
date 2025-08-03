package http

import (
	"log"
	"net/http"
	"victor-contest-go/internal/domain"
	"victor-contest-go/internal/usecase"

	"github.com/gin-gonic/gin"
)

type ContestHandler struct {
	usecase usecase.ContestUsecase
}

func NewContestHandler(u usecase.ContestUsecase) *ContestHandler {
	return &ContestHandler{usecase: u}
}

func (h *ContestHandler) RegisterRoutes(rg *gin.RouterGroup) {
	rg.POST("/add", h.AddContest)
	rg.PATCH("/:id", h.UpdateContest)
	rg.GET("/", h.GetAllContests)
	rg.GET("/:id", h.GetContestByID)
	rg.DELETE("/delete/:id", h.DeleteContest)
	rg.POST("/announce/:id",h.AnnounceContest)
}

func (h *ContestHandler) AddContest(c *gin.Context) {
	var contest domain.Contest
	if err := c.ShouldBindJSON(&contest); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	log.Printf("contest %s",contest)
	id, err := h.usecase.AddContest(contest)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"id": id})
}

func (h *ContestHandler) UpdateContest(c *gin.Context) {
	id := c.Param("id")
	var update domain.Contest
	if err := c.ShouldBindJSON(&update); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	err := h.usecase.UpdateContest(id, update)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"message": "success"})
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

func (h *ContestHandler) AnnounceContest(c *gin.Context){
	id:= c.Param("id")
	var contest domain.Contest
	if err := c.ShouldBindJSON(&contest);err != nil{
		c.JSON(http.StatusBadRequest,gin.H{"error":err.Error()})
	} 

	err := h.usecase.UpdateContest(id,contest)
	if err != nil {
		c.JSON(http.StatusInternalServerError,gin.H{"error":err.Error()})
	}

	c.JSON(http.StatusOK,gin.H{"message":"success"})
}