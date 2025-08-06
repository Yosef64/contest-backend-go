package domain

type Notification struct {
	ID          string `json:"id"           dynamodbav:"id"`
	RecipientID string `json:"recipient_id" dynamodbav:"recipient_id"`
	Title       string `json:"title"        dynamodbav:"title"`
	Message     string `json:"message"      dynamodbav:"message"`
	IsRead      bool   `json:"is_read"      dynamodbav:"is_read"`
	SentAt      string `json:"sent_at"      dynamodbav:"sent_at"`
	Type        string `json:"type"         dynamodbav:"type"`
}
