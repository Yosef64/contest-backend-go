package api

import (
	"errors"
	"log/slog"
	"strconv"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/jackc/pgx/v5"

	"victor-contest-go/internal/auth"
	"victor-contest-go/internal/httpx"
	"victor-contest-go/internal/store"
)

// Sign-in ----------------------------------------------------------------------

type account struct {
	Provider    string `json:"provider"`
	ID          string `json:"id"`
	DisplayName string `json:"display_name"`
	Email       string `json:"email,omitempty"`
	Username    string `json:"username,omitempty"`
	PhotoURL    string `json:"photo_url,omitempty"`
}

// completeSignIn issues an app token and returns the existing profile, if
// any. A null student means the app should show profile setup.
func (s *Server) completeSignIn(c *gin.Context, acc account, studentID string) error {
	photo := signInPhoto(acc)
	token, err := s.tokens.IssueStudent(studentID, acc.DisplayName, acc.Provider, acc.ID, photo, s.cfg.StudentTokenTTL)
	if err != nil {
		return err
	}
	var student *store.Student
	st, err := s.store.StudentByIdentity(c, acc.Provider, acc.ID)
	if errors.Is(err, store.ErrNotFound) {
		// Signed in another way than the account was made with (Google
		// and a password on one email).
		st, err = s.store.StudentByID(c, studentID)
	}
	switch {
	case err == nil:
		st = s.syncSignInPhoto(c, st, photo)
		student = &st
	case errors.Is(err, store.ErrNotFound):
	default:
		return err
	}
	if student != nil && student.IsSuspended {
		return httpx.Forbidden("This account has been suspended. Contact Victory support.")
	}
	return httpx.OK(c, gin.H{"token": token, "account": acc, "student": student})
}

// signInPhoto is the Google or Telegram account picture, at a size fit
// for the profile (Google serves 96 px by default).
func signInPhoto(acc account) string {
	if (acc.Provider != "google" && acc.Provider != "telegram") || !strings.HasPrefix(acc.PhotoURL, "https://") {
		return ""
	}
	if i := strings.LastIndex(acc.PhotoURL, "=s96-c"); i > 0 && acc.Provider == "google" {
		return acc.PhotoURL[:i] + "=s256-c"
	}
	return acc.PhotoURL
}

// syncSignInPhoto keeps the profile on the sign-in picture until the
// student picks their own.
func (s *Server) syncSignInPhoto(c *gin.Context, st store.Student, photo string) store.Student {
	if photo == "" || st.PhotoCustom || st.PhotoURL == photo {
		return st
	}
	if synced, err := s.store.SyncSignInPhoto(c, st.ID, photo); err == nil {
		return synced
	}
	return st
}

func (s *Server) signInGoogle(c *gin.Context) error {
	var in struct {
		IDToken string `json:"id_token" binding:"required"`
	}
	if err := httpx.Bind(c, &in); err != nil {
		return err
	}
	u, err := auth.VerifyGoogleIDToken(c, in.IDToken, s.cfg.GoogleClientIDs)
	if err != nil || !u.Verified {
		return httpx.Unauthorized("Google sign-in could not be verified.")
	}
	studentID, err := s.googleStudent(c, strings.ToLower(strings.TrimSpace(u.Email)), u.Subject)
	if err != nil {
		return err
	}
	return s.completeSignIn(c, account{Provider: "google", ID: u.Subject, DisplayName: u.Name, Email: u.Email, PhotoURL: u.Picture}, studentID)
}

// googleStudent is the account a Google sign-in opens: one email is one
// Victory account. The first Google sign-in for an email that already has
// a password account (and no Google account of its own) opens that account.
// A password nobody proved owning the email for is removed then, so someone
// who registered another person's email first can't share their account.
func (s *Server) googleStudent(c *gin.Context, email, subject string) (string, error) {
	googleID := "g_" + subject
	if email == "" {
		return googleID, nil
	}
	var studentID string
	err := pgx.BeginFunc(c, s.pool, func(tx pgx.Tx) error {
		err := tx.QueryRow(c, `SELECT student_id FROM google_accounts WHERE email = $1`, email).Scan(&studentID)
		if err == nil || !errors.Is(err, pgx.ErrNoRows) {
			return err
		}
		studentID = googleID
		var hasOwn bool
		if err := tx.QueryRow(c, `SELECT EXISTS (SELECT 1 FROM students WHERE id = $1 OR (auth_provider = 'google' AND auth_subject = $2))`,
			googleID, subject).Scan(&hasOwn); err != nil {
			return err
		}
		if !hasOwn {
			var passwordStudent string
			var proven bool
			err := tx.QueryRow(c, `SELECT student_id, email_verified_at IS NOT NULL FROM student_credentials WHERE login = $1`, email).
				Scan(&passwordStudent, &proven)
			switch {
			case err == nil:
				studentID = passwordStudent
				if !proven {
					if _, err := tx.Exec(c, `DELETE FROM student_credentials WHERE login = $1`, email); err != nil {
						return err
					}
					slog.Info("google sign-in took over an unproven password account", "student", passwordStudent)
				}
			case !errors.Is(err, pgx.ErrNoRows):
				return err
			}
		}
		if _, err := tx.Exec(c, `INSERT INTO google_accounts (email, student_id) VALUES ($1, $2) ON CONFLICT (email) DO NOTHING`,
			email, studentID); err != nil {
			return err
		}
		return tx.QueryRow(c, `SELECT student_id FROM google_accounts WHERE email = $1`, email).Scan(&studentID)
	})
	return studentID, err
}

func (s *Server) signInTelegram(c *gin.Context) error {
	var in map[string]string
	if err := c.ShouldBindJSON(&in); err != nil {
		return httpx.BadRequest("Send the Telegram login fields as a JSON object of strings.")
	}
	u, err := auth.VerifyTelegramLogin(s.cfg.TelegramBotToken, in, 24*time.Hour)
	if err != nil {
		return httpx.Unauthorized("Telegram sign-in could not be verified.")
	}
	name := strings.TrimSpace(u.FirstName + " " + u.LastName)
	// Telegram students keep their Telegram ID as student ID, so profiles
	// imported from the old mini app are found automatically.
	return s.completeSignIn(c, account{Provider: "telegram", ID: u.ID, DisplayName: name, Username: u.Username, PhotoURL: u.PhotoURL}, u.ID)
}

// signInDev exists only in development, for testing without Google/Telegram.
func (s *Server) signInDev(c *gin.Context) error {
	var in struct {
		ID   string `json:"id" binding:"required,max=64"`
		Name string `json:"name" binding:"required,max=80"`
	}
	if err := httpx.Bind(c, &in); err != nil {
		return err
	}
	return s.completeSignIn(c, account{Provider: "dev", ID: in.ID, DisplayName: in.Name}, "dev_"+in.ID)
}

// Profile ----------------------------------------------------------------------

func (s *Server) me(c *gin.Context) (store.Student, error) {
	st, err := s.store.StudentByID(c, auth.Caller(c).Subject)
	if errors.Is(err, store.ErrNotFound) {
		return st, httpx.NewError(404, "profile_required", "Complete your profile first.")
	}
	if err == nil && st.IsSuspended {
		return st, httpx.Forbidden("This account has been suspended. Contact Victory support.")
	}
	return st, err
}

func (s *Server) getMe(c *gin.Context) error {
	st, err := s.me(c)
	if err != nil {
		return err
	}
	st = s.syncSignInPhoto(c, st, auth.Caller(c).Photo)
	academic, err := s.academicProfile(c, st.ID)
	if err != nil {
		return err
	}
	return httpx.OK(c, gin.H{"student": st, "academic": academic})
}

// academicProfile returns the student's department and stream (registration
// fields required by the academy) and partner school, if any.
func (s *Server) academicProfile(c *gin.Context, studentID string) (gin.H, error) {
	var department, stream string
	var school *string
	err := s.pool.QueryRow(c, `SELECT department, stream, school_id FROM academic_profiles WHERE student_id = $1`, studentID).
		Scan(&department, &stream, &school)
	if errors.Is(err, store.ErrNotFound) {
		return gin.H{"department": "", "stream": "", "school_id": nil}, nil
	}
	if err != nil {
		return nil, err
	}
	return gin.H{"department": department, "stream": stream, "school_id": school}, nil
}

// academicLevel reconciles a student's grade, department and stream. High
// school students in grades 9-11 belong to no department; grade 12 students
// prepare for the national entrance exam; remedial and freshman students are
// university-level and are stored as grade 12. A stream applies from grade 11.
func academicLevel(grade, department, stream string) (string, string, string, map[string]string) {
	fields := map[string]string{}
	switch department {
	case "remedial", "freshman":
		grade = "12"
	case "entrance":
		// Older app builds sent "entrance" for every grade: it only means
		// something for grade 12 and must never promote a high-school
		// student.
		if grade == "" {
			grade = "12"
		} else if grade != "12" {
			department = ""
		}
	case "":
		if grade == "12" {
			department = "entrance"
		}
	default:
		fields["department"] = "Choose Remedial or Freshman, or leave empty for high school."
	}
	if grade == "" {
		fields["grade"] = "Choose grade 9, 10, 11 or 12."
	}
	switch {
	case grade == "9" || grade == "10":
		stream = ""
	case stream != "natural" && stream != "social":
		fields["stream"] = "Choose Natural or Social Science."
	}
	return grade, department, stream, fields
}

func (s *Server) putMe(c *gin.Context) error {
	var in struct {
		Name        string `json:"name" binding:"required,min=2,max=80"`
		PhoneNumber string `json:"phone_number" binding:"required,min=10,max=13"`
		Gender      string `json:"gender" binding:"required,oneof=male female"`
		Age         int    `json:"age" binding:"required,min=5,max=99"`
		School      string `json:"school" binding:"required,min=3,max=120"`
		Grade       string `json:"grade" binding:"required"`
		City        string `json:"city" binding:"required,min=2,max=80"`
		Region      string `json:"region" binding:"required,max=80"`
		// Empty for grades 9-11 (validated by academicLevel). When both are
		// omitted on an update the stored academic profile is kept.
		Department *string `json:"department"`
		Stream     *string `json:"stream"`
	}
	if err := httpx.Bind(c, &in); err != nil {
		return err
	}
	var department, stream string
	if in.Department != nil {
		department = *in.Department
	}
	if in.Stream != nil {
		stream = *in.Stream
	}
	claims := auth.Caller(c)
	existing, err := s.store.StudentByID(c, claims.Subject)
	// A new profile starts with the sign-in picture.
	photo := claims.Photo
	if err == nil {
		if existing.IsSuspended {
			return httpx.Forbidden("This account has been suspended.")
		}
		photo = existing.PhotoURL
		if in.Department == nil && in.Stream == nil {
			academic, err := s.academicProfile(c, existing.ID)
			if err != nil {
				return err
			}
			department, _ = academic["department"].(string)
			stream, _ = academic["stream"].(string)
		}
	} else if !errors.Is(err, store.ErrNotFound) {
		return err
	}
	grade, department, stream, fields := academicLevel(NormalizeGrade(in.Grade), department, stream)
	if len(fields) > 0 {
		return httpx.Invalid(fields)
	}
	st, err := s.store.UpsertStudentProfile(c, store.Student{
		ID: claims.Subject, AuthProvider: claims.Provider, AuthSubject: claims.Identity,
		Name: strings.TrimSpace(in.Name), PhoneNumber: strings.TrimSpace(in.PhoneNumber), Gender: in.Gender, Age: in.Age,
		School: strings.TrimSpace(in.School), Grade: grade, City: strings.TrimSpace(in.City), Region: strings.TrimSpace(in.Region), PhotoURL: photo,
	})
	if err != nil {
		return storeErr(err, "Profile")
	}
	_, err = s.pool.Exec(c, `INSERT INTO academic_profiles(student_id, department, stream) VALUES($1, $2, $3)
		ON CONFLICT (student_id) DO UPDATE SET department = $2, stream = $3, updated_at = now()`, st.ID, department, stream)
	if err != nil {
		return err
	}
	academic, err := s.academicProfile(c, st.ID)
	if err != nil {
		return err
	}
	return httpx.OK(c, gin.H{"student": st, "academic": academic})
}

func (s *Server) getMyStats(c *gin.Context) error {
	st, err := s.me(c)
	if err != nil {
		return err
	}
	stats, err := s.store.StudentStats(c, st.ID)
	if err != nil {
		return err
	}
	return httpx.OK(c, gin.H{"stats": stats})
}

func (s *Server) getMyProgress(c *gin.Context) error {
	st, err := s.me(c)
	if err != nil {
		return err
	}
	ids, err := s.store.CompletedChapters(c, st.ID)
	if err != nil {
		return err
	}
	return httpx.OK(c, gin.H{"completed_chapter_ids": ids})
}

func (s *Server) putMyProgress(c *gin.Context) error {
	var in struct {
		Completed bool `json:"completed"`
	}
	if err := httpx.Bind(c, &in); err != nil {
		return err
	}
	st, err := s.me(c)
	if err != nil {
		return err
	}
	return orNoContent(c, storeErr(s.store.SetChapterCompleted(c, st.ID, c.Param("chapterID"), in.Completed), "Chapter"))
}

// Courses ----------------------------------------------------------------------

func (s *Server) appCourses(c *gin.Context) error {
	// A grade (9-12) or university level (remedial, freshman); by default
	// the student's own: remedial and freshman students see their courses.
	grade := questionLevel(c.Query("grade"))
	if grade == "" {
		st, err := s.me(c)
		if err != nil {
			return err
		}
		grade = st.Grade
		academic, err := s.academicProfile(c, st.ID)
		if err != nil {
			return err
		}
		if d, _ := academic["department"].(string); d == "remedial" || d == "freshman" {
			grade = d
		}
	}
	courses, err := s.store.Courses(c, grade, true)
	if err != nil {
		return err
	}
	type withChapters struct {
		store.Course
		Chapters []store.ChapterSummary `json:"chapters"`
	}
	st, err := s.me(c)
	if err != nil {
		return err
	}
	out := make([]withChapters, 0, len(courses))
	ids := make([]string, 0, len(courses))
	for _, co := range courses {
		if e := s.checkLearningGroup(c, st, "course", co.ID); e != nil {
			continue
		}
		chs, err := s.store.Chapters(c, co.ID, true)
		if err != nil {
			return err
		}
		out = append(out, withChapters{co, chs})
		ids = append(ids, co.ID)
	}
	locks, err := s.chapterLocks(c, st, ids...)
	if err != nil {
		return err
	}
	access, err := s.accessFor(c, st, "course", ids)
	if err != nil {
		return err
	}
	return httpx.OKCached(c, gin.H{"courses": out, "locks": locks, "access": access})
}

func (s *Server) appCourse(c *gin.Context) error {
	co, err := s.store.Course(c, c.Param("id"))
	if err != nil || co.PublishedCount == 0 {
		return httpx.NotFound("Course")
	}
	st, e := s.me(c)
	if e != nil {
		return e
	}
	if e = s.checkLearningGroup(c, st, "course", co.ID); e != nil {
		return e
	}
	chs, err := s.store.Chapters(c, co.ID, true)
	if err != nil {
		return err
	}
	locks, err := s.chapterLocks(c, st, co.ID)
	if err != nil {
		return err
	}
	access, err := s.accessFor(c, st, "course", []string{co.ID})
	if err != nil {
		return err
	}
	return httpx.OKCached(c, gin.H{"course": co, "chapters": chs, "locks": locks, "access": access})
}

func (s *Server) appChapter(c *gin.Context) error {
	ch, err := s.store.Chapter(c, c.Param("id"))
	if err != nil || ch.Status != "published" {
		return httpx.NotFound("Chapter")
	}
	st, e := s.me(c)
	if e != nil {
		return e
	}
	// The course limits who may see it; the chapter's own access (premium
	// unless made free) decides whether it opens.
	if e = s.checkLearningGroup(c, st, "course", ch.CourseID); e != nil {
		return e
	}
	if e = s.checkLearningAccess(c, st, "chapter", ch.ID); e != nil {
		return e
	}
	ch.Lock = nil
	until, e := s.offlineExpiry(c, st, ch.CourseID, ch.ID)
	if e != nil {
		return e
	}
	// The lease is renewed on every request, so it is sent as a header too
	// (304 responses have no body); the ETag follows the chapter version.
	c.Header("X-Offline-Until", until.UTC().Format(time.RFC3339))
	return httpx.OKCached(c, gin.H{"chapter": ch, "offline_until": until}, ch.ID, strconv.Itoa(ch.Version))
}

// Contests ---------------------------------------------------------------------

type appContest struct {
	ExamMode bool `json:"exam_mode"`
	// Exam-mode breaks before the app submits the exam (0: never), and
	// this student's breaks so far (so a restarted app still knows).
	MaxViolations int `json:"max_violations"`
	Violations    int `json:"violations"`
	resourceAccess
	store.Contest
	QuestionIDs   []string           `json:"question_ids,omitempty"`
	QuestionCount int                `json:"question_count"`
	Participants  []string           `json:"participants"`
	IsRegistered  bool               `json:"is_registered"`
	Submission    *submissionSummary `json:"submission"`
	// Deadline of a started national exam attempt.
	AttemptEndsAt *time.Time `json:"attempt_ends_at,omitempty"`
}

// practiceAttempt returns the student's timed attempt at a national exam set,
// starting it when start is true. The deadline never moves on resume.
func (s *Server) practiceAttempt(c *gin.Context, co store.Contest, studentID string, start bool) (started, ends time.Time, err error) {
	if !start {
		err = s.pool.QueryRow(c, "SELECT started_at, ends_at FROM practice_attempts WHERE contest_id=$1 AND student_id=$2", co.ID, studentID).Scan(&started, &ends)
		return
	}
	now := s.now()
	ends = now.Add(time.Duration(*co.DurationMinutes) * time.Minute)
	if ends.After(co.EndTime) {
		ends = co.EndTime
	}
	err = s.pool.QueryRow(c, `INSERT INTO practice_attempts(contest_id, student_id, started_at, ends_at) VALUES($1, $2, $3, $4)
		ON CONFLICT (contest_id, student_id) DO UPDATE SET contest_id = EXCLUDED.contest_id
		RETURNING started_at, ends_at`, co.ID, studentID, now, ends).Scan(&started, &ends)
	return
}

// revealScore reports whether a submission's score may be shown: national
// practice sets are scored immediately, contests once they end.
func revealScore(co store.Contest) bool { return co.IsNational() || co.Phase == "ended" }

type submissionSummary struct {
	Answered    int       `json:"answered"`
	Correct     *int      `json:"correct,omitempty"` // hidden until the contest ends
	Total       int       `json:"total"`
	TimeSeconds int       `json:"time_seconds"`
	SubmittedAt time.Time `json:"submitted_at"`
}

func summarize(sub store.Submission, ended bool) *submissionSummary {
	out := &submissionSummary{Answered: len(sub.Answers), Total: sub.Total, TimeSeconds: sub.TimeSeconds, SubmittedAt: sub.SubmittedAt}
	if ended {
		correct := sub.Correct
		out.Correct = &correct
	}
	return out
}

func (s *Server) appContests(c *gin.Context) error {
	st, err := s.me(c)
	if err != nil {
		return err
	}
	academic, err := s.academicProfile(c, st.ID)
	if err != nil {
		return err
	}
	department, _ := academic["department"].(string)
	contests, err := s.store.Contests(c, st.Grade, department)
	if err != nil {
		return err
	}
	myStream, _ := academic["stream"].(string)
	registered, submitted, err := s.store.StudentContestState(c, st.ID)
	if err != nil {
		return err
	}
	participants, err := s.store.ContestParticipants(c, 8)
	if err != nil {
		return err
	}
	ids := make([]string, len(contests))
	for i, co := range contests {
		ids[i] = co.ID
	}
	access, err := s.accessFor(c, st, "contest", ids)
	if err != nil {
		return err
	}
	// Which exams run in exam mode: the app locks the phone for them.
	examMode := map[string]bool{}
	maxViolations := map[string]int{}
	breaks, err := s.studentBreaks(c, st.ID)
	if err != nil {
		return err
	}
	rows, err := s.pool.Query(c, "SELECT contest_id, max_violations FROM exam_mode_settings WHERE enabled AND contest_id = ANY($1)", ids)
	if err != nil {
		return err
	}
	for rows.Next() {
		var id string
		var limit int
		if err := rows.Scan(&id, &limit); err != nil {
			rows.Close()
			return err
		}
		examMode[id], maxViolations[id] = true, limit
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return err
	}
	out := make([]appContest, 0, len(contests))
	for _, co := range contests {
		if e := s.checkLearningGroup(c, st, "contest", co.ID); e != nil {
			continue
		}
		if co.Stream != "" && co.Stream != myStream {
			continue
		}
		if co.IsNational() && len(co.QuestionIDs) == 0 {
			continue // still being prepared
		}
		ac := appContest{Contest: co, resourceAccess: access[co.ID], QuestionCount: len(co.QuestionIDs), Participants: participants[co.ID], IsRegistered: registered[co.ID], ExamMode: examMode[co.ID], MaxViolations: maxViolations[co.ID], Violations: breaks[co.ID]}
		if ac.Participants == nil {
			ac.Participants = []string{}
		}
		if sub, ok := submitted[co.ID]; ok {
			ac.Submission = summarize(sub, revealScore(co))
		}
		out = append(out, ac)
	}
	return httpx.OK(c, gin.H{"contests": out})
}

func (s *Server) appContest(c *gin.Context) error {
	st, err := s.me(c)
	if err != nil {
		return err
	}
	co, err := s.store.Contest(c, c.Param("id"))
	if err != nil {
		return storeErr(err, "Exam")
	}
	if err = s.checkLearningGroup(c, st, "contest", co.ID); err != nil {
		return err
	}
	access, err := s.accessFor(c, st, "contest", []string{co.ID})
	if err != nil {
		return err
	}
	if co.IsNational() && co.Stream != "" {
		// Same stream rule as the list, for links opened directly.
		academic, err := s.academicProfile(c, st.ID)
		if err != nil {
			return err
		}
		if mine, _ := academic["stream"].(string); mine != "" && mine != co.Stream {
			return httpx.Forbidden("This exam set is for the " + co.Stream + " science stream.")
		}
	}
	registered, submitted, err := s.store.StudentContestState(c, st.ID)
	if err != nil {
		return err
	}
	ac := appContest{Contest: co, resourceAccess: access[co.ID], QuestionCount: len(co.QuestionIDs), Participants: []string{}, IsRegistered: registered[co.ID]}
	if err = s.pool.QueryRow(c, "SELECT EXISTS(SELECT 1 FROM exam_mode_settings WHERE contest_id=$1 AND enabled), coalesce((SELECT max_violations FROM exam_mode_settings WHERE contest_id=$1 AND enabled), 0)", co.ID).Scan(&ac.ExamMode, &ac.MaxViolations); err != nil {
		return err
	}
	if err = s.pool.QueryRow(c, `SELECT count(*) FROM exam_violations WHERE contest_id = $1 AND student_id = $2 AND kind <> 'auto_submitted'`, co.ID, st.ID).Scan(&ac.Violations); err != nil {
		return err
	}
	if sub, ok := submitted[co.ID]; ok {
		ac.Submission = summarize(sub, revealScore(co))
	}
	if co.IsNational() {
		if _, ends, err := s.practiceAttempt(c, co, st.ID, false); err == nil {
			ac.AttemptEndsAt = &ends
		} else if !errors.Is(err, store.ErrNotFound) {
			return err
		}
	}
	return httpx.OK(c, gin.H{"contest": ac})
}

func (s *Server) appRegister(c *gin.Context) error {
	var in struct {
		Subjects   []string `json:"subjects" binding:"max=10"`
		Experience string   `json:"experience" binding:"omitempty,oneof=beginner intermediate advanced"`
	}
	if err := httpx.Bind(c, &in); err != nil {
		return err
	}
	st, err := s.me(c)
	if err != nil {
		return err
	}
	co, err := s.store.Contest(c, c.Param("id"))
	if err != nil {
		return storeErr(err, "Exam")
	}
	if err = s.checkLearningAccess(c, st, "contest", co.ID); err != nil {
		return err
	}
	if co.Phase == "ended" {
		return httpx.Conflict("contest_ended", "Registration for this exam has closed.")
	}
	if err := s.store.Register(c, co.ID, st.ID, trimAll(in.Subjects), in.Experience); err != nil {
		return err
	}
	return httpx.NoContent(c)
}

// contestForPlay checks the student may take the contest right now.
func (s *Server) contestForPlay(c *gin.Context) (store.Student, store.Contest, error) {
	st, err := s.me(c)
	if err != nil {
		return st, store.Contest{}, err
	}
	co, err := s.store.Contest(c, c.Param("id"))
	if err != nil {
		return st, co, storeErr(err, "Exam")
	}
	if err = s.checkLearningAccess(c, st, "contest", co.ID); err != nil {
		return st, co, err
	}
	registered, submitted, err := s.store.StudentContestState(c, st.ID)
	if err != nil {
		return st, co, err
	}
	switch {
	case !registered[co.ID] && !co.IsNational():
		return st, co, httpx.Forbidden("Register for this exam first.")
	case co.Phase == "upcoming":
		return st, co, httpx.Conflict("not_started", "This exam hasn't started yet.")
	case len(co.QuestionIDs) == 0:
		return st, co, httpx.Conflict("not_ready", "This exam isn't ready yet.")
	case submitted[co.ID].StudentID != "":
		return st, co, httpx.Conflict("already_submitted", "You have already submitted this exam.")
	}
	return st, co, nil
}

func (s *Server) appContestQuestions(c *gin.Context) error {
	st, co, err := s.contestForPlay(c)
	if err != nil {
		return err
	}
	if co.Phase == "ended" {
		return httpx.Conflict("contest_ended", "This exam has ended.")
	}
	if err = s.requireExamEntry(c, st.ID, co.ID); err != nil {
		return err
	}
	endsAt := co.EndTime
	if co.IsNational() {
		if _, endsAt, err = s.practiceAttempt(c, co, st.ID, true); err != nil {
			return err
		}
		if !s.now().Before(endsAt) {
			return httpx.Conflict("time_up", "Your time for this exam has run out.")
		}
	}
	qs, err := s.store.ContestQuestions(c, co.ID)
	if err != nil {
		return err
	}
	return httpx.OK(c, gin.H{"questions": playQuestions(co, st.ID, qs), "ends_at": endsAt})
}

// submitGrace allows for network delay when the timer auto-submits at the end.
const submitGrace = 60 * time.Second

func (s *Server) appSubmit(c *gin.Context) error {
	var in struct {
		Answers     map[string]int `json:"answers"`
		TimeSeconds int            `json:"time_seconds" binding:"min=0"`
	}
	if err := httpx.Bind(c, &in); err != nil {
		return err
	}
	st, co, err := s.contestForPlay(c)
	if err != nil {
		return err
	}
	deadline, maxSeconds := co.EndTime, int(co.EndTime.Sub(co.StartTime).Seconds())
	if co.IsNational() {
		started, ends, err := s.practiceAttempt(c, co, st.ID, false)
		if errors.Is(err, store.ErrNotFound) {
			return httpx.Conflict("not_started", "Start the exam before submitting.")
		}
		if err != nil {
			return err
		}
		deadline, maxSeconds = ends, int(ends.Sub(started).Seconds())
		if elapsed := int(s.now().Sub(started).Seconds()); in.TimeSeconds > elapsed {
			in.TimeSeconds = elapsed
		}
	}
	if s.now().After(deadline.Add(submitGrace)) {
		return httpx.Conflict("contest_ended", "The time is over, so answers can no longer be submitted.")
	}
	if err = s.requireExamEntry(c, st.ID, co.ID); err != nil {
		return err
	}
	qs, err := s.store.ContestQuestions(c, co.ID)
	if err != nil {
		return err
	}
	// Score on the server; ignore answers for unknown questions or options.
	answers, correct := scoreAnswers(qs, unshuffleAnswers(co, st.ID, qs, in.Answers))
	if in.TimeSeconds > maxSeconds {
		in.TimeSeconds = maxSeconds
	}
	sub := store.Submission{ContestID: co.ID, StudentID: st.ID, Answers: answers, Correct: correct, Total: len(qs), TimeSeconds: in.TimeSeconds, SubmittedAt: s.now()}
	if err := s.store.CreateSubmission(c, sub); err != nil {
		if errors.Is(err, store.ErrDuplicate) {
			return httpx.Conflict("already_submitted", "You have already submitted this exam.")
		}
		return err
	}
	// Contest scores stay hidden until the contest ends.
	return httpx.Created(c, gin.H{"submission": summarize(sub, co.IsNational())})
}

func (s *Server) appEditorial(c *gin.Context) error {
	st, err := s.me(c)
	if err != nil {
		return err
	}
	co, err := s.store.Contest(c, c.Param("id"))
	if err != nil {
		return storeErr(err, "Exam")
	}
	if err = s.checkLearningAccess(c, st, "contest", co.ID); err != nil {
		return err
	}
	answers := map[string]int{}
	submitted := false
	if sub, err := s.store.Submission(c, co.ID, st.ID); err == nil {
		answers, submitted = sub.Answers, true
	} else if !errors.Is(err, store.ErrNotFound) {
		return err
	}
	if co.Phase != "ended" && !(co.IsNational() && submitted) {
		if co.IsNational() {
			return httpx.Conflict("not_submitted", "Answers are shown after you submit the exam.")
		}
		return httpx.Conflict("not_ended", "The editorial is published when the exam ends.")
	}
	qs, err := s.store.ContestQuestions(c, co.ID)
	if err != nil {
		return err
	}
	return httpx.OK(c, gin.H{"questions": qs, "answers": answers})
}

func (s *Server) appStandings(c *gin.Context) error {
	st, err := s.me(c)
	if err != nil {
		return err
	}
	if err = s.checkLearningAccess(c, st, "contest", c.Param("id")); err != nil {
		return err
	}
	co, err := s.store.Contest(c, c.Param("id"))
	if err != nil {
		return err
	}
	if co.Phase != "ended" {
		return httpx.Conflict("not_ended", "Results are published when the exam ends.")
	}
	rows, err := s.store.PrivateStandings(c, c.Param("id"), st.ID)
	if err != nil {
		return err
	}
	return httpx.OK(c, gin.H{"standings": rows})
}

func (s *Server) appLeaderboard(c *gin.Context) error {
	st, err := s.me(c)
	if err != nil {
		return err
	}
	academic, err := s.academicProfile(c, st.ID)
	if err != nil {
		return err
	}
	level, hasLevel := c.GetQuery("level")
	if !hasLevel {
		level, _ = academic["department"].(string)
		if level == "" {
			level = st.Grade
		}
	}
	if level == "12" {
		level = "entrance"
	}
	switch level {
	case "9", "10", "11", "entrance", "remedial", "freshman":
	default:
		return httpx.Field("level", "Choose grade 9, 10, 11, entrance, remedial or freshman.")
	}
	stream, hasStream := c.GetQuery("stream")
	if !hasStream {
		stream, _ = academic["stream"].(string)
	}
	if level == "9" || level == "10" {
		stream = ""
	}
	if stream != "" && stream != "natural" && stream != "social" {
		return httpx.Field("stream", "Choose natural or social.")
	}
	// Paid and free students have separate boards: the student's own plan's.
	var paid bool
	if err := s.pool.QueryRow(c, `SELECT `+store.PaidNow("$1"), st.ID).Scan(&paid); err != nil {
		return err
	}
	rows, err := s.store.PrivateLeaderboard(c, st.ID, level, stream, paid)
	if err != nil {
		return err
	}
	var mine *store.StandingRow
	top := make([]store.StandingRow, 0, len(rows))
	for i := range rows {
		if rows[i].StudentID == st.ID {
			copy := rows[i]
			mine = &copy
		}
		if rows[i].Rank <= 50 {
			top = append(top, rows[i])
		}
	}
	rows = top
	plan := "free"
	if paid {
		plan = "paid"
	}
	return httpx.OK(c, gin.H{"leaderboard": rows, "me": mine, "level": level, "stream": stream, "plan": plan})
}
