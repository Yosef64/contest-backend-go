package domain

import "time"

// FeedbackQuestion represents a feedback question created by admin
type FeedbackQuestion struct {
	ID        string    `json:"id"          dynamodbav:"id"`
	AdminID   string    `json:"admin_id"    dynamodbav:"admin_id"`
	Question  string    `json:"question"    dynamodbav:"question"`
	Options   []string  `json:"options"     dynamodbav:"options"`
	IsActive  bool      `json:"is_active"   dynamodbav:"is_active"`
	CreatedAt time.Time `json:"created_at"  dynamodbav:"created_at"`
	UpdatedAt time.Time `json:"updated_at"  dynamodbav:"updated_at"`
}

// PollOption represents poll options for high-scoring students
type PollOption struct {
	ID              string    `json:"id"              dynamodbav:"id"`
	Label           string    `json:"label"           dynamodbav:"label"`
	MinScore        int       `json:"min_score"       dynamodbav:"min_score"`
	MaxScore        int       `json:"max_score"       dynamodbav:"max_score"`
	RequiresContact bool      `json:"requires_contact" dynamodbav:"requires_contact"`
	CreatedAt       time.Time `json:"created_at"      dynamodbav:"created_at"`
	UpdatedAt       time.Time `json:"updated_at"      dynamodbav:"updated_at"`
}

// ContactInfo represents contact information for high-scoring students
type ContactInfo struct {
	PhoneNumber string `json:"phone_number" dynamodbav:"phone_number"`
	Language    string `json:"language"     dynamodbav:"language"`
	Score       int    `json:"score"        dynamodbav:"score"`
}

// QuestionResponse represents a student's response to a specific question
type QuestionResponse struct {
	QuestionID     string `json:"question_id"     dynamodbav:"question_id"`
	SelectedOption string `json:"selected_option" dynamodbav:"selected_option"`
}

// FeedbackResponse represents a student's complete feedback response
type FeedbackResponse struct {
	ID                string                      `json:"id"                dynamodbav:"id"`
	StudentID         string                      `json:"student_id"        dynamodbav:"student_id"`
	StudentName       string                      `json:"student_name"      dynamodbav:"student_name"`
	Comment           string                      `json:"comment"           dynamodbav:"comment"`
	PollResponse      string                      `json:"poll_response"     dynamodbav:"poll_response"`
	ContactInfo       *ContactInfo                `json:"contact_info"      dynamodbav:"contact_info"`
	QuestionResponses map[string]QuestionResponse `json:"question_responses" dynamodbav:"question_responses"`
	SubmittedAt       time.Time                   `json:"submitted_at"      dynamodbav:"submitted_at"`
}

// AnalyticsData represents the complete analytics data for feedback
type AnalyticsData struct {
	TotalResponses int               `json:"total_responses"`
	QuestionStats  []QuestionStat    `json:"question_stats"`
	PollStats      []PollStat        `json:"poll_stats"`
	ContactList    []ContactListItem `json:"contact_list"`
	CommentSummary CommentSummary    `json:"comment_summary"`
}

// QuestionStat represents statistics for a specific question
type QuestionStat struct {
	QuestionID string         `json:"question_id"`
	Question   string         `json:"question"`
	Responses  map[string]int `json:"responses"`
}

// PollStat represents statistics for poll options
type PollStat struct {
	OptionID   string  `json:"option_id"`
	Option     string  `json:"option"`
	Count      int     `json:"count"`
	Percentage float64 `json:"percentage"`
}

// ContactListItem represents a contact in the high scorers list
type ContactListItem struct {
	Name        string    `json:"name"`
	Score       int       `json:"score"`
	PhoneNumber string    `json:"phone_number"`
	SubmittedAt time.Time `json:"submitted_at"`
}

// CommentSummary represents summary statistics for comments
type CommentSummary struct {
	TotalComments int     `json:"total_comments"`
	AverageLength float64 `json:"average_length"`
}

// AnalyticsFilter represents filters for analytics queries
type AnalyticsFilter struct {
	TimeRange string `json:"time_range"` // "all", "7d", "30d", "90d"
	AdminID   string `json:"admin_id"`
}
