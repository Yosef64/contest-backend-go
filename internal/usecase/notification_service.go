package usecase

import (
	"time"
	"victor-contest-go/internal/domain"
)

type NotificationService struct {
	notificationRepo NotificationRepository
	studentRepo      StudentRepository
}

func NewNotificationService(notificationRepo NotificationRepository, studentRepo StudentRepository) *NotificationService {
	return &NotificationService{
		notificationRepo: notificationRepo,
		studentRepo:      studentRepo,
	}
}

// SendContestAnnouncementNotification sends notifications to all students when a contest is announced
func (s *NotificationService) SendContestAnnouncementNotification(contest domain.Contest, customMessage string) error {
	students, err := s.studentRepo.GetStudents()
	if err != nil {
		return err
	}

	// Use custom message if provided, otherwise use default
	message := customMessage
	if message == "" {
		message = "A new contest '" + contest.Title + "' has been announced. Check it out and register now!"
	}

	for _, student := range students {
		notification := domain.Notification{
			RecipientID: student.TelegramID,
			Title:       "New Contest Announced! 🏆",
			Message:     message,
			IsRead:      false,
			SentAt:      time.Now().Format(time.RFC3339),
			Type:        "contest_announcement",
		}

		_, err := s.notificationRepo.AddNotification(notification)
		if err != nil {
			// Log error but continue with other students
			continue
		}
	}

	return nil
}

// SendFeedbackQuestionNotification sends notifications to all students when a feedback question is created
func (s *NotificationService) SendFeedbackQuestionNotification(question domain.FeedbackQuestion) error {
	students, err := s.studentRepo.GetStudents()
	if err != nil {
		return err
	}

	for _, student := range students {
		notification := domain.Notification{
			RecipientID: student.TelegramID,
			Title:       "New Feedback Question 📝",
			Message:     "A new feedback question has been posted. Please take a moment to share your thoughts!",
			IsRead:      false,
			SentAt:      time.Now().Format(time.RFC3339),
			Type:        "feedback_question",
		}

		_, err := s.notificationRepo.AddNotification(notification)
		if err != nil {
			// Log error but continue with other students
			continue
		}
	}

	return nil
}
