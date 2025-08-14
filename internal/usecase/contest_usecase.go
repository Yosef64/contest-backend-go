package usecase

import (
	"fmt"
	"victor-contest-go/internal/domain"
)

type ContestInput struct {
	Title       string `json:"title"`
	Description string `json:"description"`
	StartTime   string `json:"start_time"`
	EndTime     string `json:"end_time"`
	// Add other fields as needed
}

func (ci *ContestInput) ToDomain() domain.Contest {
	return domain.Contest{
		Title:       ci.Title,
		Description: ci.Description,
		StartTime:   ci.StartTime,
		EndTime:     ci.EndTime,
	}
}

type ContestUsecase interface {
	GetAllContests() ([]domain.Contest, error)
	GetContestByID(id string) (*domain.ContestTypeWithQuestionObj, error)
	AddContest(contest domain.Contest) (string, error)
	UpdateContest(id string, update domain.Contest) error
	DeleteContest(id string) error
}

type contestUsecase struct {
	contestRepo  ContestRepository
	questionRepo QuestionRepository
}

func NewContestUsecase(repo ContestRepository, qRepo QuestionRepository) ContestUsecase {
	return &contestUsecase{contestRepo: repo, questionRepo: qRepo}
}

func (u *contestUsecase) GetAllContests() ([]domain.Contest, error) {
	return u.contestRepo.GetAllContests()
}
func (u *contestUsecase) GetContestByID(id string) (*domain.ContestTypeWithQuestionObj, error) {
	contest, err := u.contestRepo.GetContestByID(id)
	if err != nil || contest == nil {
		if contest == nil {
			return nil, nil
		}
		return nil, err
	}

	// Debug logging
	fmt.Printf("Contest questions IDs: %+v\n", contest.Questions)

	questions, err := u.questionRepo.GetAllQuestions()
	if err != nil {
		return nil, err
	}

	// Debug logging
	fmt.Printf("Total questions in repo: %d\n", len(questions))

	structuredQuestion := make(map[string]domain.Question)
	for _, q := range questions {
		structuredQuestion[q.ID] = q
		fmt.Printf("Question ID: %s, Question: %+v\n", q.ID, q)
	}

	qs := []domain.Question{}

	for _, questionID := range contest.Questions {
		if question, exists := structuredQuestion[questionID]; exists {
			qs = append(qs, question)
			fmt.Printf("Found question for ID %s: %+v\n", questionID, question)
		} else {
			fmt.Printf("Question not found for ID: %s\n", questionID)
		}
	}

	resultContest := &domain.ContestTypeWithQuestionObj{}
	resultContest.Contest = *contest
	resultContest.Questions = qs

	// Debug logging
	fmt.Printf("Final questions count: %d\n", len(qs))

	return resultContest, nil
}
func (u *contestUsecase) AddContest(contest domain.Contest) (string, error) {
	return u.contestRepo.AddContest(contest)
}
func (u *contestUsecase) UpdateContest(id string, update domain.Contest) error {
	return u.contestRepo.UpdateContest(id, update)
}
func (u *contestUsecase) DeleteContest(id string) error { return u.contestRepo.DeleteContest(id) }
