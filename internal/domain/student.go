package domain

type Student struct {
	ID                string                           `dynamodbav:"id"          json:"id"`
	TelegramID        string                           `dynamodbav:"telegram_id"  json:"telegram_id"`
	Name              string                           `dynamodbav:"name"        json:"name"`
	Age               string                           `dynamodbav:"age"         json:"age"`
	Grade             string                           `dynamodbav:"grade"       json:"grade"`
	School            string                           `dynamodbav:"school"      json:"school"`
	City              string                           `dynamodbav:"city"        json:"city"`
	Region            string                           `dynamodbav:"region"      json:"region"`
	ImgURL            string                           `dynamodbav:"imgurl"      json:"imgurl"`
	IsSuspended       bool                             `dynamodbav:"isSuspended" json:"isSuspended"`
	PhoneNumber       string                           `dynamodbav:"phoneNumber" json:"phoneNumber"`
	Badge             []string                         `dynamodbav:"badge"       json:"badge"`
	Gender            string                           `dynamodbav:"gender"      json:"gender"`
	IsPremium         bool                             `dynamodbav:"is_premium"    json:"is_premium"`
	ReadNotifications map[string]ReadNotificationModel `dynamodbav:"read_notifications"    json:"read_notifications"`
	DefaultScoreRange string                           `dynamodbav:"defaultScoreRange"    json:"defaultScoreRange"`
}

type ReadNotificationModel struct {
	Id        string `json:"id"`
	IsDeleted bool   `json:"is_deleted"`
}
type StudentProfilesStatisticsDto struct {
	TotalContests  int `json:"totalContests"`
	TotalQuestions int `json:"totalQuestions"`
	CorrectAnswers int `json:"correctAnswers"`
	Accuracy       int `json:"accuracy"`
	AverageTime    int `json:"averageTime"`
	Rank           int `json:"rank"`
}

type CategoryStat struct {
	Total    int     `json:"total"`
	Correct  int     `json:"correct"`
	Accuracy float64 `json:"accuracy"`
}

type PerformanceTrendPoint struct {
	Month     string  `json:"month"`
	Accuracy  float64 `json:"accuracy"`
	Questions int     `json:"questions"`
}

type UserStatistics struct {
	TotalContests    int                      `json:"total_contests"`
	TotalQuestions   int                      `json:"total_questions"`
	CorrectAnswers   int                      `json:"correct_answers"`
	Accuracy         float64                  `json:"accuracy"`
	AverageTime      float64                  `json:"average_time"`
	Subjects         map[string]*CategoryStat `json:"subjects"`
	Chapters         map[string]*CategoryStat `json:"chapters"`
	Grades           map[string]*CategoryStat `json:"grades"`
	PerformanceTrend []PerformanceTrendPoint  `json:"performance_trend"`
}

type StudentProfileAdminResponse struct {
	Student
	Payment            PaymentRequest `json:"payment"`
	ContestSubmissions []Submission   `json:"contestSubmissions"`
	TotalPoints        int64          `json:"totalPoints"`
}
