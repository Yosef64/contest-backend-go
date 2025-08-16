package usecase

import (
	"math"
	"sort"
	"time"
	"victor-contest-go/internal/domain"
)

type PageViewUsecase interface {
	TrackPageView(pageView domain.PageView) error
	GetPageViewStats(days int) (*domain.PageViewStats, error)
	GetPageViewsByDateRange(startDate, endDate time.Time) ([]domain.PageView, error)
}

type pageViewUsecase struct {
	repo PageViewRepository
}

func NewPageViewUsecase(repo PageViewRepository) PageViewUsecase {
	return &pageViewUsecase{repo: repo}
}

func (u *pageViewUsecase) TrackPageView(pageView domain.PageView) error {
	return u.repo.AddPageView(pageView)
}

func (u *pageViewUsecase) GetPageViewsByDateRange(startDate, endDate time.Time) ([]domain.PageView, error) {
	return u.repo.GetPageViewsByDateRange(startDate, endDate)
}

func (u *pageViewUsecase) GetPageViewStats(days int) (*domain.PageViewStats, error) {
	// Get page views for the last N days
	endDate := time.Now()
	startDate := endDate.AddDate(0, 0, -days)

	pageViews, err := u.repo.GetPageViewsByDateRange(startDate, endDate)
	if err != nil {
		return nil, err
	}

	// Calculate total views
	totalViews := len(pageViews)

	// Calculate unique visitors
	uniqueVisitors := make(map[string]bool)
	viewsByPage := make(map[string]int)
	viewsByDay := make([]int, days)

	for _, pv := range pageViews {
		// Count unique visitors (by user_id or ip_address if no user_id)
		visitorKey := pv.UserID
		if visitorKey == "" {
			visitorKey = pv.IPAddress
		}
		uniqueVisitors[visitorKey] = true

		// Count views by page
		viewsByPage[pv.Page]++

		// Count views by day
		dayIndex := int(endDate.Sub(pv.ViewedAt).Hours() / 24)
		if dayIndex >= 0 && dayIndex < days {
			viewsByDay[days-1-dayIndex]++
		}
	}

	// Calculate top pages
	type pageCount struct {
		page  string
		count int
	}

	var pageCounts []pageCount
	for page, count := range viewsByPage {
		pageCounts = append(pageCounts, pageCount{page: page, count: count})
	}

	// Sort by count descending
	sort.Slice(pageCounts, func(i, j int) bool {
		return pageCounts[i].count > pageCounts[j].count
	})

	// Create top pages summary (limit to top 10)
	topPages := make([]domain.PageViewSummary, 0)
	for i, pc := range pageCounts {
		if i >= 10 {
			break
		}
		percentage := float64(pc.count) / float64(totalViews) * 100
		topPages = append(topPages, domain.PageViewSummary{
			Page:       pc.page,
			Views:      pc.count,
			Percentage: math.Round(percentage*100) / 100,
		})
	}

	return &domain.PageViewStats{
		TotalViews:     totalViews,
		UniqueVisitors: len(uniqueVisitors),
		ViewsByPage:    viewsByPage,
		ViewsByDay:     viewsByDay,
		TopPages:       topPages,
	}, nil
}
