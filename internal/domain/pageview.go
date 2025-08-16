package domain

import "time"

type PageView struct {
	ID        string    `json:"id"         dynamodbav:"id"`
	UserID    string    `json:"user_id"    dynamodbav:"user_id"`
	Page      string    `json:"page"       dynamodbav:"page"`
	UserAgent string    `json:"user_agent" dynamodbav:"user_agent"`
	IPAddress string    `json:"ip_address" dynamodbav:"ip_address"`
	Referrer  string    `json:"referrer"   dynamodbav:"referrer"`
	ViewedAt  time.Time `json:"viewed_at"  dynamodbav:"viewed_at"`
}

type PageViewStats struct {
	TotalViews     int               `json:"total_views"`
	UniqueVisitors int               `json:"unique_visitors"`
	ViewsByPage    map[string]int    `json:"views_by_page"`
	ViewsByDay     []int             `json:"views_by_day"`
	TopPages       []PageViewSummary `json:"top_pages"`
}

type PageViewSummary struct {
	Page       string  `json:"page"`
	Views      int     `json:"views"`
	Percentage float64 `json:"percentage"`
}
