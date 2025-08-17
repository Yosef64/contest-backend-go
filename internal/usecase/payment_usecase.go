package usecase

import (
	"time"
	"victor-contest-go/internal/domain"
)

type PaymentUsecase interface {
	AddPayment(payment domain.PaymentRequest) error
	UpdatePaymentStatus(id string, status domain.PaymentStatus, reason string) error
	GetPaymentById(id string) (*domain.PaymentRequest, error)
	GetPaymentByStudent(studId string) ([]domain.PaymentRequest, error)
	GetExpiredPayment() ([]domain.PaymentRequest, error)
	GetPaymentByStatus(status domain.PaymentStatus) ([]domain.PaymentRequest, error)
	GetAllPayments() ([]domain.PaymentRequest, error)
}

type paymentUsecase struct {
	paymentRepo PaymentRepository
}

// GetAllPayments implements PaymentUsecase.
func (p *paymentUsecase) GetAllPayments() ([]domain.PaymentRequest, error) {
	payments, err := p.paymentRepo.ListAll()
	if err != nil {
		return nil, err
	}
	return payments, nil
}

// GetPaymentByStatus implements PaymentUsecase.
func (p *paymentUsecase) GetPaymentByStatus(status domain.PaymentStatus) ([]domain.PaymentRequest, error) {
	return p.paymentRepo.ListByStatus(status)
}

// AddPayment implements PaymentUsecase.
func (p *paymentUsecase) AddPayment(payment domain.PaymentRequest) error {
	payment.CreatedAt = time.Now().UTC()
	expDate := time.Now().UTC().AddDate(0, 1, 0)
	payment.ExpirationDate = &expDate
	return p.paymentRepo.Create(&payment)
}

func (p *paymentUsecase) GetExpiredPayment() ([]domain.PaymentRequest, error) {
	return p.paymentRepo.ListExpired(time.Now().In(time.Local))
}

// GetPaymentById implements PaymentUsecase.
func (p *paymentUsecase) GetPaymentById(id string) (*domain.PaymentRequest, error) {
	return p.paymentRepo.GetByID(id)
}

// GetPaymentByStudent implements PaymentUsecase.
func (p *paymentUsecase) GetPaymentByStudent(studId string) ([]domain.PaymentRequest, error) {
	payments, err := p.paymentRepo.ListByUser(studId)
	// log.Printf("The length of the payments :%s",len(payments))
	if err != nil {
		return nil, err
	}

	return payments, nil
}

// UpdatePayment implements PaymentUsecase.
func (p *paymentUsecase) UpdatePaymentStatus(id string, status domain.PaymentStatus, reason string) error {
	return p.paymentRepo.UpdateStatus(id, status, reason)
}

func NewPaymentUsecases(payRepo PaymentRepository) PaymentUsecase {
	return &paymentUsecase{paymentRepo: payRepo}
}
