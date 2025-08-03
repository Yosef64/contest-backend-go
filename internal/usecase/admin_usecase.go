package usecase

import "victor-contest-go/internal/domain"

type AdminUsecase interface {
	AddAdmin(admin domain.Admin) (string, error)
	UpdateAdmin(id string, update domain.Admin) error
	DeleteAdmin(id string) error
	GetAdminByID(id string) (*domain.Admin, error)
	GetAllAdmins() ([]domain.Admin, error)
	SignIn(email, password string) (*domain.Admin, error)
	GetAdminByEmail(email string) (*domain.Admin, error)
}

type adminUsecase struct {
	repo AdminRepository
}

// GetAdminByEmail implements AdminUsecase.
func (u *adminUsecase) GetAdminByEmail(email string) (*domain.Admin, error) {
	return u.repo.GetAdminByEmail(email)
}

func NewAdminUsecase(repo AdminRepository) AdminUsecase {
	return &adminUsecase{repo: repo}
}

func (u *adminUsecase) AddAdmin(admin domain.Admin) (string, error) {
	return u.repo.AddAdmin(admin)
}
func (u *adminUsecase) UpdateAdmin(id string, update domain.Admin) error {
	return u.repo.UpdateAdmin(id, update)
}
func (u *adminUsecase) DeleteAdmin(id string) error {
	return u.repo.DeleteAdmin(id)
}
func (u *adminUsecase) GetAdminByID(id string) (*domain.Admin, error) {
	return u.repo.GetAdminByID(id)
}
func (u *adminUsecase) GetAllAdmins() ([]domain.Admin, error) {
	return u.repo.GetAllAdmins()
}
func (u *adminUsecase) SignIn(email, password string) (*domain.Admin, error) {
	return u.repo.SignIn(email, password)
}
