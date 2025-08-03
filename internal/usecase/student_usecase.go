package usecase

import "victor-contest-go/internal/domain"

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
}

type studentUsecase struct {
	repo StudentRepository
}

func NewStudentUsecase(repo StudentRepository) StudentUsecase {
	return &studentUsecase{repo: repo}
}

func (u *studentUsecase) AddStudent(student domain.Student) error {
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
	return u.repo.GetStudentByID(id)
}
func (u *studentUsecase) GetQuickStat(studentID string) (map[string]interface{}, error) {
	return u.repo.GetQuickStat(studentID)
}
func (u *studentUsecase) GetStudentRankings() ([]map[string]interface{}, error) {
	return u.repo.GetStudentRankings()
}
func (u *studentUsecase) GetStudentRankingsByContest(contestID string) ([]map[string]interface{}, error) {
	return u.repo.GetStudentRankingsByContest(contestID)
}
func (u *studentUsecase) GetGradesAndSchools() (map[string][]string, error) {
	return u.repo.GetGradesAndSchools()
}
func (u *studentUsecase) GetUserProfile(studentID string) (map[string]interface{}, error) {
	return u.repo.GetUserProfile(studentID)
} 