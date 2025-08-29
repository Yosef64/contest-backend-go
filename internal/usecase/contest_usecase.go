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
	CloneContest(id string, newTitle string, newDescription string) (string, error)
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

	questions, err := u.questionRepo.GetAllQuestions()
	if err != nil {
		return nil, err
	}

	structuredQuestion := make(map[string]domain.Question)
	for _, q := range questions {
		structuredQuestion[q.ID] = q
	}

	qs := []domain.Question{}

	for _, questionID := range contest.Questions {
		if question, exists := structuredQuestion[questionID]; exists {
			qs = append(qs, question)
		}
	}

	resultContest := &domain.ContestTypeWithQuestionObj{}
	resultContest.Contest = *contest
	resultContest.Questions = qs

	return resultContest, nil
}
func (u *contestUsecase) AddContest(contest domain.Contest) (string, error) {
	contest.ID = GenerateUniqueId()
	return u.contestRepo.AddContest(contest)
}
func (u *contestUsecase) UpdateContest(id string, update domain.Contest) error {
	return u.contestRepo.UpdateContest(id, update)
}
func (u *contestUsecase) DeleteContest(id string) error { return u.contestRepo.DeleteContest(id) }

func (u *contestUsecase) CloneContest(id string, newTitle string, newDescription string) (string, error) {
	// Get the original contest
	originalContest, err := u.contestRepo.GetContestByID(id)
	if err != nil {
		return "", fmt.Errorf("failed to get original contest: %w", err)
	}
	if originalContest == nil {
		return "", fmt.Errorf("original contest not found")
	}

	// Create a new contest with the same data but new title and description
	clonedContest := domain.Contest{
		Title:       newTitle,
		Description: newDescription,
		StartTime:   originalContest.StartTime,
		EndTime:     originalContest.EndTime,
		Subject:     originalContest.Subject,
		Grade:       originalContest.Grade,
		Prize:       originalContest.Prize,
		Status:      "draft", // Set to draft status for cloned contests
		Type:        originalContest.Type,
		Questions:   originalContest.Questions, // Clone the questions
	}

	// Add the cloned contest
	clonedContestID, err := u.contestRepo.AddContest(clonedContest)
	if err != nil {
		return "", fmt.Errorf("failed to add cloned contest: %w", err)
	}

	return clonedContestID, nil
}
