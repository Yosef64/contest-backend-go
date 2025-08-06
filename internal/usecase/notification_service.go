package usecase

import (
	"time"
	"victor-contest-go/internal/domain"
)

type NotificationService struct {
	notificationRepo NotificationRepository
	studentRepo      StudentRepository
	adminRepo        AdminRepository
}

func NewNotificationService(notificationRepo NotificationRepository, studentRepo StudentRepository, adminRepo AdminRepository) *NotificationService {
	return &NotificationService{
		notificationRepo: notificationRepo,
		studentRepo:      studentRepo,
		adminRepo:        adminRepo,
	}
}

// SendContestAnnouncementNotification sends notifications to all students when a contest is announced
func (s *NotificationService) SendContestAnnouncementNotification(contest domain.Contest) error {
	students, err := s.studentRepo.GetStudents()
	if err != nil {
		return err
	}

	for _, student := range students {
		notification := domain.Notification{
			RecipientID: student.TelegramID,
			Title:       "New Contest Announced! 🏆",
			Message:     "A new contest '" + contest.Title + "' has been announced. Check it out and register now!",
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

// SendStudentRegistrationNotification sends notifications to all admins when a new student registers
func (s *NotificationService) SendStudentRegistrationNotification(student domain.Student) error {
	admins, err := s.adminRepo.GetAllAdmins()
	if err != nil {
		return err
	}

	for _, admin := range admins {
		notification := domain.Notification{
			RecipientID: admin.Email, // Use admin email as recipient ID
			Title:       "New Student Registration 👨‍🎓",
			Message:     "A new student '" + student.Name + "' has registered with phone: " + student.PhoneNumber,
			IsRead:      false,
			SentAt:      time.Now().Format(time.RFC3339),
			Type:        "student_registration",
		}

		_, err := s.notificationRepo.AddNotification(notification)
		if err != nil {
			// Log error but continue with other admins
			continue
		}
	}

	return nil
}

// SendFeedbackResponseNotification sends notifications to all admins when a feedback response is submitted
func (s *NotificationService) SendFeedbackResponseNotification(response domain.FeedbackResponse) error {
	admins, err := s.adminRepo.GetAllAdmins()
	if err != nil {
		return err
	}

	// Get student info for the notification
	student, err := s.studentRepo.GetStudentByID(response.StudentID)
	if err != nil {
		// If we can't get student info, use the name from the response
		studentName := response.StudentName
		if studentName == "" {
			studentName = "Unknown Student"
		}

		for _, admin := range admins {
			notification := domain.Notification{
				RecipientID: admin.Email, // Use admin email as recipient ID
				Title:       "New Feedback Response 📝",
				Message:     "Student '" + studentName + "' has submitted a new feedback response.",
				IsRead:      false,
				SentAt:      time.Now().Format(time.RFC3339),
				Type:        "feedback_response",
			}

			_, err := s.notificationRepo.AddNotification(notification)
			if err != nil {
				// Log error but continue with other admins
				continue
			}
		}
		return nil
	}

	for _, admin := range admins {
		notification := domain.Notification{
			RecipientID: admin.Email, // Use admin email as recipient ID
			Title:       "New Feedback Response 📝",
			Message:     "Student '" + student.Name + "' has submitted a new feedback response.",
			IsRead:      false,
			SentAt:      time.Now().Format(time.RFC3339),
			Type:        "feedback_response",
		}

		_, err := s.notificationRepo.AddNotification(notification)
		if err != nil {
			// Log error but continue with other admins
			continue
		}
	}

	return nil
}
