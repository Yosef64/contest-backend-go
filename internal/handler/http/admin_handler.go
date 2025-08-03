package http

import (
	"fmt"

	"net/http"
	"time"
	"victor-contest-go/internal/domain"
	"victor-contest-go/internal/usecase"

	"github.com/gin-gonic/gin"
	"github.com/golang-jwt/jwt/v5"
)

type AdminHandler struct {
	usecase usecase.AdminUsecase
}

func NewAdminHandler(u usecase.AdminUsecase) *AdminHandler {
	return &AdminHandler{usecase: u}
}

func (h *AdminHandler) RegisterRoutes(rg *gin.RouterGroup) {
	rg.POST("/register", h.AddAdmin)
	rg.PUT("/:id", h.UpdateAdmin)
	rg.DELETE("/:id", h.DeleteAdmin)
	rg.GET("/:id", h.GetAdminByID)
	rg.GET("/me",h.GetMe)
	rg.GET("/", h.GetAllAdmins)
	rg.POST("/login", h.SignIn)
}
func (h *AdminHandler) GetMe(c *gin.Context) {
	tokenString,err := c.Cookie("token")
	if err != nil {
		c.JSON(http.StatusUnauthorized,gin.H{"message":"unauthorized"})
	}
	claims := &domain.CustomClaims{}
	token, err := jwt.ParseWithClaims(tokenString, claims, func(token *jwt.Token) (interface{}, error) {
		if _, ok := token.Method.(*jwt.SigningMethodHMAC); !ok {
			return nil, fmt.Errorf("unexpected signing method: %v", token.Header["alg"])
		}
		return []byte("a-very-long-and-secret-string"), nil
	})

	if err != nil || !token.Valid {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "Unauthorized: Invalid token"})
		return
	}
	admin, err := h.usecase.GetAdminByEmail(claims.UserID)
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "User not found"})
		return
	}

	c.JSON(http.StatusOK, admin)
	
}

func (h *AdminHandler) AddAdmin(c *gin.Context) {
	var admin domain.Admin
	if err := c.ShouldBindJSON(&admin); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	id, err := h.usecase.AddAdmin(admin)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"id": id})
}

func (h *AdminHandler) UpdateAdmin(c *gin.Context) {
	id := c.Param("id")
	var update domain.Admin
	if err := c.ShouldBindJSON(&update); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	err := h.usecase.UpdateAdmin(id, update)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"message": "success"})
}

func (h *AdminHandler) DeleteAdmin(c *gin.Context) {
	id := c.Param("id")
	err := h.usecase.DeleteAdmin(id)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"message": "success"})
}

func (h *AdminHandler) GetAdminByID(c *gin.Context) {
	id := c.Param("id")
	admin, err := h.usecase.GetAdminByEmail(id)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"admin": admin})
}

func (h *AdminHandler) GetAllAdmins(c *gin.Context) {
	admins, err := h.usecase.GetAllAdmins()
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"admins": admins})
}

func (h *AdminHandler) SignIn(c *gin.Context) {
	var req struct {
		Email    string `json:"email"`
		Password string `json:"password"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	admin, err := h.usecase.SignIn(req.Email, req.Password)
	if err != nil || admin == nil {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "Invalid credentials"})
		return
	}
	claims := domain.CustomClaims{
		UserID: admin.Email,
		RegisteredClaims: jwt.RegisteredClaims{
			ExpiresAt: jwt.NewNumericDate(time.Now().Add(time.Hour * 24)), // Token is valid for 24 hours
			IssuedAt:  jwt.NewNumericDate(time.Now()),
			NotBefore: jwt.NewNumericDate(time.Now()),
			Issuer:    "my-auth-service",
		},
	}
	token := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
	tokenString, err := token.SignedString([]byte("a-very-long-and-secret-string"))
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Could not create token"})
		return
	}
	cookieMaxAge := 3600 * 24
	c.SetCookie(
		"token",
		tokenString,
		cookieMaxAge,
		"/",           
		"localhost",
		false,          
		true,          
	)

	c.JSON(http.StatusOK, gin.H{"message": "Login successful, cookie set"})
} 