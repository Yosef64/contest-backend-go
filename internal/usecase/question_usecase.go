package usecase

import "victor-contest-go/internal/domain"

type QuestionUsecase interface {
	AddQuestion(question domain.Question) (string, error)
	UpdateQuestion(id string, update domain.Question) error
	DeleteQuestion(id string) error
	GetQuestionByID(id string) (*domain.Question, error)
	GetAllQuestions() ([]domain.Question, error)
	AddMultipleQuestions(questions []domain.Question) error
}

type questionUsecase struct {
	repo QuestionRepository
}

func NewQuestionUsecase(repo QuestionRepository) QuestionUsecase {
	return &questionUsecase{repo: repo}
}

func (u *questionUsecase) AddQuestion(question domain.Question) (string, error) {
	return u.repo.AddQuestion(question)
}
func (u *questionUsecase) UpdateQuestion(id string, update domain.Question) error {
	return u.repo.UpdateQuestion(id, update)
}
func (u *questionUsecase) DeleteQuestion(id string) error {
	return u.repo.DeleteQuestion(id)
}
func (u *questionUsecase) GetQuestionByID(id string) (*domain.Question, error) {
	return u.repo.GetQuestionByID(id)
}
func (u *questionUsecase) GetAllQuestions() ([]domain.Question, error) {
	return u.repo.GetAllQuestions()
}

func (u *questionUsecase) AddMultipleQuestions(questions []domain.Question) error {
	err := u.repo.AddMultipleQuestions(questions)
	return err
}
