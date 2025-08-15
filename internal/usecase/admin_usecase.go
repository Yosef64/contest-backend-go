package usecase

import (
	"fmt"
	"math"
	"sort"
	"strconv"
	"strings"
	"time"
	"victor-contest-go/internal/domain"
)

type AdminUsecase interface {
	AddAdmin(admin domain.Admin) (string, error)
	UpdateAdmin(id string, update domain.Admin) error
	DeleteAdmin(id string) error
	GetAdminByID(id string) (*domain.Admin, error)
	GetAllAdmins() ([]domain.Admin, error)
	SignIn(email, password string) (*domain.Admin, error)
	GetAdminByEmail(email string) (*domain.Admin, error)
	GetDashboardStats() (*domain.DashboardStatsResponse, error)
}

type adminUsecase struct {
	repo                    AdminRepository
	studentRepo             StudentRepository
	contestRepo             ContestRepository
	submissionRepo          SubmissionRepository
	contestRegistrationRepo ContestRegistrationRepository
	paymentRepo             PaymentRepository
}

// GetAdminByEmail implements AdminUsecase.
func (u *adminUsecase) GetAdminByEmail(email string) (*domain.Admin, error) {
	return u.repo.GetAdminByEmail(email)
}

func NewAdminUsecase(repo AdminRepository, studentRepo StudentRepository, contestRepo ContestRepository, submissionRepo SubmissionRepository, contestRegistrationRepo ContestRegistrationRepository, paymentRepo PaymentRepository) AdminUsecase {
	return &adminUsecase{
		repo:                    repo,
		studentRepo:             studentRepo,
		contestRepo:             contestRepo,
		submissionRepo:          submissionRepo,
		contestRegistrationRepo: contestRegistrationRepo,
		paymentRepo:             paymentRepo,
	}
}

func (u *adminUsecase) AddAdmin(admin domain.Admin) (string, error) {
	return u.repo.AddAdmin(admin)
}
func (u *adminUsecase) UpdateAdmin(id string, update domain.Admin) error {
	return u.repo.UpdateAdmin(id, update)
}
func (u *adminUsecase) DeleteAdmin(id string) error {
	return u.repo.DeleteAdmin(id)
}
func (u *adminUsecase) GetAdminByID(id string) (*domain.Admin, error) {
	return u.repo.GetAdminByID(id)
}
func (u *adminUsecase) GetAllAdmins() ([]domain.Admin, error) {
	return u.repo.GetAllAdmins()
}
func (u *adminUsecase) SignIn(email, password string) (*domain.Admin, error) {
	return u.repo.SignIn(email, password)
}

func (u *adminUsecase) GetDashboardStats() (*domain.DashboardStatsResponse, error) {
	// Get all required data
	students, err := u.studentRepo.GetStudents()
	if err != nil {
		return nil, err
	}

	contests, err := u.contestRepo.GetAllContests()
	if err != nil {
		return nil, err
	}

	submissions, err := u.submissionRepo.GetAllSubmissions()
	if err != nil {
		return nil, err
	}

	payments, err := u.paymentRepo.ListAll()
	if err != nil {
		return nil, err
	}

	// Calculate overview stats
	overviewStats := u.calculateOverviewStats(students, contests, submissions, payments)

	// Calculate user stats
	userStats := u.calculateUserStats(students)

	// Calculate contest stats
	contestStats := u.calculateContestStats(contests, submissions)

	// Get recent activity
	recentActivity := u.getRecentActivity(contests, submissions)

	return &domain.DashboardStatsResponse{
		Overview:       overviewStats,
		UserStats:      userStats,
		ContestStats:   contestStats,
		RecentActivity: recentActivity,
	}, nil
}

func (u *adminUsecase) calculateOverviewStats(students []domain.Student, contests []domain.Contest, submissions []domain.Submission, payments []domain.PaymentRequest) domain.OverviewStats {
	// Calculate total users with trend
	totalUsers := len(students)
	userTrendData := u.calculateUserTrendData(students)
	userTrend, userChange := u.calculateTrend(userTrendData)

	// Calculate total contests with trend
	totalContests := len(contests)
	contestTrendData := u.calculateContestTrendData(contests)
	contestTrend, contestChange := u.calculateTrend(contestTrendData)

	// Calculate revenue with trend
	revenue := u.calculateRevenue(payments)
	revenueTrendData := u.calculateRevenueTrendData(payments)
	revenueTrend, revenueChange := u.calculateTrend(revenueTrendData)

	// Calculate registrations with trend
	registrations := len(submissions)
	registrationTrendData := u.calculateRegistrationTrendData(submissions)
	registrationTrend, registrationChange := u.calculateTrend(registrationTrendData)

	return domain.OverviewStats{
		TotalUsers: domain.StatWithTrend{
			Value:  strconv.Itoa(totalUsers),
			Trend:  userTrend,
			Change: userChange,
			Data:   userTrendData,
		},
		TotalContests: domain.StatWithTrend{
			Value:  strconv.Itoa(totalContests),
			Trend:  contestTrend,
			Change: contestChange,
			Data:   contestTrendData,
		},
		Revenue: domain.StatWithTrend{
			Value:  fmt.Sprintf("$%.2f", revenue),
			Trend:  revenueTrend,
			Change: revenueChange,
			Data:   revenueTrendData,
		},
		Registrations: domain.StatWithTrend{
			Value:  strconv.Itoa(registrations),
			Trend:  registrationTrend,
			Change: registrationChange,
			Data:   registrationTrendData,
		},
	}
}

func (u *adminUsecase) calculateUserStats(students []domain.Student) domain.UserStats {
	// Calculate city distribution
	cityMap := make(map[string]int)
	genderMap := make(map[string]int)
	gradeMap := make(map[string]int)

	for _, student := range students {
		// City distribution
		if student.City != "" {
			cityMap[student.City]++
		}

		// Gender distribution
		if student.Gender != "" {
			genderMap[strings.ToLower(student.Gender)]++
		}

		// Grade distribution
		if student.Grade != "" {
			gradeMap[student.Grade]++
		}
	}

	totalStudents := len(students)

	// Convert city map to sorted slice
	cityDistribution := make([]domain.CityDistribution, 0, len(cityMap))
	for city, count := range cityMap {
		percentage := float64(count) / float64(totalStudents) * 100
		cityDistribution = append(cityDistribution, domain.CityDistribution{
			City:       city,
			Count:      count,
			Percentage: math.Round(percentage*100) / 100,
		})
	}

	// Sort by count descending
	sort.Slice(cityDistribution, func(i, j int) bool {
		return cityDistribution[i].Count > cityDistribution[j].Count
	})

	// Convert grade map to sorted slice
	gradeDistribution := make([]domain.GradeDistribution, 0, len(gradeMap))
	for grade, count := range gradeMap {
		percentage := float64(count) / float64(totalStudents) * 100
		gradeDistribution = append(gradeDistribution, domain.GradeDistribution{
			Grade:      grade,
			Count:      count,
			Percentage: math.Round(percentage*100) / 100,
		})
	}

	// Sort by grade
	sort.Slice(gradeDistribution, func(i, j int) bool {
		return gradeDistribution[i].Grade < gradeDistribution[j].Grade
	})

	// Calculate growth trend (30-day data points)
	growthTrend := u.calculateUserTrendData(students)

	return domain.UserStats{
		ByCity: cityDistribution,
		ByGender: domain.GenderDistribution{
			Male:   genderMap["male"],
			Female: genderMap["female"],
			Other:  genderMap["other"],
		},
		ByGrade:     gradeDistribution,
		GrowthTrend: growthTrend,
	}
}

func (u *adminUsecase) calculateContestStats(contests []domain.Contest, submissions []domain.Submission) domain.ContestStats {
	// Calculate participation data (30-day trend)
	participationData := u.calculateParticipationTrendData(submissions)

	// Calculate status distribution
	statusMap := make(map[string]int)
	now := time.Now()

	for _, contest := range contests {
		startTime, err := time.Parse("2006-01-02T15:04:05Z", contest.StartTime)
		if err != nil {
			continue
		}
		endTime, err := time.Parse("2006-01-02T15:04:05Z", contest.EndTime)
		if err != nil {
			continue
		}

		if now.Before(startTime) {
			statusMap["upcoming"]++
		} else if now.After(endTime) {
			statusMap["completed"]++
		} else {
			statusMap["active"]++
		}
	}

	// Calculate subject distribution
	subjectMap := make(map[string]int)
	for _, contest := range contests {
		if contest.Subject != "" {
			subjectMap[contest.Subject]++
		}
	}

	totalContests := len(contests)
	subjectDistribution := make([]domain.SubjectStat, 0, len(subjectMap))
	for subject, count := range subjectMap {
		percentage := float64(count) / float64(totalContests) * 100
		subjectDistribution = append(subjectDistribution, domain.SubjectStat{
			Subject:    subject,
			Count:      count,
			Percentage: math.Round(percentage*100) / 100,
		})
	}

	// Sort by count descending
	sort.Slice(subjectDistribution, func(i, j int) bool {
		return subjectDistribution[i].Count > subjectDistribution[j].Count
	})

	return domain.ContestStats{
		ParticipationData: participationData,
		StatusDistribution: domain.StatusStats{
			Active:    statusMap["active"],
			Completed: statusMap["completed"],
			Upcoming:  statusMap["upcoming"],
		},
		SubjectDistribution: subjectDistribution,
	}
}

func (u *adminUsecase) getRecentActivity(contests []domain.Contest, submissions []domain.Submission) []domain.RecentContest {
	// Create a map to count participants per contest
	participantMap := make(map[string]int)
	for _, submission := range submissions {
		participantMap[submission.ContestID]++
	}

	// Convert contests to recent activity format
	recentActivity := make([]domain.RecentContest, 0, len(contests))
	for _, contest := range contests {
		// Parse start time for date formatting
		startTime, err := time.Parse("2006-01-02T15:04:05Z", contest.StartTime)
		if err != nil {
			startTime = time.Now()
		}

		// Determine status
		status := "Offline"
		if contest.Status == "active" {
			status = "Online"
		}

		// Calculate total time (assuming it's duration between start and end)
		endTime, err := time.Parse("2006-01-02T15:04:05Z", contest.EndTime)
		totalTime := "N/A"
		if err == nil {
			duration := endTime.Sub(startTime)
			hours := int(duration.Hours())
			minutes := int(duration.Minutes()) % 60
			totalTime = fmt.Sprintf("%dh %dm", hours, minutes)
		}

		recentActivity = append(recentActivity, domain.RecentContest{
			ID:            contest.ID,
			Title:         contest.Title,
			Status:        status,
			Users:         participantMap[contest.ID],
			Subject:       contest.Subject,
			QuestionCount: len(contest.Questions),
			TotalTime:     totalTime,
			Date:          startTime.Format("2006-01-02"),
		})
	}

	// Sort by date descending (most recent first)
	sort.Slice(recentActivity, func(i, j int) bool {
		dateI, _ := time.Parse("2006-01-02", recentActivity[i].Date)
		dateJ, _ := time.Parse("2006-01-02", recentActivity[j].Date)
		return dateI.After(dateJ)
	})

	// Return only the most recent 10 contests
	if len(recentActivity) > 10 {
		recentActivity = recentActivity[:10]
	}

	return recentActivity
}

func (u *adminUsecase) calculateRevenue(payments []domain.PaymentRequest) float64 {
	var totalRevenue float64
	for _, payment := range payments {
		if payment.Status == domain.StatusApproved {
			// Assuming a fixed amount per approved payment
			// This should be adjusted based on actual payment amounts in the system
			totalRevenue += 100.0 // Placeholder amount
		}
	}
	return totalRevenue
}

func (u *adminUsecase) calculateUserTrendData(students []domain.Student) []int {
	// Generate 30-day trend data for users (daily data)
	now := time.Now()
	trendData := make([]int, 30)

	// Count users registered in each of the last 30 days
	for i := 0; i < 30; i++ {
		// Go back i days from current day
		targetDate := now.AddDate(0, 0, -29+i)
		count := 0

		for _, student := range students {
			createdAt := student.CreatedAt

			// Check if student was created on the target date
			if createdAt.Year() == targetDate.Year() &&
				createdAt.YearDay() == targetDate.YearDay() {
				count++
			}
		}

		trendData[i] = count
	}

	return trendData
}

func (u *adminUsecase) calculateContestTrendData(contests []domain.Contest) []int {
	// Generate 30-day trend data for contests (daily data)
	now := time.Now()
	trendData := make([]int, 30)

	// Count contests created in each of the last 30 days
	for i := 0; i < 30; i++ {
		// Go back i days from current day
		targetDate := now.AddDate(0, 0, -29+i)
		count := 0

		for _, contest := range contests {
			startTime, err := time.Parse("2006-01-02T15:04:05Z", contest.StartTime)
			if err != nil {
				continue
			}

			// Check if contest was created on the target date
			if startTime.Year() == targetDate.Year() &&
				startTime.YearDay() == targetDate.YearDay() {
				count++
			}
		}

		trendData[i] = count
	}

	return trendData
}

func (u *adminUsecase) calculateRevenueTrendData(payments []domain.PaymentRequest) []int {
	// Generate 30-day revenue trend data (daily data)
	now := time.Now()
	trendData := make([]int, 30)

	for i := 0; i < 30; i++ {
		// Go back i days from current day
		targetDate := now.AddDate(0, 0, -29+i)
		revenue := 0

		for _, payment := range payments {
			if payment.Status == domain.StatusApproved &&
				payment.UpdatedAt.Year() == targetDate.Year() &&
				payment.UpdatedAt.YearDay() == targetDate.YearDay() {
				revenue += 100 // Placeholder amount
			}
		}

		trendData[i] = revenue
	}

	return trendData
}

func (u *adminUsecase) calculateRegistrationTrendData(submissions []domain.Submission) []int {
	// Generate 30-day registration trend data (daily data)
	now := time.Now()
	trendData := make([]int, 30)

	for i := 0; i < 30; i++ {
		// Go back i days from current day
		targetDate := now.AddDate(0, 0, -29+i)
		count := 0

		for _, submission := range submissions {
			if submission.SubmissionTime.Year() == targetDate.Year() &&
				submission.SubmissionTime.YearDay() == targetDate.YearDay() {
				count++
			}
		}

		trendData[i] = count
	}

	return trendData
}

func (u *adminUsecase) calculateParticipationTrendData(submissions []domain.Submission) []int {
	// Generate 12-month participation trend data (monthly data)
	now := time.Now()
	trendData := make([]int, 12)

	for i := 0; i < 12; i++ {
		// Go back i months from current month
		targetMonth := now.AddDate(0, -11+i, 0)
		count := 0

		for _, submission := range submissions {
			if submission.SubmissionTime.Year() == targetMonth.Year() &&
				submission.SubmissionTime.Month() == targetMonth.Month() {
				count++
			}
		}

		trendData[i] = count
	}

	return trendData
}

func (u *adminUsecase) calculateTrend(data []int) (string, string) {
	if len(data) < 2 {
		return "neutral", "0%"
	}

	// Compare last value with previous value
	current := data[len(data)-1]
	previous := data[len(data)-2]

	if current > previous {
		change := float64(current-previous) / float64(previous) * 100
		return "up", fmt.Sprintf("+%.1f%%", change)
	} else if current < previous {
		change := float64(previous-current) / float64(previous) * 100
		return "down", fmt.Sprintf("-%.1f%%", change)
	}

	return "neutral", "0%"
}
