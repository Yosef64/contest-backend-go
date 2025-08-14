package http

import (
	"victor-contest-go/internal/repository"
	"victor-contest-go/internal/usecase"

	"github.com/gin-contrib/cors"
	"github.com/gin-gonic/gin"
	tgbotapi "github.com/go-telegram-bot-api/telegram-bot-api/v5"
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
	feedbackQuestionHandler    *FeedbackQuestionHandler
	pollOptionHandler          *PollOptionHandler
	feedbackResponseHandler    *FeedbackResponseHandler
	paymentHandler             *PaymentHandler
	aiHandler                  *AiHandler
	telegramHandler            *telegramHandler
}

func NewServer() *Server {

	// Bot
	bot, _ := tgbotapi.NewBotAPI("7521099565:AAGDQx5aMWUOdidp5Vr8_eFHZH0dQOp3bYU")

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

	// --- Initialize Feedback Repositories ---
	feedbackQuestionRepo := repository.NewFeedbackQuestionDynamoRepository("eu-north-1", "feedback_questions")
	pollOptionRepo := repository.NewPollOptionDynamoRepository("eu-north-1", "poll_options")
	feedbackResponseRepo := repository.NewFeedbackResponseDynamoRepository("eu-north-1", "feedback_responses")

	// --- Initialize Use Cases ---
	contestUsecase := usecase.NewContestUsecase(contestRepo, questionRepo)
	studentUsecase := usecase.NewStudentUsecase(studentRepo, paymentRepo, submissionRepo, contestRepo)
	questionUsecase := usecase.NewQuestionUsecase(questionRepo)
	submissionUsecase := usecase.NewSubmissionUsecase(submissionRepo, contestUsecase, questionRepo, studentRepo)
	adminUsecase := usecase.NewAdminUsecase(adminRepo)
	notificationUsecase := usecase.NewNotificationUsecase(notificationRepo, contestRepo)
	achievementUsecase := usecase.NewAchievementUsecase(achievementRepo)
	contestRegistrationUsecase := usecase.NewContestRegistrationUsecase(contestRegistrationRepo)
	paymentUsecase := usecase.NewPaymentUsecases(paymentRepo)
	aiUsecase := usecase.NewAiUsecase(submissionRepo)
	telegramUsecase := usecase.NewTelegramUsecase(bot)

	// --- Initialize Feedback Use Cases ---
	feedbackQuestionUsecase := usecase.NewFeedbackQuestionUsecase(feedbackQuestionRepo)
	pollOptionUsecase := usecase.NewPollOptionUsecase(pollOptionRepo)
	feedbackResponseUsecase := usecase.NewFeedbackResponseUsecase(feedbackResponseRepo)

	// --- Initialize Notification Service ---
	notificationService := usecase.NewNotificationService(notificationRepo, studentRepo)

	// --- Initialize Handlers ---
	server := &Server{
		contestHandler:             NewContestHandler(contestUsecase, notificationService),
		studentHandler:             NewStudentHandler(studentUsecase, notificationUsecase),
		questionHandler:            NewQuestionHandler(questionUsecase, imgRepo), // Corrected line
		submissionHandler:          NewSubmissionHandler(submissionUsecase),
		adminHandler:               NewAdminHandler(adminUsecase),
		notificationHandler:        NewNotificationHandler(notificationUsecase),
		achievementHandler:         NewAchievementHandler(achievementUsecase),
		contestRegistrationHandler: NewContestRegistrationHandler(contestRegistrationUsecase),
		feedbackQuestionHandler:    NewFeedbackQuestionHandler(feedbackQuestionUsecase, notificationUsecase),
		pollOptionHandler:          NewPollOptionHandler(pollOptionUsecase),
		feedbackResponseHandler:    NewFeedbackResponseHandler(feedbackResponseUsecase, notificationUsecase),
		paymentHandler:             NewPaymentHandler(paymentUsecase, *imgRepo),
		aiHandler:                  NewAiHandler(aiUsecase),
		telegramHandler:            NewTelegramHandler(telegramUsecase),
	}
	return server
}

func (s *Server) NewRouter() *gin.Engine {
	r := gin.Default()
	r.Use(cors.New(cors.Config{
		AllowOrigins:     []string{"https://www.my-frontend.com", "http://localhost:5173", "http://localhost:5174", "http://localhost:3000", "http://localhost:3001", "https://7wwb0knl-5173.euw.devtunnels.ms", "https://victory-contest.vercel.app", "https://txnfqqn7-5173.euw.devtunnels.ms"},
		AllowMethods:     []string{"PUT", "PATCH", "POST", "GET", "DELETE", "OPTIONS"},
		AllowHeaders:     []string{"Origin", "Authorization", "Content-Type", "Accept", "X-Requested-With"},
		AllowCredentials: true,
		MaxAge:           12 * 60 * 60,
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
	s.feedbackQuestionHandler.RegisterRoutes(api.Group("/feedback-question"))
	s.pollOptionHandler.RegisterRoutes(api.Group("/poll-option"))
	s.feedbackResponseHandler.RegisterRoutes(api.Group("/feedback-response"))
	s.paymentHandler.RegisterRoutes(api.Group("/payment"))
	s.aiHandler.RegisterRoutes(api.Group("/ai"))
	s.telegramHandler.RegisterRoutes(api.Group("/telegram"))

	return r
}
