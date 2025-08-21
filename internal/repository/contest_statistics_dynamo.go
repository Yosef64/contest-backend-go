package repository

import (
	"victor-contest-go/internal/domain"
	"victor-contest-go/internal/usecase"
)

type ContestStatisticsDynamo struct {
	contestUsecase   usecase.ContestUsecase
	submissionUsecase usecase.SubmissionUsecase
	studentUsecase   usecase.StudentUsecase
	questionUsecase  usecase.QuestionUsecase
	statsUsecase     usecase.ContestStatisticsUsecase
}

func NewContestStatisticsDynamo(
	contestUsecase usecase.ContestUsecase,
	submissionUsecase usecase.SubmissionUsecase,
	studentUsecase usecase.StudentUsecase,
	questionUsecase usecase.QuestionUsecase,
	statsUsecase usecase.ContestStatisticsUsecase,
) *ContestStatisticsDynamo {
	return &ContestStatisticsDynamo{
		contestUsecase:    contestUsecase,
		submissionUsecase: submissionUsecase,
		studentUsecase:    studentUsecase,
		questionUsecase:   questionUsecase,
		statsUsecase:      statsUsecase,
	}
}

func (r *ContestStatisticsDynamo) GetContestStatistics(contestID string, filters domain.StatisticsFilters) (*domain.ContestStatistics, error) {
	return r.statsUsecase.GetContestStatistics(contestID, filters)
}

func (r *ContestStatisticsDynamo) GetStudentPerformancesByContest(contestID string, filters domain.StatisticsFilters, page, pageSize int) (*domain.StudentPerformanceList, error) {
	return r.statsUsecase.GetStudentPerformancesByContest(contestID, filters, page, pageSize)
}

func (r *ContestStatisticsDynamo) GetContestSummary(contestID string) (*domain.ContestStatistics, error) {
	return r.statsUsecase.GetContestSummary(contestID)
}
