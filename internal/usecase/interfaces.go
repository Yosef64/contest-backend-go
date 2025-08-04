package usecase

import (
	"time"
	"victor-contest-go/internal/domain"
)

type ContestRepository interface {
	GetAllContests() ([]domain.Contest, error)
	GetContestByID(id string) (*domain.Contest, error)
	AddContest(contest domain.Contest) (string, error)
	UpdateContest(id string, update domain.Contest) error
	DeleteContest(id string) error
}

type StudentRepository interface {
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
	GetStructuredStudents() (map[string]domain.Student, error)
}

type SubmissionRepository interface {
	AddSubmission(submission domain.Submission) (string, error)
	GetSubmissionByID(id string) (*domain.Submission, error)
	GetAllSubmissions() ([]domain.Submission, error)
	GetSubmissionsByContest(contestID string) ([]domain.Submission, error)
	GetSubmissionsByStudent(studentID string) ([]domain.Submission, error)
}

type QuestionRepository interface {
	AddQuestion(question domain.Question) (string, error)
	UpdateQuestion(id string, update domain.Question) error
	DeleteQuestion(id string) error
	GetQuestionByID(id string) (*domain.Question, error)
	GetAllQuestions() ([]domain.Question, error)
}

type AdminRepository interface {
	AddAdmin(admin domain.Admin) (string, error)
	UpdateAdmin(id string, update domain.Admin) error
	DeleteAdmin(id string) error
	GetAdminByID(id string) (*domain.Admin, error)
	GetAllAdmins() ([]domain.Admin, error)
	SignIn(email, password string) (*domain.Admin, error)
	GetAdminByEmail(email string) (*domain.Admin,error)
}


type NotificationRepository interface {
	AddNotification(notification domain.Notification) (string, error)
	UpdateNotification(id string, update domain.Notification) error
	DeleteNotification(id string) error
	GetNotificationByID(id string) (*domain.Notification, error)
	GetAllNotifications() ([]domain.Notification, error)
	GetNotificationsByRecipient(recipientID string) ([]domain.Notification, error)
}

type AchievementRepository interface {
	AddAchievement(achievement domain.Achievement) (string, error)
	UpdateAchievement(id string, update domain.Achievement) error
	DeleteAchievement(id string) error
	GetAchievementByID(id string) (*domain.Achievement, error)
	GetAllAchievements() ([]domain.Achievement, error)
	GetAchievementsByStudent(studentID string) ([]domain.Achievement, error)
}

type ContestRegistrationRepository interface {
	AddContestRegistration(registration domain.ContestRegistration) (string, error)
	UpdateContestRegistration(id string, update domain.ContestRegistration) error
	DeleteContestRegistration(id string) error
	GetRegistrationsByContestAndStudent(contestID string, studentID string) (*domain.ContestRegistration, error)
}

type PaymentRepository interface {
	Create( req *domain.PaymentRequest) error
	GetByID(id string) (*domain.PaymentRequest, error)
	UpdateStatus( id string,newStatus domain.PaymentStatus,reason domain.PaymentReason) error
	ListByStatus( status domain.PaymentStatus) ([]domain.PaymentRequest, error)
	ListByUser( userID string) ([]domain.PaymentRequest, error)
	ListExpired(now time.Time) ([]domain.PaymentRequest,error)
	ListAll() ([]domain.PaymentRequest,error)
}
