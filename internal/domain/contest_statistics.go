package domain

// ContestStatistics represents comprehensive statistics for a contest
type ContestStatistics struct {
	ContestID         string                   `json:"contest_id"`
	ContestTitle      string                   `json:"contest_title"`
	TotalParticipants int                      `json:"total_participants"`
	PassedCount       int                      `json:"passed_count"`
	FailedCount       int                      `json:"failed_count"`
	PassRate          float64                  `json:"pass_rate"`
	AverageScore      float64                  `json:"average_score"`
	TotalQuestions    int                      `json:"total_questions"`
	GenderStats       GenderStatistics         `json:"gender_stats"`
	CityStats         map[string]CategoryStats `json:"city_stats"`
	SchoolStats       map[string]CategoryStats `json:"school_stats"`
	GradeStats        map[string]CategoryStats `json:"grade_stats"`
	ScoreDistribution ScoreDistribution        `json:"score_distribution"`
	PerformanceLevels PerformanceLevels        `json:"performance_levels"`
	GeneratedAt       string                   `json:"generated_at"`
}

// GenderStatistics represents statistics broken down by gender
type GenderStatistics struct {
	Male   CategoryStats `json:"male"`
	Female CategoryStats `json:"female"`
}

// CategoryStats represents statistics for any category (city, school, grade, gender)
type CategoryStats struct {
	Total        int     `json:"total"`
	Passed       int     `json:"passed"`
	Failed       int     `json:"failed"`
	PassRate     float64 `json:"pass_rate"`
	AverageScore float64 `json:"average_score"`
}

// ScoreDistribution represents the distribution of scores across different ranges
type ScoreDistribution struct {
	Excellent int `json:"excellent"` // 90-100%
	Good      int `json:"good"`      // 70-89%
	Average   int `json:"average"`   // 50-69%
	Poor      int `json:"poor"`      // 0-49%
}

// PerformanceLevels represents detailed performance categorization
type PerformanceLevels struct {
	Excellent int `json:"excellent"` // 90-100%
	Good      int `json:"good"`      // 70-89%
	Average   int `json:"average"`   // 50-69%
	Fail      int `json:"fail"`      // 0-49%
}

// StudentContestPerformance represents individual student performance in a contest
type StudentContestPerformance struct {
	StudentID      string  `json:"student_id"`
	StudentName    string  `json:"student_name"`
	StudentGender  string  `json:"student_gender"`
	StudentCity    string  `json:"student_city"`
	StudentSchool  string  `json:"student_school"`
	StudentGrade   string  `json:"student_grade"`
	Score          float64 `json:"score"`
	TotalQuestions int     `json:"total_questions"`
	CorrectAnswers int     `json:"correct_answers"`
	Percentage     float64 `json:"percentage"`
	Performance    string  `json:"performance"` // "excellent", "good", "average", "fail"
	TimeSpent      string  `json:"time_spent"`
	SubmissionTime string  `json:"submission_time"`
}

// ContestStatisticsRequest represents the request parameters for getting contest statistics
type ContestStatisticsRequest struct {
	ContestID string            `json:"contest_id"`
	Filters   StatisticsFilters `json:"filters"`
}

// StatisticsFilters represents filters that can be applied to contest statistics
type StatisticsFilters struct {
	Gender string `json:"gender"`
	City   string `json:"city"`
	School string `json:"school"`
	Grade  string `json:"grade"`
}

// ContestStatisticsResponse represents the response for contest statistics
type ContestStatisticsResponse struct {
	Success bool              `json:"success"`
	Data    ContestStatistics `json:"data"`
	Message string            `json:"message"`
}

// StudentPerformanceList represents a list of student performances with pagination
type StudentPerformanceList struct {
	Students   []StudentContestPerformance `json:"students"`
	TotalCount int                         `json:"total_count"`
	Page       int                         `json:"page"`
	PageSize   int                         `json:"page_size"`
	TotalPages int                         `json:"total_pages"`
}
