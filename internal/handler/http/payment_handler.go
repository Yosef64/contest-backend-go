package http

import (
	"log"
	"net/http"
	"time"
	"victor-contest-go/internal/domain"
	"victor-contest-go/internal/repository"
	"victor-contest-go/internal/usecase"

	"github.com/gin-gonic/gin"
)

type PaymentHandler struct{
	usecase usecase.PaymentUsecase
	imgRepo  repository.ImageRepository
}

func  NewPaymentHandler(paymentUsecase usecase.PaymentUsecase,imgRepo repository.ImageRepository) *PaymentHandler{
	return &PaymentHandler{usecase: paymentUsecase,imgRepo: imgRepo}
}

func (h *PaymentHandler) RegisterRoutes(rg *gin.RouterGroup) {
	rg.GET("/",h.GetAllPayments)
	rg.POST("/update",h.UpdatePaymentStatus)
	rg.POST("/",h.CreatePayment)
	rg.GET("/getexpired",h.GetExpiredPayments)
	rg.GET("/withstatus",h.GetPaymentsWithStatus)
	rg.GET("/:user_id",h.GetByUserId)
}

func (h *PaymentHandler) GetAllPayments(c *gin.Context){
		payments,err := h.usecase.GetAllPayments()
		if err != nil {
			c.JSON(http.StatusInternalServerError,gin.H{"error":err.Error()})
			return
		}
		c.JSON(http.StatusOK,gin.H{"payments":payments})
}
func(h *PaymentHandler) UpdatePaymentStatus(c *gin.Context){
	var reason domain.PaymentReason
	 c.ShouldBindJSON(&reason)
	log.Printf(reason.Reason)
	paymentId, status := c.Query("payment_id"), c.Query("status")
	paymentStatus := domain.PaymentStatus(status)
	err := h.usecase.UpdatePaymentStatus(paymentId, paymentStatus,reason)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"message": "ok"})
	return
}

func (h *PaymentHandler) CreatePayment(c *gin.Context){
	userID := c.Request.PostFormValue("user_id")
	fullName := c.Request.PostFormValue("fullName")
	bankName := c.Request.PostFormValue("bankName")
	status := c.Request.PostFormValue("status")

	if userID == "" || fullName == "" || bankName == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "missing required form fields: user_id, fullName, bankName"})
		return
	}

	file, err := c.FormFile("img")
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "bill screenshot ('img' field) is required"})
		return
	}
	
	openedFile, openErr := file.Open()
	if openErr != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "failed to open uploaded file"})
		return
	}
	defer openedFile.Close()
	img_url, err := h.imgRepo.UploadImage(openedFile, "payments")

	expirationDate := time.Now().In(time.Local).AddDate(0,1,0)
	payment := domain.PaymentRequest{
		FullName: fullName,
		BankName: bankName,
		UserID: userID,
		BillScreenshotURL: img_url,
		Status: domain.PaymentStatus(status),
		CreatedAt: time.Now().In(time.Local),
		UpdatedAt: time.Now().In(time.Local),
		ExpirationDate: &expirationDate,
		RejectionReason: "",
	 }
	
	if err := h.usecase.AddPayment(payment); err != nil {
		c.JSON(http.StatusInternalServerError,gin.H{"error":err.Error()})
	}
	c.JSON(http.StatusOK,gin.H{"message":"ok"})
	return
}
func (h *PaymentHandler) GetExpiredPayments(c *gin.Context){
	payments,err := h.usecase.GetExpiredPayment()
	if err != nil {
		c.JSON(http.StatusBadRequest,gin.H{"error":err.Error()})
		return
	}
	c.JSON(http.StatusOK,gin.H{"payments":payments})
	return
}
func ( h *PaymentHandler) GetPaymentsWithStatus( c *gin.Context){
	status := c.Query("status")
	payments,err := h.usecase.GetPaymentByStatus(domain.PaymentStatus(status))
	if err != nil {
		c.JSON(http.StatusInternalServerError,gin.H{"error":err.Error()})
		return
	}
	if payments == nil {
		ps := make([]domain.PaymentRequest, 0)
		c.JSON(http.StatusOK,gin.H{"payments":ps})
		return
	}
	c.JSON(http.StatusOK,gin.H{"payments":payments})
	return
}
func (h *PaymentHandler) GetByUserId(c *gin.Context){
	userId := c.Param("user_id")
	payments,err := h.usecase.GetPaymentByStudent(userId)
	if err != nil {
		c.JSON(http.StatusInternalServerError,gin.H{"error":err.Error()})
		return
	}
	c.JSON(http.StatusOK,gin.H{"payments":payments})
	return
}