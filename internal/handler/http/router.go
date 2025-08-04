package http

import (
	"victor-contest-go/internal/repository"
	"victor-contest-go/internal/usecase"

	"github.com/gin-contrib/cors"
	"github.com/gin-gonic/gin"
)

// Server holds all dependencies for the application.
type Server struct {
	contestHandler             *ContestHandler
	studentHandler             *StudentHandler
	questionHandler            *QuestionHandler
	submissionHandler          *SubmissionHandler
	adminHandler               *AdminHandler
	notificationHandler        *NotificationHandler
	achievementHandler         *AchievementHandler
	contestRegistrationHandler *ContestRegistrationHandler
	paymentHandler  *PaymentHandler
}

func NewServer() *Server {
	// --- Initialize Repositories ---
	imgRepo := repository.NewImageRepostory("something")
	questionRepo := repository.NewQuestionDynamoRepository("eu-north-1", "question")
	contestRepo := repository.NewContestDynamoRepository("eu-north-1", "contests")
	studentRepo := repository.NewStudentDynamoRepository("eu-north-1", "student")
	submissionRepo := repository.NewSubmissionDynamoRepository("eu-north-1", "submissions")
	adminRepo := repository.NewAdminDynamoRepository("eu-north-1", "admin")
	notificationRepo := repository.NewNotificationDynamoRepository("eu-north-1", "notification")
	achievementRepo := repository.NewAchievementDynamoRepository("eu-north-1", "achievement")
	contestRegistrationRepo := repository.NewContestRegistrationDynamoRepository("eu-north-1", "contest_registeration")
	paymentRepo := repository.NewDynamoDBPaymentRepository("eu-north-1", "payment")

	// --- Initialize Use Cases ---
	contestUsecase := usecase.NewContestUsecase(contestRepo, questionRepo)
	studentUsecase := usecase.NewStudentUsecase(studentRepo)
	questionUsecase := usecase.NewQuestionUsecase(questionRepo)
	submissionUsecase := usecase.NewSubmissionUsecase(submissionRepo, contestUsecase , questionRepo)
	adminUsecase := usecase.NewAdminUsecase(adminRepo)
	notificationUsecase := usecase.NewNotificationUsecase(notificationRepo)
	achievementUsecase := usecase.NewAchievementUsecase(achievementRepo)
	contestRegistrationUsecase := usecase.NewContestRegistrationUsecase(contestRegistrationRepo)
	paymentUsecase := usecase.NewPaymentUsecases(paymentRepo)

	// --- Initialize Handlers ---
	server := &Server{
		contestHandler:             NewContestHandler(contestUsecase),
		studentHandler:             NewStudentHandler(studentUsecase),
		questionHandler:            NewQuestionHandler(questionUsecase, imgRepo), // Corrected line
		submissionHandler:          NewSubmissionHandler(submissionUsecase),
		adminHandler:               NewAdminHandler(adminUsecase),
		notificationHandler:        NewNotificationHandler(notificationUsecase),
		achievementHandler:         NewAchievementHandler(achievementUsecase),
		contestRegistrationHandler: NewContestRegistrationHandler(contestRegistrationUsecase),
		paymentHandler : NewPaymentHandler(paymentUsecase,*imgRepo),
	}
	return server
}

func (s *Server) NewRouter() *gin.Engine {
	r := gin.Default()
	r.Use(cors.New(cors.Config{
		AllowOrigins:     []string{"https://www.my-frontend.com", "http://localhost:5173","http://localhost:5174", "https://7wwb0knl-5173.euw.devtunnels.ms","https://victory-contest.vercel.app"},
		AllowMethods:     []string{"PUT", "PATCH", "POST", "GET", "DELETE"},
		AllowHeaders:     []string{"Origin", "Authorization", "Content-Type"},
		AllowCredentials: true,
	}))

	api := r.Group("/api")
	s.contestHandler.RegisterRoutes(api.Group("/contest"))
	s.studentHandler.RegisterRoutes(api.Group("/student"))
	s.questionHandler.RegisterRoutes(api.Group("/question"))
	s.submissionHandler.RegisterRoutes(api.Group("/submission"))
	s.adminHandler.RegisterRoutes(api.Group("/admin"))
	s.notificationHandler.RegisterRoutes(api.Group("/notification"))
	s.achievementHandler.RegisterRoutes(api.Group("/achievement"))
	s.contestRegistrationHandler.RegisterRoutes(api.Group("/contest-registration"))
	s.paymentHandler.RegisterRoutes(api.Group("/payment"))

	return r
}