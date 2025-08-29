package usecase

import (
	"errors"
	"time"
	"victor-contest-go/internal/domain"
)

type ContestRegistrationUsecase interface {
	AddContestRegistration(registration domain.ContestRegistrationDto) (string, error)
	UpdateContestRegistration(id string, update domain.ContestRegistration) error
	DeleteContestRegistration(id string) error
	GetRegisterationForContest(contest_id string) ([]domain.ContestRegistration, error)
	CheckRegistrationsByContestAndStudent(contestID, studentID string) (bool, error)
	CheckStudentActiveInContest(contestId, studentId string) (*bool, error)
}

type contestRegistrationUsecase struct {
	repo ContestRegistrationRepository
}

// GetRegisterationForContest implements ContestRegistrationUsecase.
func (u *contestRegistrationUsecase) GetRegisterationForContest(contest_id string) ([]domain.ContestRegistration, error) {
	regiterations, err := u.repo.GetRegistrationsByContest(contest_id)
	if err != nil {
		return nil, err
	}
	return regiterations, nil
}

func (u *contestRegistrationUsecase) CheckStudentActiveInContest(contestId string, studentId string) (*bool, error) {
	registeration, err := u.repo.GetRegistrationsByContestAndStudent(contestId, studentId)
	if err != nil {
		return nil, errors.New("the user is not registered for the contest")
	}
	if registeration.IsActive {
		return nil, errors.New("the user has been in the contest")
	}
	registeration.IsActive = true
	u.UpdateContestRegistration(registeration.ID, *registeration)
	return &registeration.IsActive, nil
}

func NewContestRegistrationUsecase(repo ContestRegistrationRepository) ContestRegistrationUsecase {
	return &contestRegistrationUsecase{repo: repo}
}

func (u *contestRegistrationUsecase) AddContestRegistration(registrationDto domain.ContestRegistrationDto) (string, error) {
	id := GenerateUniqueId()
	registration := domain.ContestRegistration{
		ContestID:    registrationDto.ContestID,
		StudentID:    registrationDto.StudentID,
		ID:           id,
		IsActive:     false,
		RegisteredAt: time.Now().In(time.Local),
	}
	return u.repo.AddContestRegistration(registration)
}

func (u *contestRegistrationUsecase) UpdateContestRegistration(id string, update domain.ContestRegistration) error {
	return u.repo.UpdateContestRegistration(id, update)
}

func (u *contestRegistrationUsecase) DeleteContestRegistration(id string) error {
	return u.repo.DeleteContestRegistration(id)
}

func (u *contestRegistrationUsecase) CheckRegistrationsByContestAndStudent(contestID, studentID string) (bool, error) {
	registered, err := u.repo.GetRegistrationsByContestAndStudent(contestID, studentID)
	if err != nil {
		return false, err
	}
	return registered != nil, nil
}
