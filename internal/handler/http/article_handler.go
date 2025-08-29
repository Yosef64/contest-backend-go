package http

import (
	"net/http"
	"strconv"
	"victor-contest-go/internal/domain"
	"victor-contest-go/internal/usecase"

	"github.com/gin-gonic/gin"
)

type ArticleHandler struct {
    uc *usecase.ArticleUsecase
}

func NewArticleHandler(uc *usecase.ArticleUsecase) *ArticleHandler {
    return &ArticleHandler{uc: uc}
}

func (h *ArticleHandler) Register(rg *gin.RouterGroup) {
    rg.GET("/articles", h.List)
    rg.GET("/articles/published", h.ListPublished)
    rg.GET("/articles/status/:status", h.ListByStatus)
    rg.GET("/articles/:id", h.GetByID)
    rg.GET("/articles/:id/comments", h.ListComments)
    rg.POST("/articles", h.Create)
    rg.POST("/articles/:id/comments", h.CreateComment)
    rg.PUT("/articles/:id", h.Update)
    rg.DELETE("/articles/:id", h.Delete)
    rg.PATCH("/articles/:id/status", h.ToggleStatus)
    rg.PATCH("/articles/:id/stats", h.UpdateStats)
}

func (h *ArticleHandler) List(c *gin.Context) {
    items, err := h.uc.List()
    if err != nil { c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()}); return }

    c.JSON(http.StatusOK, items)
}

func (h *ArticleHandler) ListByStatus(c *gin.Context) {
    status := c.Param("status")
    items, err := h.uc.GetByStatus(domain.ArticleStatus(status))
    if err != nil { c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()}); return }
    c.JSON(http.StatusOK, items)
}

func (h *ArticleHandler) ListPublished(c *gin.Context) {
    number := c.Query("number")
    
    items, err := h.uc.ListPublished()
    if err != nil { c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()}); return }
    if number != "" && len(items) > 0 {
        n, err := strconv.Atoi(number)
        if err == nil && n < len(items) {
            items = items[:n]
        }
    }
    c.JSON(http.StatusOK, gin.H{"articles": items})
}

func (h *ArticleHandler) GetByID(c *gin.Context) {
    id := c.Param("id")
    item, err := h.uc.GetByID(id)
    if err != nil { c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()}); return }
    if item == nil { c.JSON(http.StatusNotFound, gin.H{"error": "not found"}); return }
    c.JSON(http.StatusOK, gin.H{"article": item})
}

func (h *ArticleHandler) Create(c *gin.Context) {
    var in domain.Article
    if err := c.ShouldBindJSON(&in); err != nil { c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()}); return }
    id, err := h.uc.Create(in)
    if err != nil { c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()}); return }
    item, _ := h.uc.GetByID(id)
    c.JSON(http.StatusCreated, item)
}

func (h *ArticleHandler) Update(c *gin.Context) {
    id := c.Param("id")
    var in domain.Article
    if err := c.ShouldBindJSON(&in); err != nil { c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()}); return }
    if err := h.uc.Update(id, in); err != nil { c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()}); return }
    item, _ := h.uc.GetByID(id)
    c.JSON(http.StatusOK, item)
}

func (h *ArticleHandler) Delete(c *gin.Context) {
    id := c.Param("id")
    if err := h.uc.Delete(id); err != nil { c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()}); return }
    c.Status(http.StatusNoContent)
}

type statusPayload struct { Status domain.ArticleStatus `json:"status"` }

func (h *ArticleHandler) ToggleStatus(c *gin.Context) {
    id := c.Param("id")
    var p statusPayload
    if err := c.ShouldBindJSON(&p); err != nil { c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()}); return }
    if err := h.uc.ToggleStatus(id, p.Status); err != nil { c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()}); return }
    item, _ := h.uc.GetByID(id)
    c.JSON(http.StatusOK, item)
}

type statsPayload struct {
    Type   string `json:"type" binding:"required,oneof=view like"`
    Action string `json:"action" binding:"required,oneof=increment decrement"`
}

func (h *ArticleHandler) UpdateStats(c *gin.Context) {
    id := c.Param("id")
    var payload statsPayload
    if err := c.ShouldBindJSON(&payload); err != nil {
        c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
        return
    }

    var err error
    switch payload.Type {
    case "view":
        if payload.Action == "increment" {
            err = h.uc.IncrementView(id)
        } else {
            err = h.uc.DecrementView(id)
        }
    case "like":
        if payload.Action == "increment" {
            err = h.uc.IncrementLike(id)
        } else {
            err = h.uc.DecrementLike(id)
        }
    }

    if err != nil {
        c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
        return
    }

    c.JSON(http.StatusOK, gin.H{"message": "Stats updated successfully"})
}

func (h *ArticleHandler) ListComments(c *gin.Context) {
    articleID := c.Param("id")
    comments, err := h.uc.ListCommentsByArticleID(articleID)
    if err != nil {
        c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
        return
    }
    c.JSON(http.StatusOK, gin.H{"comments": comments})
}

func (h *ArticleHandler) CreateComment(c *gin.Context) {
    articleID := c.Param("id")
    var comment domain.Comment
    if err := c.ShouldBindJSON(&comment); err != nil {
        c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
        return
    }
    
    comment.ArticleID = articleID
    
    id, err := h.uc.CreateComment(comment)
    if err != nil {
        c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
        return
    }
    
    c.JSON(http.StatusCreated, gin.H{"id": id, "message": "Comment created successfully"})
}