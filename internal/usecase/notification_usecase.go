package usecase

import (
	"fmt"
	"strings"
	"time"
	"victor-contest-go/internal/domain"
)

type NotificationUsecase interface {
	AddNotification(notification domain.Notification) (string, error)
	UpdateNotification(id string, update domain.Notification) error
	DeleteNotification(id string) error
	MarkNotificationAsRead(id string) error
	GetNotificationByID(id string) (*domain.Notification, error)
	GetAllNotifications() ([]domain.Notification, error)
	GetNotificationsByRecipient(recipientID string) ([]domain.Notification, error)
	GetNotificationsByRecipientAfterDate(recipientID string, afterDate time.Time) ([]domain.Notification, error)
	GetNotificationsByRecipientAfterRegistration(recipientID string) ([]domain.Notification, error)
	AnnounceContest(contest domain.Contest) error
	SendNotification(title, message, Type, recipientId string) error
}

type notificationUsecase struct {
	repo        NotificationRepository
	contestRep  ContestRepository
	studentRepo StudentRepository
}

// AnnounceContest implements NotificationUsecase.
func (u *notificationUsecase) AnnounceContest(contest domain.Contest) error {
	notification := domain.Notification{
		RecipientID: "all",
		Title:       "New Contest Announced 🏆",
		Message: fmt.Sprintf(
			"We're excited to announce a new contest: %s! It starts on %s. Don't miss your chance to participate!",
			contest.Title,
			strings.Split(contest.StartTime, "T")[0],
		),
		IsRead: false,
		SentAt: time.Now().Format(time.RFC3339),
		Type:   "contest_announcement",
	}
	_, err := u.AddNotification(notification)
	if err != nil {
		return err
	}
	if err := u.contestRep.UpdateContest(contest.ID, contest); err != nil {
		return err
	}
	return nil
}

func NewNotificationUsecase(repo NotificationRepository, contestRepo ContestRepository, studentRepo StudentRepository) NotificationUsecase {
	return &notificationUsecase{repo: repo, contestRep: contestRepo, studentRepo: studentRepo}
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

func (u *notificationUsecase) MarkNotificationAsRead(id string) error {
	// Get the notification first
	notification, err := u.repo.GetNotificationByID(id)
	if err != nil {
		return err
	}

	// Mark as read
	notification.IsRead = true

	// Update the notification
	return u.repo.UpdateNotification(id, *notification)
}

func (u *notificationUsecase) GetNotificationByID(id string) (*domain.Notification, error) {

	return u.repo.GetNotificationByID(id)
}
func (u *notificationUsecase) GetAllNotifications() ([]domain.Notification, error) {
	return u.repo.GetAllNotifications()
}
func (u *notificationUsecase) GetNotificationsByRecipient(recipientID string) ([]domain.Notification, error) {
	notifications := make([]domain.Notification, 0)
	n, err := u.repo.GetNotificationsByRecipient(recipientID)
	if err != nil {
		return nil, err
	}
	if n == nil {
		return notifications, nil
	}

	return n, nil
}

func (u *notificationUsecase) GetNotificationsByRecipientAfterDate(recipientID string, afterDate time.Time) ([]domain.Notification, error) {
	// Get all notifications for the recipient
	allNotifications, err := u.repo.GetNotificationsByRecipient(recipientID)
	if err != nil {
		return nil, err
	}

	// Filter notifications to only include those sent after the user's registration date
	filteredNotifications := make([]domain.Notification, 0)
	for _, notification := range allNotifications {
		// Parse the SentAt timestamp
		sentAt, err := time.Parse(time.RFC3339, notification.SentAt)
		if err != nil {
			// If we can't parse the date, skip this notification or log the error
			// fmt.Printf("Warning: Could not parse notification SentAt date: %s\n", notification.SentAt)
			continue
		}

		// Only include notifications sent after the user's registration date
		if sentAt.After(afterDate) {
			filteredNotifications = append(filteredNotifications, notification)
		}
	}

	return filteredNotifications, nil
}

func (u *notificationUsecase) GetNotificationsByRecipientAfterRegistration(recipientID string) ([]domain.Notification, error) {
	// Get the student's registration date
	if recipientID == "admin" {
		return u.GetNotificationsByRecipient(recipientID)
	}
	student, err := u.studentRepo.GetStudentByID(recipientID)
	if err != nil {
		return nil, fmt.Errorf("failed to get student registration date: %w", err)
	}
	if student == nil {
		return nil, fmt.Errorf("student not found with ID: %s", recipientID)
	}
	if student.CreatedAt.IsZero() {
		fmt.Printf("Warning: Student %s has no registration date, returning all notifications\n", recipientID)
		return u.GetNotificationsByRecipient(recipientID)
	}

	return u.GetNotificationsByRecipientAfterDate(recipientID, student.CreatedAt)
}

func (s *notificationUsecase) SendNotification(title, message, Type, recipientId string) error {
	notification := domain.Notification{
		RecipientID: recipientId, // Use admin email as recipient ID
		Title:       title,
		Message:     message,
		IsRead:      false,
		SentAt:      time.Now().Format(time.RFC3339),
		Type:        Type,
	}

	_, err := s.AddNotification(notification)
	if err != nil {
		return err
	}
	return nil
}
