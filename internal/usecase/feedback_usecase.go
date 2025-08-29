package usecase

import (
	"time"
	"victor-contest-go/internal/domain"
)

// FeedbackQuestionUsecase interface
type FeedbackQuestionUsecase interface {
	AddFeedbackQuestion(question domain.FeedbackQuestion) (string, error)
	UpdateFeedbackQuestion(id string, update domain.FeedbackQuestion) error
	DeleteFeedbackQuestion(id string) error
	GetFeedbackQuestionByID(id string) (*domain.FeedbackQuestion, error)
	GetAllFeedbackQuestions() ([]domain.FeedbackQuestion, error)
	GetActiveFeedbackQuestions() ([]domain.FeedbackQuestion, error)
	GetFeedbackQuestionsByAdmin(adminID string) ([]domain.FeedbackQuestion, error)
}

type feedbackQuestionUsecase struct {
	repo FeedbackQuestionRepository
}

func NewFeedbackQuestionUsecase(repo FeedbackQuestionRepository) FeedbackQuestionUsecase {
	return &feedbackQuestionUsecase{repo: repo}
}

func (u *feedbackQuestionUsecase) AddFeedbackQuestion(question domain.FeedbackQuestion) (string, error) {
	question.CreatedAt = time.Now()
	question.UpdatedAt = time.Now()
	question.ID = GenerateUniqueId()
	return u.repo.AddFeedbackQuestion(question)
}

func (u *feedbackQuestionUsecase) UpdateFeedbackQuestion(id string, update domain.FeedbackQuestion) error {
	update.UpdatedAt = time.Now()
	return u.repo.UpdateFeedbackQuestion(id, update)
}

func (u *feedbackQuestionUsecase) DeleteFeedbackQuestion(id string) error {
	return u.repo.DeleteFeedbackQuestion(id)
}

func (u *feedbackQuestionUsecase) GetFeedbackQuestionByID(id string) (*domain.FeedbackQuestion, error) {
	return u.repo.GetFeedbackQuestionByID(id)
}

func (u *feedbackQuestionUsecase) GetAllFeedbackQuestions() ([]domain.FeedbackQuestion, error) {
	return u.repo.GetAllFeedbackQuestions()
}

func (u *feedbackQuestionUsecase) GetActiveFeedbackQuestions() ([]domain.FeedbackQuestion, error) {
	return u.repo.GetActiveFeedbackQuestions()
}

func (u *feedbackQuestionUsecase) GetFeedbackQuestionsByAdmin(adminID string) ([]domain.FeedbackQuestion, error) {
	return u.repo.GetFeedbackQuestionsByAdmin(adminID)
}

// PollOptionUsecase interface
type PollOptionUsecase interface {
	AddPollOption(option domain.PollOption) (string, error)
	UpdatePollOption(id string, update domain.PollOption) error
	DeletePollOption(id string) error
	GetPollOptionByID(id string) (*domain.PollOption, error)
	GetAllPollOptions() ([]domain.PollOption, error)
	GetPollOptionByScore(score int) (*domain.PollOption, error)
}

type pollOptionUsecase struct {
	repo PollOptionRepository
}

func NewPollOptionUsecase(repo PollOptionRepository) PollOptionUsecase {
	return &pollOptionUsecase{repo: repo}
}

func (u *pollOptionUsecase) AddPollOption(option domain.PollOption) (string, error) {
	option.CreatedAt = time.Now()
	option.UpdatedAt = time.Now()
	return u.repo.AddPollOption(option)
}

func (u *pollOptionUsecase) UpdatePollOption(id string, update domain.PollOption) error {
	update.UpdatedAt = time.Now()
	return u.repo.UpdatePollOption(id, update)
}

func (u *pollOptionUsecase) DeletePollOption(id string) error {
	return u.repo.DeletePollOption(id)
}

func (u *pollOptionUsecase) GetPollOptionByID(id string) (*domain.PollOption, error) {
	return u.repo.GetPollOptionByID(id)
}

func (u *pollOptionUsecase) GetAllPollOptions() ([]domain.PollOption, error) {
	return u.repo.GetAllPollOptions()
}

func (u *pollOptionUsecase) GetPollOptionByScore(score int) (*domain.PollOption, error) {
	return u.repo.GetPollOptionByScore(score)
}

// FeedbackResponseUsecase interface
type FeedbackResponseUsecase interface {
	AddFeedbackResponse(response domain.FeedbackResponse) (string, error)
	UpdateFeedbackResponse(id string, update domain.FeedbackResponse) error
	DeleteFeedbackResponse(id string) error
	GetFeedbackResponseByID(id string) (*domain.FeedbackResponse, error)
	GetAllFeedbackResponses() ([]domain.FeedbackResponse, error)
	GetFeedbackResponsesByStudent(studentID string) ([]domain.FeedbackResponse, error)
	GetFeedbackResponsesByQuestion(questionID string) ([]domain.FeedbackResponse, error)
	GetFeedbackAnalytics(filter domain.AnalyticsFilter) (*domain.AnalyticsData, error)
	DeleteContactByPhoneNumber(phoneNumber string) error
}

type feedbackResponseUsecase struct {
	repo FeedbackResponseRepository
}

func NewFeedbackResponseUsecase(repo FeedbackResponseRepository) FeedbackResponseUsecase {
	return &feedbackResponseUsecase{repo: repo}
}

func (u *feedbackResponseUsecase) AddFeedbackResponse(response domain.FeedbackResponse) (string, error) {
	response.SubmittedAt = time.Now()
	return u.repo.AddFeedbackResponse(response)
}

func (u *feedbackResponseUsecase) UpdateFeedbackResponse(id string, update domain.FeedbackResponse) error {
	return u.repo.UpdateFeedbackResponse(id, update)
}

func (u *feedbackResponseUsecase) DeleteFeedbackResponse(id string) error {
	return u.repo.DeleteFeedbackResponse(id)
}

func (u *feedbackResponseUsecase) GetFeedbackResponseByID(id string) (*domain.FeedbackResponse, error) {
	return u.repo.GetFeedbackResponseByID(id)
}

func (u *feedbackResponseUsecase) GetAllFeedbackResponses() ([]domain.FeedbackResponse, error) {
	return u.repo.GetAllFeedbackResponses()
}

func (u *feedbackResponseUsecase) GetFeedbackResponsesByStudent(studentID string) ([]domain.FeedbackResponse, error) {
	return u.repo.GetFeedbackResponsesByStudent(studentID)
}

func (u *feedbackResponseUsecase) GetFeedbackResponsesByQuestion(questionID string) ([]domain.FeedbackResponse, error) {
	return u.repo.GetFeedbackResponsesByQuestion(questionID)
}

func (u *feedbackResponseUsecase) GetFeedbackAnalytics(filter domain.AnalyticsFilter) (*domain.AnalyticsData, error) {
	return u.repo.GetFeedbackAnalytics(filter)
}

func (u *feedbackResponseUsecase) DeleteContactByPhoneNumber(phoneNumber string) error {
	return u.repo.DeleteContactByPhoneNumber(phoneNumber)
} 