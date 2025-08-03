package usecase

import "victor-contest-go/internal/domain"

type AchievementUsecase interface {
	AddAchievement(achievement domain.Achievement) (string, error)
	UpdateAchievement(id string, update domain.Achievement) error
	DeleteAchievement(id string) error
	GetAchievementByID(id string) (*domain.Achievement, error)
	GetAllAchievements() ([]domain.Achievement, error)
	GetAchievementsByStudent(studentID string) ([]domain.Achievement, error)
}

type achievementUsecase struct {
	repo AchievementRepository
}

func NewAchievementUsecase(repo AchievementRepository) AchievementUsecase {
	return &achievementUsecase{repo: repo}
}

func (u *achievementUsecase) AddAchievement(achievement domain.Achievement) (string, error) {
	return u.repo.AddAchievement(achievement)
}

func (u *achievementUsecase) UpdateAchievement(id string, update domain.Achievement) error {
	return u.repo.UpdateAchievement(id, update)
}

func (u *achievementUsecase) DeleteAchievement(id string) error {
	return u.repo.DeleteAchievement(id)
}

func (u *achievementUsecase) GetAchievementByID(id string) (*domain.Achievement, error) {
	return u.repo.GetAchievementByID(id)
}

func (u *achievementUsecase) GetAllAchievements() ([]domain.Achievement, error) {
	return u.repo.GetAllAchievements()
}

func (u *achievementUsecase) GetAchievementsByStudent(studentID string) ([]domain.Achievement, error) {
	return u.repo.GetAchievementsByStudent(studentID)
} 