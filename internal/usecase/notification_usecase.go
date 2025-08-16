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
	GetNotificationByID(id string) (*domain.Notification, error)
	GetAllNotifications() ([]domain.Notification, error)
	GetNotificationsByRecipient(recipientID string) ([]domain.Notification, error)
	GetNotificationsByRecipientAfterDate(recipientID string, afterDate time.Time) ([]domain.Notification, error)
	GetNotificationsByRecipientAfterRegistration(recipientID string) ([]domain.Notification, error)
	AnnounceContest(contest domain.Contest) error
	SendStudentRegistrationNotification(student domain.Student) error
	SendFeedbackResponseNotification(response domain.FeedbackResponse) error
	SendFeedbackQuestionNotification(question domain.FeedbackQuestion) error
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

// SendFeedbackQuestionNotification sends notifications to all students when a feedback question is created
func (s *notificationUsecase) SendFeedbackQuestionNotification(question domain.FeedbackQuestion) error {
	notification := domain.Notification{
		RecipientID: "all",
		Title:       "New Feedback Question 📝",
		Message:     "A new feedback question has been posted. Please take a moment to share your thoughts!",
		IsRead:      false,
		SentAt:      time.Now().Format(time.RFC3339),
		Type:        "feedback_question",
	}

	_, err := s.AddNotification(notification)
	if err != nil {
		return err
	}

	return nil
}

// SendStudentRegistrationNotification sends notifications to all admins when a new student registers
func (s *notificationUsecase) SendStudentRegistrationNotification(student domain.Student) error {
	notification := domain.Notification{
		RecipientID: "admin", // Use admin email as recipient ID
		Title:       "New Student Registration 👨‍🎓",
		Message:     "A new student '" + student.Name + "' has registered with phone: " + student.PhoneNumber,
		IsRead:      false,
		SentAt:      time.Now().Format(time.RFC3339),
		Type:        "student_registration",
	}
	_, err := s.AddNotification(notification)
	if err != nil {
		return err
	}
	return nil
}

// SendFeedbackResponseNotification sends notifications to all admins when a feedback response is submitted
func (s *notificationUsecase) SendFeedbackResponseNotification(response domain.FeedbackResponse) error {
	notification := domain.Notification{
		RecipientID: "admin", // Use admin email as recipient ID
		Title:       "New Feedback Response 📝",
		Message:     "Student '" + response.StudentName + "' has submitted a new feedback response.",
		IsRead:      false,
		SentAt:      time.Now().Format(time.RFC3339),
		Type:        "feedback_response",
	}

	_, err := s.AddNotification(notification)
	if err != nil {
		return err
	}
	return nil
}
