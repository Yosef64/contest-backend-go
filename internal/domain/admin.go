package domain

import "github.com/golang-jwt/jwt/v5"

type Admin struct {
	ID         string `json:"id"          dynamodbav:"id"`
	Email      string `json:"email"       dynamodbav:"email"`
	IsApproved bool   `json:"is_approved" dynamodbav:"is_approved"`
	Name       string `json:"name"        dynamodbav:"name"`
	Password   string `json:"password"    dynamodbav:"password"`
}

type CustomClaims struct {
	UserID string `json:"user_id"`
	jwt.RegisteredClaims
}

// Dashboard Response Data Structures

type DashboardStatsResponse struct {
	Overview       OverviewStats   `json:"overview" validate:"required"`
	UserStats      UserStats       `json:"user_stats" validate:"required"`
	ContestStats   ContestStats    `json:"contest_stats" validate:"required"`
	PageViewStats  PageViewStats   `json:"page_view_stats" validate:"required"`
	RecentActivity []RecentContest `json:"recent_activity" validate:"required"`
}

type OverviewStats struct {
	TotalUsers    StatWithTrend `json:"total_users" validate:"required"`
	TotalContests StatWithTrend `json:"total_contests" validate:"required"`
	Revenue       StatWithTrend `json:"revenue" validate:"required"`
	Registrations StatWithTrend `json:"registrations" validate:"required"`
}

type StatWithTrend struct {
	Value  string `json:"value" validate:"required"`
	Trend  string `json:"trend" validate:"required,oneof=up down neutral"`
	Change string `json:"change" validate:"required"`
	Data   []int  `json:"data" validate:"required"`
}

type UserStats struct {
	ByCity      []CityDistribution  `json:"by_city" validate:"required"`
	ByGender    GenderDistribution  `json:"by_gender" validate:"required"`
	ByGrade     []GradeDistribution `json:"by_grade" validate:"required"`
	GrowthTrend []int               `json:"growth_trend" validate:"required"`
}

type CityDistribution struct {
	City       string  `json:"city" validate:"required"`
	Count      int     `json:"count" validate:"min=0"`
	Percentage float64 `json:"percentage" validate:"min=0,max=100"`
}

type GenderDistribution struct {
	Male   int `json:"male" validate:"min=0"`
	Female int `json:"female" validate:"min=0"`
	Other  int `json:"other" validate:"min=0"`
}

type GradeDistribution struct {
	Grade      string  `json:"grade" validate:"required"`
	Count      int     `json:"count" validate:"min=0"`
	Percentage float64 `json:"percentage" validate:"min=0,max=100"`
}

type ContestStats struct {
	ParticipationData   []int         `json:"participation_data" validate:"required"`
	StatusDistribution  StatusStats   `json:"status_distribution" validate:"required"`
	SubjectDistribution []SubjectStat `json:"subject_distribution" validate:"required"`
}

type StatusStats struct {
	Active    int `json:"active" validate:"min=0"`
	Completed int `json:"completed" validate:"min=0"`
	Upcoming  int `json:"upcoming" validate:"min=0"`
}

type SubjectStat struct {
	Subject    string  `json:"subject" validate:"required"`
	Count      int     `json:"count" validate:"min=0"`
	Percentage float64 `json:"percentage" validate:"min=0,max=100"`
}

type RecentContest struct {
	ID            string `json:"id" validate:"required"`
	Title         string `json:"title" validate:"required"`
	Status        string `json:"status" validate:"required,oneof=Online Offline"`
	Users         int    `json:"users" validate:"min=0"`
	Subject       string `json:"subject" validate:"required"`
	QuestionCount int    `json:"noquestion" validate:"min=0"`
	TotalTime     string `json:"totaltime" validate:"required"`
	Date          string `json:"date" validate:"required"`
}
