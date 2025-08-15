package usecase

import (
	"errors"
	"sort"
	"time"
	"victor-contest-go/internal/domain"
)

// StudentUsecase defines the business logic for students
type StudentUsecase interface {
	AddStudent(student domain.Student) error
	UpdateStudent(student domain.Student) error
	VerifyStudentPaid(telegramID string) (bool, error)
	GetPaidStudents() ([]domain.Student, error)
	GetStudents() ([]domain.Student, error)
	GetStudentByID(id string) (*domain.Student, error)
	GetQuickStat(studentID string) (map[string]interface{}, error)
	GetStudentRankings() ([]map[string]interface{}, error)
	GetStudentRankingsByContest(contestID string) ([]map[string]interface{}, error)
	GetGradesAndSchools() (map[string][]string, error)
	GetUserProfile(studentID string) (map[string]interface{}, error)
	GetUserStatForAdmin(studId string) (*domain.StudentProfileAdminResponse, error)
}

type studentUsecase struct {
	repo           StudentRepository
	paymentRepo    PaymentRepository
	submissionRepo SubmissionRepository
	contestRepo    ContestRepository
}

func NewStudentUsecase(repo StudentRepository, paymentRepo PaymentRepository, submissionRepo SubmissionRepository, contestRepo ContestRepository) StudentUsecase {
	return &studentUsecase{repo: repo, paymentRepo: paymentRepo, submissionRepo: submissionRepo, contestRepo: contestRepo}
}

func (u *studentUsecase) AddStudent(student domain.Student) error {
	student.CreatedAt = time.Now().In(time.Local)
	return u.repo.AddStudent(student)
}
func (u *studentUsecase) UpdateStudent(student domain.Student) error {
	return u.repo.UpdateStudent(student)
}
func (u *studentUsecase) VerifyStudentPaid(telegramID string) (bool, error) {
	return u.repo.VerifyStudentPaid(telegramID)
}
func (u *studentUsecase) GetPaidStudents() ([]domain.Student, error) {
	return u.repo.GetPaidStudents()
}
func (u *studentUsecase) GetStudents() ([]domain.Student, error) {
	return u.repo.GetStudents()
}
func (u *studentUsecase) GetStudentByID(id string) (*domain.Student, error) {
	student, err := u.repo.GetStudentByID(id)
	if err != nil {
		return nil, err
	}
	if student == nil {
		return nil, nil
	}
	payments, err := u.paymentRepo.ListByUser(id)
	if err != nil {
		return nil, err
	}

	if student.ReadNotifications == nil {
		student.ReadNotifications = make(map[string]domain.ReadNotificationModel)
	}

	for _, pay := range payments {
		if pay.ExpirationDate.After(time.Now().In(time.Local)) {
			student.IsPremium = true
			break
		}
	}

	return student, nil
}
func (u *studentUsecase) GetQuickStat(studentID string) (map[string]any, error) {
	return u.repo.GetQuickStat(studentID)
}
func (u *studentUsecase) GetStudentRankings() ([]map[string]any, error) {
	return u.repo.GetStudentRankings()
}
func (u *studentUsecase) GetStudentRankingsByContest(contestID string) ([]map[string]any, error) {
	return u.repo.GetStudentRankingsByContest(contestID)
}
func (u *studentUsecase) GetGradesAndSchools() (map[string][]string, error) {
	return u.repo.GetGradesAndSchools()
}
func (u *studentUsecase) GetUserProfile(studentID string) (map[string]any, error) {
	return u.repo.GetUserProfile(studentID)
}

func (r *studentUsecase) GetUserStatForAdmin(studId string) (*domain.StudentProfileAdminResponse, error) {
	student, err := r.repo.GetStudentByID(studId)
	if err != nil {
		return nil, err
	}
	if student == nil {
		return nil, errors.New("no student found with the provided ID")
	}

	userSubmissions, err := r.submissionRepo.GetSubmissionsByStudent(studId)
	if err != nil {
		return nil, err
	}
	totalPoints := CalculatePoints(userSubmissions)

	payments, err := r.paymentRepo.ListByUser(studId)
	if err != nil && err.Error() != "payments not found" {
		return nil, err
	}
	sort.Slice(payments, func(i, j int) bool {
		return payments[i].CreatedAt.After(payments[j].CreatedAt)
	})
	var payment domain.PaymentRequest
	if len(payments) > 0 {
		payment = payments[len(payments)-1]
	}
	contests, err := r.contestRepo.GetAllContests()
	if err != nil {
		return nil, err
	}
	structuredContests := make(map[string]domain.Contest)
	for _, contest := range contests {
		structuredContests[contest.ID] = contest
	}

	contestSubmissions := make([]domain.Submission, 0, len(userSubmissions))
	for _, sub := range userSubmissions {
		sub.Contest = structuredContests[sub.ContestID]
		contestSubmissions = append(contestSubmissions, sub)
	}

	result := &domain.StudentProfileAdminResponse{
		Student:            *student,
		TotalPoints:        totalPoints,
		Payment:            payment,
		ContestSubmissions: contestSubmissions,
	}

	return result, nil
}

func CalculatePoints(submissions []domain.Submission) int64 {
	var totalPoints int64
	for _, sub := range submissions {
		totalPoints += int64(sub.Score)
	}
	return totalPoints
}
