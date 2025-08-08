package usecase

import (
	"errors"
	"fmt"
	"log"
	"math"
	"sort"
	"strconv"
	"strings"
	"time"
	"victor-contest-go/internal/domain"
)

type SubmissionUsecase interface {
	AddSubmission(submission domain.SubmissionDto) (string, error)
	GetSubmissionByID(id string) (*domain.Submission, error)
	GetAllSubmissions() ([]domain.Submission, error)
	GetSubmissionsByContest(contestID string) ([]domain.Submission, error)
	GetSubmissionsByStudent(studentID string) ([]domain.Submission, error)
	GetLeaderboardByTimeFrame(timeFrame string) ([]domain.LeaderboardEntry, error)
	sortAndRank(aggregates map[string]*domain.LeaderboardEntry) []domain.LeaderboardEntry
	GetRankingsForContest(contestId string) ([]domain.LeaderboardForContestEntry, error)
	GetStudentEditorial(conId, studId string) ([]domain.Editorial, error)
	GetStudentProfileStatistics(studId string) (*domain.StudentProfilesStatisticsDto, error)
	GetStudentStatistics(studId string) (*domain.UserStatistics, error)
}

type submissionUsecase struct {
	subRepo      SubmissionRepository
	questionRepo QuestionRepository
	conUsecase   ContestUsecase
}

// GetStudentStatistics implements SubmissionUsecase.
func (u *submissionUsecase) GetStudentStatistics(studId string) (*domain.UserStatistics, error) {
	userSubmissions, err := u.subRepo.GetAllSubmissions()
	if err != nil {
		return nil, err
	}

	allQuestions, err := u.questionRepo.GetAllQuestions()
	if err != nil {
		return nil, err
	}
	allContests, err := u.conUsecase.GetAllContests()
	if err != nil {
		return nil, err
	}
	structuredContests := make(map[string]domain.Contest)
	for _, con := range allContests {
		structuredContests[con.ID] = con
	}
	structuredQuestion := make(map[string]domain.Question)
	for _, q := range allQuestions {
		structuredQuestion[q.ID] = q
	}

	if len(userSubmissions) == 0 {
		return &domain.UserStatistics{
			Subjects:         make(map[string]*domain.CategoryStat),
			Chapters:         make(map[string]*domain.CategoryStat),
			Grades:           make(map[string]*domain.CategoryStat),
			PerformanceTrend: make([]domain.PerformanceTrendPoint, 0),
		}, err
	}

	contestsParticipated := make(map[string]struct{}) // Using a map as a Set
	totalTimeSeconds := 0

	// Using pointers to CategoryStat to modify values in the map directly
	subjects := make(map[string]*domain.CategoryStat)
	chapters := make(map[string]*domain.CategoryStat)
	grades := make(map[string]*domain.CategoryStat)
	performanceTrendData := make(map[string]*domain.CategoryStat)

	for _, sub := range userSubmissions {
		contest, ok := structuredContests[sub.ContestID]
		if !ok {
			continue // Skip if contest data is missing
		}

		contestsParticipated[contest.ID] = struct{}{}
		totalTimeSeconds += ParseTimeSpend(sub.TimeSpend)

		missedQuestionIDs := make(map[string]struct{})
		for _, missed := range sub.MissedQuestions {
			missedQuestionIDs[missed.ID] = struct{}{}
		}

		submissionMonth := sub.SubmissionTime.Format("2006-01")

		for _, questionID := range contest.Questions {
			question, ok := structuredQuestion[questionID]
			if !ok {
				continue
			}

			_, isMissed := missedQuestionIDs[questionID]
			isCorrect := !isMissed

			// Helper function to update stats map
			updateStatMap := func(m map[string]*domain.CategoryStat, key string) {
				if _, exists := m[key]; !exists {
					m[key] = &domain.CategoryStat{}
				}
				m[key].Total++
				if isCorrect {
					m[key].Correct++
				}
			}

			updateStatMap(subjects, question.Subject)
			updateStatMap(chapters, question.Chapter)
			updateStatMap(grades, question.Grade)
			updateStatMap(performanceTrendData, submissionMonth)
		}
	}

	totalQuestions := 0
	correctAnswers := 0
	for _, stat := range subjects {
		totalQuestions += stat.Total
		correctAnswers += stat.Correct
	}

	totalContests := len(contestsParticipated)

	accuracy := 0.0
	if totalQuestions > 0 {
		accuracy = roundTo((float64(correctAnswers)/float64(totalQuestions))*100, 2)
	}

	averageTime := 0.0
	if totalContests > 0 {
		averageTime = roundTo(float64(totalTimeSeconds)/float64(totalContests), 2)
	}

	// Calculate accuracy for each category
	finalSubjects := make(map[string]domain.CategoryStat)
	for key, stat := range subjects {
		stat.Accuracy = calculateAccuracy(stat.Correct, stat.Total)
		finalSubjects[key] = *stat
	}

	finalChapters := make(map[string]domain.CategoryStat)
	for key, stat := range chapters {
		stat.Accuracy = calculateAccuracy(stat.Correct, stat.Total)
		finalChapters[key] = *stat
	}

	finalGrades := make(map[string]domain.CategoryStat)
	for key, stat := range grades {
		stat.Accuracy = calculateAccuracy(stat.Correct, stat.Total)
		finalGrades[key] = *stat
	}

	// Build and sort performance trend
	trendList := make([]domain.PerformanceTrendPoint,0)
	var months []string
	for month := range performanceTrendData {
		months = append(months, month)
	}
	sort.Strings(months) // Sort months chronologically

	for _, month := range months {
		data := performanceTrendData[month]
		trendList = append(trendList, domain.PerformanceTrendPoint{
			Month:     month,
			Accuracy:  calculateAccuracy(data.Correct, data.Total),
			Questions: data.Total,
		})
	}

	return &domain.UserStatistics{
		TotalContests:    totalContests,
		TotalQuestions:   totalQuestions,
		Accuracy:         accuracy,
		CorrectAnswers:   correctAnswers,
		AverageTime:      averageTime,
		Subjects:         subjects,
		Chapters:         chapters,
		Grades:           grades,
		PerformanceTrend: trendList,
	}, nil
}

// GetStudentProfileStatistics implements SubmissionUsecase.
func (u *submissionUsecase) GetStudentProfileStatistics(studId string) (*domain.StudentProfilesStatisticsDto, error) {
	submissions, err := u.subRepo.GetSubmissionsByStudent(studId)
	if err != nil {
		return nil, err
	}
	totalContests := len(submissions)
	totalQuestions := 0
	correctAnswers := 0
	totalTime := 0
	for _, sub := range submissions {
		missedCount := len(sub.MissedQuestions)
		totalQuestions += missedCount + int(sub.Score)
		correctAnswers += int(sub.Score)
		seconds, err := timeStrToSeconds(sub.TimeSpend)
		if err == nil {
			totalTime += seconds
		}
	}
	accuracy := 0
	averageTime := 0
	if totalQuestions > 0 {
		accuracy = (correctAnswers * 100) / totalQuestions
		averageTime = totalTime / totalQuestions
	}

	rankings, err := u.GetLeaderboardByTimeFrame("year")
	if err != nil {
		return nil, err
	}

	rank := -1
	for i, st := range rankings {
		if st.UserID == studId {
			rank = i + 1
			break
		}
	}
	stats := domain.StudentProfilesStatisticsDto{
		Accuracy:       accuracy,
		AverageTime:    averageTime,
		TotalContests:  totalContests,
		TotalQuestions: totalQuestions,
		Rank:           rank,
		CorrectAnswers: correctAnswers,
	}
	return &stats, nil
}

// GetStudentEditorial implements SubmissionUsecase.
func (u *submissionUsecase) GetStudentEditorial(conId string, studId string) ([]domain.Editorial, error) {
	submission, err := u.GetSubmissionByID(fmt.Sprintf("%s#%s", conId, studId))
	if err != nil {
		return nil, err
	}

	contest, err := u.conUsecase.GetContestByID(conId)
	if err != nil {
		return nil, errors.New("no contest found with the submission")
	}
	if contest == nil {
		return nil, fmt.Errorf("contest not found")
	}
	missedQuestionSet := make(map[string]domain.SubmissionMissedQuestionDto)
	if submission != nil {
		for _, mQ := range submission.MissedQuestions {
			missedQuestionSet[mQ.ID] = mQ
		}
	}

	var editorial []domain.Editorial
	for _, q := range contest.Questions {
		editorialQuestion := domain.Editorial{
            Question: q,
        }
		if editorialQuestion.Question.MultipleChoice == nil {
            editorialQuestion.Question.MultipleChoice = make([]string, 0)
        }


		if missedQuestion, ok := missedQuestionSet[q.ID]; ok {
			editorialQuestion.UserAnswer = missedQuestion.SelectedAnswer
			editorialQuestion.IsCorrect = false
		} else {
			if submission == nil {
				editorialQuestion.UserAnswer = -1
			} else {
				editorialQuestion.UserAnswer = q.Answer
			}
			editorialQuestion.IsCorrect = true
		}
		editorial = append(editorial, editorialQuestion)

	}
	return editorial, nil
}

// GetLeaderboardByTimeFrame implements SubmissionUsecase.
func (u *submissionUsecase) GetLeaderboardByTimeFrame(timeFrame string) ([]domain.LeaderboardEntry, error) {
	startTime, err := u.calculateStartTime(timeFrame)
	if err != nil {
		log.Println(err)
		return nil, err
	}
	submissions, err := u.subRepo.GetAllSubmissions()

	if err != nil {
		log.Print(submissions)
		return nil, err
	}
	filteredSubmissions := make([]domain.Submission, 0)
	for _, sub := range submissions {
		if sub.SubmissionTime.After(startTime) {
			filteredSubmissions = append(filteredSubmissions, sub)
		}
	}
	userAggregates := u.aggregateSubmissions(filteredSubmissions)

	leaderboard := u.sortAndRank(userAggregates)

	return leaderboard, nil
}

func NewSubmissionUsecase(repo SubmissionRepository, conUsecase ContestUsecase, questionRepo QuestionRepository) SubmissionUsecase {
	return &submissionUsecase{subRepo: repo, conUsecase: conUsecase, questionRepo: questionRepo}
}

func (u *submissionUsecase) AddSubmission(submission domain.SubmissionDto) (string, error) {
	final_submission := domain.Submission{
		ID:              fmt.Sprintf("%s#%s", submission.ContestID, submission.Student.ID), // will be set later
		ContestID:       submission.ContestID,
		Student:         submission.Student,
		Score:           submission.Score,
		MissedQuestions: submission.MissedQuestions,
		SubmissionTime:  time.Now().In(time.Local),
		TimeSpend:       submission.TimeSpend,
	}

	return u.subRepo.AddSubmission(final_submission)
}
func (u *submissionUsecase) GetSubmissionByID(id string) (*domain.Submission, error) {
	if id == "leaderboard" {
		return nil, errors.New("unknown route")

	}
	return u.subRepo.GetSubmissionByID(id)
}
func (u *submissionUsecase) GetAllSubmissions() ([]domain.Submission, error) {
	return u.subRepo.GetAllSubmissions()
}
func (u *submissionUsecase) GetSubmissionsByContest(contestID string) ([]domain.Submission, error) {
	return u.subRepo.GetSubmissionsByContest(contestID)
}
func (u *submissionUsecase) GetSubmissionsByStudent(studentID string) ([]domain.Submission, error) {
	return u.subRepo.GetSubmissionsByStudent(studentID)
}
func (u *submissionUsecase) GetRankingsForContest(contestId string) ([]domain.LeaderboardForContestEntry, error) {

	submissionForContest, err := u.GetSubmissionsByContest(contestId)
	if err != nil {
		return nil, err
	}

	rankings := make([]domain.LeaderboardForContestEntry, 0)
	for _, sub := range submissionForContest {
		rankings = append(rankings, domain.LeaderboardForContestEntry{
			UserId:         sub.Student.ID,
			UserName:       sub.Student.Name,
			Score:          int(sub.Score),
			CorrectAnswers: int(sub.Score),
			TotalQuestions: int(sub.Score) + len(sub.MissedQuestions),
			TimeTaken:      sub.TimeSpend,
		})
	}
	sort.Slice(rankings, func(i, j int) bool {
		rank1, rank2 := rankings[i], rankings[j]
		if rank1.Score == rank2.Score {
			return rank1.TimeTaken > rank2.TimeTaken
		}
		return rank1.Score > rank2.Score
	})
	return rankings, nil
}

func (u *submissionUsecase) calculateStartTime(timeFrame string) (time.Time, error) {
	now := time.Now().In(time.Local)
	year, month, day := now.Date()

	switch timeFrame {
	case "today":
		return time.Date(year, month, day, 0, 0, 0, 0, time.Local), nil
	case "week":
		weekday := int(now.Weekday())
		startOfWeek := now.AddDate(0, 0, -weekday)
		y, m, d := startOfWeek.Date()
		return time.Date(y, m, d, 0, 0, 0, 0, time.Local), nil
	case "month":
		return time.Date(year, month, 1, 0, 0, 0, 0, time.Local), nil
	case "all":
		return time.Date(year-1, month, 1, 0, 0, 0, 0, time.Local), nil
	default:
		return time.Time{}, fmt.Errorf("invalid timeFrame: %s", timeFrame)
	}
}
func (uc *submissionUsecase) aggregateSubmissions(submissions []domain.Submission) map[string]*domain.LeaderboardEntry {
	userAggregates := make(map[string]*domain.LeaderboardEntry)

	for _, sub := range submissions {
		userID := sub.Student.ID

		timeTakenSeconds := parseTimeSpent(sub.TimeSpend)
		score := int(sub.Score)
		totalQuestions := int(score) + len(sub.MissedQuestions)

		if entry, exists := userAggregates[userID]; exists {
			entry.Score += score
			entry.CorrectAnswers += score
			entry.TotalQuestions += totalQuestions
			entry.TimeTakenSeconds += timeTakenSeconds
		} else {
			// Create new entry
			userAggregates[userID] = &domain.LeaderboardEntry{
				UserID:           sub.Student.ID,
				UserName:         sub.Student.Name,
				Score:            score,
				CorrectAnswers:   score,
				TotalQuestions:   totalQuestions,
				TimeTakenSeconds: timeTakenSeconds,
				ImageURL:         sub.Student.ImgURL,
			}
		}
	}
	return userAggregates
}
func (uc *submissionUsecase) sortAndRank(aggregates map[string]*domain.LeaderboardEntry) []domain.LeaderboardEntry {
	// Convert map to slice for sorting
	var list []domain.LeaderboardEntry
	for _, entry := range aggregates {
		list = append(list, *entry)
	}

	// Sort by score (desc) and then time taken (asc)
	sort.Slice(list, func(i, j int) bool {
		if list[i].Score != list[j].Score {
			return list[i].Score > list[j].Score
		}
		return list[i].TimeTaken < list[j].TimeTaken
	})

	limit := 100
	if len(list) < limit {
		limit = len(list)
	}

	finalLeaderboard := make([]domain.LeaderboardEntry, limit)
	for i := 0; i < limit; i++ {
		entry := list[i]
		entry.Rank = i + 1
		entry.TimeTaken = formatSecondsToHMS(entry.TimeTakenSeconds)
		finalLeaderboard[i] = entry
	}

	return finalLeaderboard
}

// --- Helper Functions ---

func parseTimeSpent(timeSpent any) int {
	switch v := timeSpent.(type) {
	case string:
		parts := strings.Split(v, ":")
		var h, m, s int
		var err error
		if len(parts) == 3 {
			h, err = strconv.Atoi(parts[0])
			if err != nil {
				return 0
			}
			m, err = strconv.Atoi(parts[1])
			if err != nil {
				return 0
			}
			s, err = strconv.Atoi(parts[2])
			if err != nil {
				return 0
			}
			return h*3600 + m*60 + s
		}
		return 0
	case int:
		return v
	case int64:
		return int(v)
	case float64:
		return int(v)
	default:
		return 0
	}
}

func formatSecondsToHMS(seconds int) string {
	hours := seconds / 3600
	minutes := (seconds % 3600) / 60
	secs := seconds % 60
	if hours > 0 {
		return fmt.Sprintf("%d:%02d:%02d", hours, minutes, secs)
	}
	return fmt.Sprintf("%02d:%02d", minutes, secs)
}
func timeStrToSeconds(timeStr string) (int, error) {
	parts := strings.Split(timeStr, ":")
	if len(parts) != 3 {
		return 0, fmt.Errorf("invalid time format")
	}
	hours, _ := strconv.Atoi(parts[0])
	minutes, _ := strconv.Atoi(parts[1])
	seconds, _ := strconv.Atoi(parts[2])
	return hours*3600 + minutes*60 + seconds, nil
}
func calculateAccuracy(correct, total int) float64 {
	if total == 0 {
		return 0.0
	}
	return roundTo((float64(correct)/float64(total))*100, 2)
}

func roundTo(val float64, places int) float64 {
	pow := math.Pow(10, float64(places))
	return math.Round(val*pow) / pow
}
func ParseTimeSpend(value string) int {
	parts := strings.Split(value, ":")
	if len(parts) != 3 {
		if s, err := strconv.Atoi(value); err == nil {
			return s
		}
		return 0
	}

	h, errH := strconv.Atoi(parts[0])
	m, errM := strconv.Atoi(parts[1])
	s, errS := strconv.Atoi(parts[2])

	if errH != nil || errM != nil || errS != nil {
		return 0
	}
	return h*3600 + m*60 + s
}
