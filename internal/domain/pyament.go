package domain

import "time"

type PaymentRequest struct {
	ID string `json:"id" dynamodbav:"id"`
	UserID string `json:"user_id" dynamodbav:"user_id"`
	FullName string `json:"fullName" dynamodbav:"full_name"`
	BankName string `json:"bankName" dynamodbav:"bank_name"`
	BillScreenshotURL string `json:"billScreenshotUrl" dynamodbav:"bill_screenshot_url"`
	Status PaymentStatus `json:"status" dynamodbav:"status"`
	RejectionReason string `json:"rejectionReason,omitempty" dynamodbav:"rejection_reason"`
	CreatedAt time.Time `json:"createdAt" dynamodbav:"created_at"`
	UpdatedAt time.Time `json:"updatedAt" dynamodbav:"updated_at"`
	ExpirationDate    *time.Time    `dynamodbav:"expirationDate,omitempty"`
	GSI1PK            string     `dynamodbav:"GSI1PK,omitempty"` // New field


}

type PaymentStatus string

const (
	StatusPending PaymentStatus = "Pending"
	StatusApproved PaymentStatus = "Approved"
	StatusRejected PaymentStatus = "Rejected"
)

type PaymentReason struct{
	Reason string `json:"reason"`
}