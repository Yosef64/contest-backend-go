package domain

import (
	"time"
)

type Submission struct {
    ID    string   `json:"id"     dynamodbav:"id"`
    ContestID       string   `json:"contest_id"        dynamodbav:"contest_id"`
    Student       StudentSub   `json:"student"        dynamodbav:"student"`
    Score           float64  `json:"score"             dynamodbav:"score"`
    MissedQuestions []SubmissionMissedQuestionDto `json:"missed_questions"  dynamodbav:"missed_questions"`
    SubmissionTime  time.Time   `json:"submission_time"   dynamodbav:"submission_time"`
    TimeSpend       string   `json:"time_spend"        dynamodbav:"time_spend"`
}
type SubmissionMissedQuestionDto struct{
	ID string `json:"id"`
	SelectedAnswer int `json:"selected_answer"`
}
type SubmissionDto struct{
	ContestID string `json:"contest_id"`
	Student StudentSub `json:"student"`
	Score float64 `json:"score"`
	MissedQuestions []SubmissionMissedQuestionDto `json:"missed_question"`
	TimeSpend string `json:"time_spend"`
}

type StudentSub struct {
    ID string `json:"id" dynamodbav:"submission_id"`
    ImgURL string `json:"imgurl" dynamodbav:"imgurl"`
    Name string    `json:"name" dynamodbav:"name"`
}
type LeaderboardEntry struct {
	Rank           int    `json:"rank"`
	UserID         string  `json:"user_id"`
	UserName       string `json:"user_name"`
	Score          int    `json:"score"`
	CorrectAnswers int    `json:"correct_answers"`
	TotalQuestions int    `json:"total_questions"`
	TimeTaken      string `json:"time_taken"` // Formatted as HH:MM:SS or MM:SS
	ImageURL       string `json:"imgurl"`
	TimeTakenSeconds int `json:"time_taken_seconds"`
}
type LeaderboardForContestEntry struct{
    UserId    string `json:"user_id"` 
    UserName  string  `json:"user_name"`
	Score       int   `json:"score"` 
    CorrectAnswers int	`json:"correct_answers"`
    TotalQuestions int `json:"total_questions"`
    TimeTaken string `json:"time_taken"`            
}
type Editorial struct {
	Question
	UserAnswer int `json:"user_answer"`
	IsCorrect  bool `json:"is_correct"`
}