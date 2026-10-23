// Package api wires HTTP routes to handlers.
//
//	/healthz                  liveness + database check
//	/api/v1/auth/*            student sign-in (Google, Telegram, dev)
//	/api/v1/...               student app API (Bearer token)
//	/api/v1/admin/auth/*      admin sign-in (httpOnly cookie)
//	/api/v1/admin/...         admin dashboard API
package api

import (
	"context"
	"log/slog"
	"net/http"
	"time"

	"github.com/gin-contrib/cors"
	"github.com/gin-gonic/gin"
	"github.com/jackc/pgx/v5/pgxpool"

	"victor-contest-go/internal/ai"
	"victor-contest-go/internal/auth"
	"victor-contest-go/internal/config"
	"victor-contest-go/internal/httpx"
	"victor-contest-go/internal/media"
	"victor-contest-go/internal/live"
	"victor-contest-go/internal/push"
	"victor-contest-go/internal/store"
)

type Server struct {
	cfg    config.Config
	store  *store.Store
	tokens *auth.Tokens
	media  *media.Store
	pool   *pgxpool.Pool
	ai     *ai.Client
	push   *push.Sender
	rooms  *live.Rooms
	now    func() time.Time
}

func NewServer(cfg config.Config, pool *pgxpool.Pool, m *media.Store) *Server {
	sender, err := push.New(cfg.FirebaseProjectID, cfg.FirebaseClientEmail, cfg.FirebasePrivateKey)
	if err != nil {
		slog.Error("push notifications off", "err", err)
	}
	s := &Server{cfg: cfg, store: store.New(pool), tokens: auth.NewTokens(cfg.JWTSecret), media: m, pool: pool,
		ai: ai.New(cfg.AnthropicAPIKey, cfg.AIModel, cfg.AIBaseURL), push: sender, now: time.Now,
		rooms: live.New(cfg.FirebaseProjectID, cfg.FirebaseClientEmail, cfg.FirebasePrivateKey, cfg.FirestoreDatabase)}
	if pool != nil {
		s.tokens.SessionCheck = s.checkSession
		// Subjects come from the subjects table (admins add their own).
		subjectOK = func(name string, mixed bool) bool {
			ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
			defer cancel()
			return s.isSubject(ctx, name, mixed)
		}
	}
	return s
}

func (s *Server) Router() *gin.Engine {
	if !s.cfg.IsDev() {
		gin.SetMode(gin.ReleaseMode)
	}
	r := gin.New()
	r.Use(gin.Logger(), gin.Recovery(), httpx.ServerTime(time.Now), httpx.Gzip())
	_ = r.SetTrustedProxies(nil)
	r.MaxMultipartMemory = 16 << 20
	r.Use(cors.New(cors.Config{
		AllowOrigins:     s.cfg.AllowedOrigins,
		AllowMethods:     []string{"GET", "POST", "PUT", "PATCH", "DELETE", "OPTIONS"},
		AllowHeaders:     []string{"Origin", "Authorization", "Content-Type", "Accept"},
		AllowCredentials: true,
		ExposeHeaders:    []string{httpx.ServerTimeHeader},
		MaxAge:           12 * time.Hour,
	}))
	r.NoRoute(func(c *gin.Context) { httpx.Fail(c, httpx.NotFound("Endpoint")) })

	r.GET("/healthz", s.health)
	if dir := s.media.LocalDir(); dir != "" {
		r.Static("/uploads", dir)
	}

	// Public page behind shared referral links.
	r.GET("/r/:owner/:chapter", s.referralLanding)
	r.GET("/a/:code", s.agentLanding)
	r.GET("/c/:owner/:challenge/:kind/:unit", s.challengeInviteLanding)
	r.GET("/account-deletion", s.accountDeletionPage)
	r.GET("/privacy", s.privacyPage)

	v1 := r.Group("/api/v1")
	v1.GET("/site", httpx.H(s.publicSite))
	v1.GET("/subjects", httpx.H(s.publicSubjects))
	// Chapa calls these; each one only triggers a verification with Chapa.
	v1.GET("/payments/chapa/callback", s.chapaCallback)
	v1.POST("/payments/chapa/webhook", s.chapaWebhook)

	// Student sign-in.
	a := v1.Group("/auth")
	a.POST("/google", httpx.H(s.signInGoogle))
	a.POST("/telegram", httpx.H(s.signInTelegram))
	a.GET("/telegram/login", s.telegramLoginPage)
	a.GET("/telegram/return", s.telegramLoginReturn)
	a.POST("/register", httpx.H(s.signUpPassword))
	a.POST("/login", httpx.H(s.signInPassword))
	a.POST("/password/forgot", httpx.H(s.forgotPassword))
	a.POST("/password/reset", httpx.H(s.resetPassword))
	if s.cfg.IsDev() {
		a.POST("/dev", httpx.H(s.signInDev))
	}

	// Student app.
	app := v1.Group("", s.tokens.Require(auth.RoleStudent))
	app.GET("/me", httpx.H(s.getMe))
	app.PUT("/me", httpx.H(s.putMe))
	app.DELETE("/me", httpx.H(s.deleteMyAccount))
	app.GET("/me/sessions", httpx.H(s.mySessions))
	app.DELETE("/me/sessions/:id", httpx.H(s.endMySession))
	app.PUT("/me/password", httpx.H(s.changeStudentPassword))
	app.PUT("/me/photo", httpx.H(s.putMyPhoto))
	app.DELETE("/me/photo", httpx.H(s.deleteMyPhoto))
	app.GET("/me/stats", httpx.H(s.getMyStats))
	app.POST("/me/checkin", httpx.H(s.appCheckin))
	app.GET("/me/achievements", httpx.H(s.appAchievements))
	app.GET("/me/progress", httpx.H(s.getMyProgress))
	app.PUT("/me/progress/:chapterID", httpx.H(s.putMyProgress))
	app.PUT("/me/progress/:chapterID/topics", httpx.H(s.putMyTopic))
	app.GET("/courses", httpx.H(s.appCourses))
	app.GET("/courses/:id", httpx.H(s.appCourse))
	app.GET("/chapters/:id", httpx.H(s.appChapter))
	app.GET("/trainings", httpx.H(s.appTrainings))
	app.GET("/trainings/:id", httpx.H(s.appTraining))
	app.PUT("/trainings/:id/progress", httpx.H(s.appTrainingProgress))
	app.GET("/contests", httpx.H(s.appContests))
	app.GET("/contests/:id", httpx.H(s.appContest))
	app.POST("/contests/:id/register", httpx.H(s.appRegister))
	app.GET("/contests/:id/questions", httpx.H(s.appContestQuestions))
	app.POST("/contests/:id/submit", httpx.H(s.appSubmit))
	app.GET("/contests/:id/offline", httpx.H(s.appOfflinePaper))
	app.POST("/contests/:id/offline-submit", httpx.H(s.appOfflineSubmit))
	app.GET("/contests/:id/editorial", httpx.H(s.appEditorial))
	app.GET("/contests/:id/standings", httpx.H(s.appStandings))
	app.GET("/leaderboard", httpx.H(s.appLeaderboard))
	app.GET("/students/:id/profile", httpx.H(s.appStudentProfile))
	app.GET("/ai/status", httpx.H(s.aiStatus))
	app.POST("/ai/tutor", httpx.H(s.aiTutor))
	app.GET("/ai/analysis", httpx.H(s.aiAnalysis))
	app.POST("/ai/review-quiz", httpx.H(s.aiReviewQuiz))
	app.GET("/payments/options", httpx.H(s.paymentOptions))
	app.POST("/payments/chapa", httpx.H(s.startChapaPayment))
	app.GET("/payments/chapa/:tx_ref", httpx.H(s.checkChapaPayment))
	app.GET("/group-exams", httpx.H(s.listGroupExams))
	app.POST("/group-exams", httpx.H(s.createGroupExam))
	app.POST("/group-exams/join", httpx.H(s.joinGroupExam))
	app.GET("/group-exams/:id", httpx.H(s.getGroupExam))
	app.POST("/group-exams/:id/start", httpx.H(s.startGroupExam))
	app.POST("/group-exams/:id/leave", httpx.H(s.leaveGroupExam))
	app.PUT("/group-exams/:id/answers", httpx.H(s.saveGroupAnswers))
	app.POST("/group-exams/:id/submit", httpx.H(s.submitGroupExam))
	app.PUT("/group-exams/:id/settings", httpx.H(s.updateGroupSettings))
	app.POST("/group-exams/:id/end", httpx.H(s.endGroupExam))
	app.DELETE("/group-exams/:id", httpx.H(s.deleteGroupExam))
	app.POST("/group-exams/:id/again", httpx.H(s.runGroupExamAgain))

	app.POST("/contests/:id/exam-mode/:action", httpx.H(s.examModePassword))
	app.POST("/contests/:id/exam-violations", httpx.H(s.appExamViolation))
	app.GET("/me/agent", httpx.H(s.appAgent))
	app.PUT("/me/agent/payout", httpx.H(s.appAgentPayout))
	app.POST("/me/agent-code", httpx.H(s.appUseAgentCode))
	app.POST("/contests/:id/makeup", httpx.H(s.startMakeup))
	app.POST("/contests/:id/makeup/submit", httpx.H(s.submitMakeup))
	app.POST("/learning-hub/agent", httpx.H(s.registerAgent))
	app.GET("/learning-hub", httpx.H(s.learningHub))
	app.GET("/announcements", httpx.H(s.appAnnouncements))
	app.GET("/study-sessions", httpx.H(s.listStudySessions))
	app.POST("/study-sessions", httpx.H(s.createStudySession))
	app.PUT("/study-sessions/:id", httpx.H(s.updateStudySession))
	app.DELETE("/study-sessions/:id", httpx.H(s.deleteStudySession))
	app.PUT("/learning-hub/profile", httpx.H(s.saveAcademicProfile))
	app.POST("/learning-hub/enrollments", httpx.H(s.requestEnrollment))
	app.PUT("/learning-hub/targets", httpx.H(s.saveTarget))
	app.PUT("/learning-hub/habits/:id", httpx.H(s.logHabit))
	app.POST("/learning-hub/referrals", httpx.H(s.referralVisit))
	app.GET("/challenges", httpx.H(s.appChallenges))
	app.POST("/challenges/referrals", httpx.H(s.appChallengeReferral))
	app.GET("/challenges/:id", httpx.H(s.appChallenge))
	app.GET("/challenges/:id/days/:day", httpx.H(s.appChallengeDay))
	app.POST("/challenges/:id/days/:day/steps/:step", httpx.H(s.appChallengeStep))
	app.GET("/challenges/:id/days/:day/leaderboard", httpx.H(s.appChallengeLeaderboard))

	// Admin sign-in (public).
	adminAuth := v1.Group("/admin/auth")
	adminAuth.POST("/login", httpx.H(s.adminLogin))
	adminAuth.POST("/google", httpx.H(s.adminGoogle))
	adminAuth.POST("/logout", httpx.H(s.adminLogout))
	adminAuth.POST("/register", httpx.H(s.adminRegister))

	// Admin dashboard.
	ad := v1.Group("/admin", s.tokens.Require(auth.RoleAdmin), s.requireApprovedAdmin, s.requirePermission)
	ad.GET("/auth/me", httpx.H(s.adminMe))
	ad.GET("/overview", httpx.H(s.adminOverview))
	ad.PUT("/services/exam-mode", httpx.H(s.adminExamMode))
	ad.PUT("/services/makeup", httpx.H(s.adminMakeup))
	ad.DELETE("/services/makeup/:id", httpx.H(s.adminCloseMakeup))
	ad.PATCH("/services/agents/:id", httpx.H(s.adminAgentReview))
	ad.POST("/services/chapters/import", httpx.H(s.adminBulkChapters))
	ad.POST("/services/chapters/convert", httpx.H(s.adminConvertChapters))
	ad.GET("/services", httpx.H(s.adminServices))
	ad.POST("/services", httpx.H(s.adminServiceWrite))
	ad.GET("/services/reports", httpx.H(s.adminReports))
	ad.GET("/services/reports/master", httpx.H(s.adminMasterReport))
	ad.GET("/services/reports/ai-summary", httpx.H(s.aiReportSummary))
	ad.PUT("/enrollments/:id", httpx.H(s.adminEditEnrollment))
	ad.PUT("/auth/password", httpx.H(s.changeAdminPassword))
	ad.GET("/settings/site", httpx.H(s.adminSiteSettings))
	ad.PUT("/settings/site", httpx.H(s.adminSaveSiteSettings))

	ad.GET("/trainings", httpx.H(s.adminTrainings))
	ad.POST("/trainings", httpx.H(s.adminCreateTraining))
	ad.PUT("/trainings/order", httpx.H(s.adminReorderTrainings))
	ad.GET("/trainings/:id", httpx.H(s.adminTraining))
	ad.PUT("/trainings/:id", httpx.H(s.adminSaveTraining))
	ad.DELETE("/trainings/:id", httpx.H(s.adminDeleteTraining))
	ad.GET("/courses", httpx.H(s.adminCourses))
	ad.POST("/courses", httpx.H(s.adminCreateCourse))
	ad.GET("/courses/:id", httpx.H(s.adminCourse))
	ad.PUT("/courses/:id", httpx.H(s.adminUpdateCourse))
	ad.DELETE("/courses/:id", httpx.H(s.adminDeleteCourse))
	ad.POST("/courses/:id/chapters", httpx.H(s.adminCreateChapter))
	ad.PUT("/courses/:id/chapters/order", httpx.H(s.adminReorderChapters))

	ad.GET("/chapters/:id", httpx.H(s.adminChapter))
	ad.PUT("/chapters/:id", httpx.H(s.adminSaveChapter))
	ad.DELETE("/chapters/:id", httpx.H(s.adminDeleteChapter))
	ad.POST("/chapters/:id/lock", httpx.H(s.adminLockChapter))
	ad.DELETE("/chapters/:id/lock", httpx.H(s.adminUnlockChapter))
	ad.GET("/chapters/:id/versions", httpx.H(s.adminVersions))
	ad.GET("/chapters/:id/versions/:v", httpx.H(s.adminVersion))
	ad.POST("/chapters/:id/versions/:v/restore", httpx.H(s.adminRestoreVersion))
	ad.GET("/chapters/:id/comments", httpx.H(s.adminComments))
	ad.POST("/chapters/:id/comments", httpx.H(s.adminAddComment))
	ad.PATCH("/comments/:id", httpx.H(s.adminResolveComment))
	ad.POST("/media", httpx.H(s.adminUpload))
	ad.GET("/subjects", httpx.H(s.adminSubjects))
	ad.POST("/subjects", httpx.H(s.adminCreateSubject))
	ad.PATCH("/subjects/:name", httpx.H(s.adminUpdateSubject))
	ad.DELETE("/subjects/:name", httpx.H(s.adminDeleteSubject))
	ad.POST("/media/sign", httpx.H(s.adminSignUpload))
	ad.POST("/media/register", httpx.H(s.adminRegisterUpload))
	ad.GET("/media/:id/uses", httpx.H(s.adminMediaUses))
	ad.DELETE("/media/:id", httpx.H(s.adminDeleteMedia))
	ad.GET("/media", httpx.H(s.adminMedia))

	ad.GET("/contests", httpx.H(s.adminContests))
	ad.POST("/contests", httpx.H(s.adminCreateContest))
	ad.GET("/contests/:id", httpx.H(s.adminContest))
	ad.PUT("/contests/:id", httpx.H(s.adminUpdateContest))
	ad.DELETE("/contests/:id", httpx.H(s.adminDeleteContest))
	ad.GET("/contests/:id/standings", httpx.H(s.adminStandings))
	ad.GET("/contests/:id/violations", httpx.H(s.adminExamViolations))
	ad.GET("/contests/:id/questions", httpx.H(s.adminContestQuestions))
	ad.POST("/contests/:id/questions/import", httpx.H(s.adminImportContestQuestions))

	ad.GET("/challenges", httpx.H(s.adminChallenges))
	ad.PUT("/challenge-settings", httpx.H(s.adminChallengeSettings))
	ad.POST("/challenges", httpx.H(s.adminCreateChallenge))
	ad.GET("/challenges/:id", httpx.H(s.adminChallenge))
	ad.PUT("/challenges/:id", httpx.H(s.adminUpdateChallenge))
	ad.DELETE("/challenges/:id", httpx.H(s.adminDeleteChallenge))
	ad.GET("/challenges/:id/report", httpx.H(s.adminChallengeReport))
	ad.GET("/challenges/:id/days/:day", httpx.H(s.adminChallengeDay))
	ad.PUT("/challenges/:id/days/:day", httpx.H(s.adminSaveChallengeDay))
	ad.PUT("/challenges/:id/days/:day/quiz", httpx.H(s.adminSaveChallengeQuiz))
	ad.DELETE("/challenges/:id/days/:day/quiz", httpx.H(s.adminDeleteChallengeQuiz))

	ad.GET("/questions", httpx.H(s.adminQuestions))
	ad.POST("/questions", httpx.H(s.adminCreateQuestion))
	ad.POST("/questions/bulk", httpx.H(s.adminBulkQuestions))
	ad.POST("/questions/batch", httpx.H(s.adminQuestionBatch))
	ad.PUT("/questions/:id", httpx.H(s.adminUpdateQuestion))
	ad.DELETE("/questions/:id", httpx.H(s.adminDeleteQuestion))

	ad.GET("/students", httpx.H(s.adminStudents))
	ad.GET("/students/:id", httpx.H(s.adminStudent))
	ad.GET("/students/:id/insights", httpx.H(s.adminStudentInsights))
	ad.GET("/agents", httpx.H(s.adminAgents))
	ad.PATCH("/agents/:id", httpx.H(s.adminUpdateAgent))
	ad.POST("/agents/:id/payout", httpx.H(s.adminAgentPayout))
	ad.GET("/agents/:id", httpx.H(s.adminAgentDetail))
	ad.PATCH("/students/:id", httpx.H(s.adminPatchStudent))
	ad.POST("/students/:id/password-reset", httpx.H(s.adminPasswordReset))
	ad.DELETE("/students/:id", httpx.H(s.adminDeleteStudent))

	ad.GET("/admins", httpx.H(s.adminAdmins))
	ad.PATCH("/admins/:id", httpx.H(s.adminPatchAdmin))
	ad.DELETE("/admins/:id", httpx.H(s.adminDeleteAdmin))
	ad.POST("/admins/:id/password-reset", httpx.H(s.adminResetAdminPassword))

	return r
}

func (s *Server) health(c *gin.Context) {
	ctx, cancel := context.WithTimeout(c.Request.Context(), 3*time.Second)
	defer cancel()
	if err := s.pool.Ping(ctx); err != nil {
		c.JSON(http.StatusServiceUnavailable, gin.H{"status": "degraded", "database": "unreachable"})
		return
	}
	c.JSON(http.StatusOK, gin.H{"status": "ok"})
}
