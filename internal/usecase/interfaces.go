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
	GetSubmissionsByStudentAndContest(conId, studentID string) (*domain.Submission, error)
}

type QuestionRepository interface {
	AddQuestion(question domain.Question) (string, error)
	AddMultipleQuestions(questions []domain.Question) error
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
	GetAdminByEmail(email string) (*domain.Admin, error)
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
	GetRegistrationsByContest(contest_id string) ([]domain.ContestRegistration, error)
}

type FeedbackQuestionRepository interface {
	AddFeedbackQuestion(question domain.FeedbackQuestion) (string, error)
	UpdateFeedbackQuestion(id string, update domain.FeedbackQuestion) error
	DeleteFeedbackQuestion(id string) error
	GetFeedbackQuestionByID(id string) (*domain.FeedbackQuestion, error)
	GetAllFeedbackQuestions() ([]domain.FeedbackQuestion, error)
	GetActiveFeedbackQuestions() ([]domain.FeedbackQuestion, error)
	GetFeedbackQuestionsByAdmin(adminID string) ([]domain.FeedbackQuestion, error)
}

type PollOptionRepository interface {
	AddPollOption(option domain.PollOption) (string, error)
	UpdatePollOption(id string, update domain.PollOption) error
	DeletePollOption(id string) error
	GetPollOptionByID(id string) (*domain.PollOption, error)
	GetAllPollOptions() ([]domain.PollOption, error)
	GetPollOptionByScore(score int) (*domain.PollOption, error)
}

type FeedbackResponseRepository interface {
	AddFeedbackResponse(response domain.FeedbackResponse) (string, error)
	UpdateFeedbackResponse(id string, update domain.FeedbackResponse) error
	DeleteFeedbackResponse(id string) error
	GetFeedbackResponseByID(id string) (*domain.FeedbackResponse, error)
	GetAllFeedbackResponses() ([]domain.FeedbackResponse, error)
	GetFeedbackResponsesByStudent(studentID string) ([]domain.FeedbackResponse, error)
	GetFeedbackResponsesByQuestion(questionID string) ([]domain.FeedbackResponse, error)
	GetFeedbackAnalytics(filter domain.AnalyticsFilter) (*domain.AnalyticsData, error)
	DeleteContactByPhoneNumber(phoneNumber string) error
}
type PaymentRepository interface {
	Create(req *domain.PaymentRequest) error
	GetByID(id string) (*domain.PaymentRequest, error)
	UpdateStatus(id string, newStatus domain.PaymentStatus, reason string) error
	ListByStatus(status domain.PaymentStatus) ([]domain.PaymentRequest, error)
	ListByUser(userID string) ([]domain.PaymentRequest, error)
	ListExpired(now time.Time) ([]domain.PaymentRequest, error)
	ListAll() ([]domain.PaymentRequest, error)
}

type PageViewRepository interface {
	AddPageView(pageView domain.PageView) error
	GetPageViewsByDateRange(startDate, endDate time.Time) ([]domain.PageView, error)
	GetAllPageViews() ([]domain.PageView, error)
	GetPageViewsByUserID(userID string) ([]domain.PageView, error)
}

type ArticleRepository interface {
	Create(article domain.Article) (string, error)
	Update(id string, article domain.Article) error
	Delete(id string) error
	GetByID(id string) (*domain.Article, error)
	List() ([]domain.Article, error)
	ListPublished() ([]domain.Article, error)
	GetByStatus(status domain.ArticleStatus) ([]domain.Article, error)
	IncrementView(id string) error
	DecrementView(id string) error
	IncrementLike(id string) error
	DecrementLike(id string) error
}

type CommentRepository interface {
	Create(comment domain.Comment) (string, error)
	ListByArticleID(articleID string) ([]domain.Comment, error)
}
