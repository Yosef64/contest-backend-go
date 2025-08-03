package usecase

import "victor-contest-go/internal/domain"

type NotificationUsecase interface {
	AddNotification(notification domain.Notification) (string, error)
	UpdateNotification(id string, update domain.Notification) error
	DeleteNotification(id string) error
	GetNotificationByID(id string) (*domain.Notification, error)
	GetAllNotifications() ([]domain.Notification, error)
	GetNotificationsByRecipient(recipientID string) ([]domain.Notification, error)
}

type notificationUsecase struct {
	repo NotificationRepository
}

func NewNotificationUsecase(repo NotificationRepository) NotificationUsecase {
	return &notificationUsecase{repo: repo}
}

func (u *notificationUsecase) AddNotification(notification domain.Notification) (string, error) {
	return u.repo.AddNotification(notification)
}
func (u *notificationUsecase) UpdateNotification(id string, update domain.Notification) error {
	return u.repo.UpdateNotification(id, update)
}
func (u *notificationUsecase) DeleteNotification(id string) error {
	return u.repo.DeleteNotification(id)
}
func (u *notificationUsecase) GetNotificationByID(id string) (*domain.Notification, error) {
	
	return u.repo.GetNotificationByID(id)
}
func (u *notificationUsecase) GetAllNotifications() ([]domain.Notification, error) {
	return u.repo.GetAllNotifications()
}
func (u *notificationUsecase) GetNotificationsByRecipient(recipientID string) ([]domain.Notification, error) {
	notifications := make([]domain.Notification, 0)
	n,err := u.repo.GetNotificationsByRecipient(recipientID)
	if err != nil {
		return nil, err
	}
	if n == nil {
		return notifications,nil
	}
	
	return  n,nil
} 