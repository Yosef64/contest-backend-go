package store

import "time"

// JSON field names match the admin dashboard (src/api/types.ts) and the
// Flutter app's API repository.

type Admin struct {
	ID             string    `json:"id" db:"id"`
	Name           string    `json:"name" db:"name"`
	Email          string    `json:"email" db:"email"`
	PasswordHash   string    `json:"-" db:"password_hash"`
	IsApproved     bool      `json:"is_approved" db:"is_approved"`
	PermissionRole string    `json:"permission_role" db:"permission_role"`
	SchoolID       *string   `json:"school_id" db:"school_id"`
	IsOwner        bool      `json:"is_owner" db:"is_owner"`
	CreatedAt      time.Time `json:"created_at" db:"created_at"`
}

type Student struct {
	ID           string    `json:"id" db:"id"`
	AuthProvider string    `json:"auth_provider" db:"auth_provider"`
	AuthSubject  string    `json:"-" db:"auth_subject"`
	Name         string    `json:"name" db:"name"`
	PhoneNumber  string    `json:"phone_number" db:"phone_number"`
	Gender       string    `json:"gender" db:"gender"`
	Age          int       `json:"age" db:"age"`
	School       string    `json:"school" db:"school"`
	Grade        string    `json:"grade" db:"grade"`
	City         string    `json:"city" db:"city"`
	Region       string    `json:"region" db:"region"`
	PhotoURL     string    `json:"photo_url" db:"photo_url"`
	PhotoCustom  bool      `json:"-" db:"photo_custom"` // own picture chosen: sign-in picture no longer replaces it
	IsSuspended  bool      `json:"is_suspended" db:"is_suspended"`
	CreatedAt    time.Time `json:"created_at" db:"created_at"`
	UpdatedAt    time.Time `json:"-" db:"updated_at"`
}

type Course struct {
	ID             string    `json:"id" db:"id"`
	Grade          string    `json:"grade" db:"grade"`
	Subject        string    `json:"subject" db:"subject"`
	Title          string    `json:"title" db:"title"`
	Description    string    `json:"description" db:"description"`
	ChapterCount   int       `json:"chapter_count" db:"chapter_count"`
	PublishedCount int       `json:"published_count" db:"published_count"`
	UpdatedAt      time.Time `json:"updated_at" db:"updated_at"`
}

type ChapterSummary struct {
	ID        string    `json:"id" db:"id"`
	CourseID  string    `json:"course_id" db:"course_id"`
	Number    int       `json:"number" db:"position"`
	Title     string    `json:"title" db:"title"`
	Summary   string    `json:"summary" db:"summary"`
	Minutes   int       `json:"minutes" db:"minutes"`
	Status    string    `json:"status" db:"status"`
	Version   int       `json:"version" db:"version"`
	UpdatedAt time.Time `json:"updated_at" db:"updated_at"`
	UpdatedBy string    `json:"updated_by" db:"updated_by"`
}

type ChapterLock struct {
	AdminID   string    `json:"admin_id"`
	AdminName string    `json:"admin_name"`
	ExpiresAt time.Time `json:"expires_at"`
}

type Chapter struct {
	ChapterSummary
	HTML string       `json:"html"`
	Lock *ChapterLock `json:"lock"`
}

type ChapterVersion struct {
	Version   int       `json:"version" db:"version"`
	Title     string    `json:"title" db:"title"`
	CreatedAt time.Time `json:"created_at" db:"created_at"`
	Author    string    `json:"author" db:"author"`
	HTML      string    `json:"html,omitempty" db:"html"`
}

type ChapterComment struct {
	ID        string    `json:"id" db:"id"`
	ChapterID string    `json:"chapter_id" db:"chapter_id"`
	Author    string    `json:"author" db:"author_name"`
	Body      string    `json:"body" db:"body"`
	Quote     string    `json:"quote" db:"quote"`
	Resolved  bool      `json:"resolved" db:"resolved"`
	CreatedAt time.Time `json:"created_at" db:"created_at"`
}

type Question struct {
	ID          string   `json:"id" db:"id"`
	Text        string   `json:"text" db:"text"`
	Options     []string `json:"options" db:"options"`
	AnswerIndex int      `json:"answer_index" db:"answer_index"`
	Explanation string   `json:"explanation" db:"explanation"`
	Subject     string   `json:"subject" db:"subject"`
	Chapter     string   `json:"chapter" db:"chapter"`
	Topic       string   `json:"topic" db:"topic"`
	Grade       string   `json:"grade" db:"grade"`
	// Stream ("natural" or "social") for grade 11 and up; empty suits both.
	Stream   string `json:"stream" db:"stream"`
	ImageURL string `json:"image_url" db:"image_url"`
	// One picture per option ('' for none), or empty when no option has one.
	OptionImages        []string `json:"option_images" db:"option_images"`
	ExplanationImageURL string   `json:"explanation_image_url" db:"explanation_image_url"`
	// A partner school's own question (nil: the main office's).
	SchoolID  *string   `json:"school_id" db:"school_id"`
	CreatedAt time.Time `json:"created_at" db:"created_at"`
	CreatedBy string    `json:"created_by" db:"created_by"`
	UpdatedAt time.Time `json:"updated_at" db:"updated_at"`
	UpdatedBy string    `json:"updated_by" db:"updated_by"`
}

// PublicQuestion is what students see during a contest: no answer.
type PublicQuestion struct {
	ID       string   `json:"id"`
	Text     string   `json:"text"`
	Options  []string `json:"options"`
	Subject  string   `json:"subject"`
	Chapter  string   `json:"chapter"`
	Grade    string   `json:"grade"`
	ImageURL string   `json:"image_url"`
	// Aligned with Options; omitted when no option has a picture.
	OptionImages []string `json:"option_images,omitempty"`
}

func (q Question) Public() PublicQuestion {
	p := PublicQuestion{ID: q.ID, Text: q.Text, Options: q.Options, Subject: q.Subject, Chapter: q.Chapter, Grade: q.Grade, ImageURL: q.ImageURL}
	if len(q.OptionImages) == len(q.Options) {
		p.OptionImages = q.OptionImages
	}
	return p
}

type Contest struct {
	ID            string    `json:"id" db:"id"`
	Title         string    `json:"title" db:"title"`
	Description   string    `json:"description" db:"description"`
	Subject       string    `json:"subject" db:"subject"`
	Grade         string    `json:"grade" db:"grade"`
	StartTime     time.Time `json:"start_time" db:"start_time"`
	EndTime       time.Time `json:"end_time" db:"end_time"`
	Prize         string    `json:"prize" db:"prize"`
	QuestionIDs   []string  `json:"question_ids" db:"question_ids"`
	Registrations int       `json:"registrations" db:"registrations"`
	Submissions   int       `json:"submissions" db:"submissions"`
	// Started timed attempts (national exam sets).
	Attempts int    `json:"attempts" db:"attempts"`
	Phase    string `json:"phase" db:"-"`

	// "contest" (scheduled); or timed practice open between StartTime and
	// EndTime: "national" (past national exam), "model" (model exam) or
	// "challenge" (a challenge day's quiz or weekly exam).
	Kind            string `json:"kind" db:"kind"`
	ExamYear        *int   `json:"exam_year" db:"exam_year"`
	Stream          string `json:"stream" db:"stream"`
	DurationMinutes *int   `json:"duration_minutes" db:"duration_minutes"`
	Rules           string `json:"rules" db:"rules"`
	Scoring         string `json:"scoring" db:"scoring"`
	// Each student sees the questions / options in their own order.
	ShuffleQuestions bool `json:"shuffle_questions" db:"shuffle_questions"`
	ShuffleOptions   bool `json:"shuffle_options" db:"shuffle_options"`
	// A partner school's internal exam, seen only by its students (nil: for
	// everyone the exam's grade and stream fit).
	SchoolID  *string   `json:"school_id" db:"school_id"`
	CreatedBy string    `json:"created_by" db:"created_by"`
	UpdatedAt time.Time `json:"updated_at" db:"updated_at"`
}

// IsNational reports a self-paced exam: a past national set, a model exam, or
// a challenge quiz or weekly exam. Each student gets a timed attempt and sees
// the score at once.
func (c *Contest) IsNational() bool { return c.Kind != "contest" }

// PhaseAt is computed from the schedule, never stored.
func (c *Contest) PhaseAt(now time.Time) string {
	switch {
	case now.Before(c.StartTime):
		return "upcoming"
	case now.Before(c.EndTime):
		return "active"
	default:
		return "ended"
	}
}

type Submission struct {
	ContestID   string         `json:"contest_id" db:"contest_id"`
	StudentID   string         `json:"student_id" db:"student_id"`
	Answers     map[string]int `json:"answers" db:"answers"`
	Correct     int            `json:"correct" db:"correct"`
	Total       int            `json:"total" db:"total"`
	TimeSeconds int            `json:"time_seconds" db:"time_seconds"`
	SubmittedAt time.Time      `json:"submitted_at" db:"submitted_at"`
}

type StandingRow struct {
	Rank        int    `json:"rank" db:"rank"`
	StudentID   string `json:"student_id" db:"student_id"`
	Name        string `json:"name" db:"name"`
	PhotoURL    string `json:"photo_url" db:"photo_url"`
	Correct     int    `json:"correct" db:"correct"`
	Total       int    `json:"total" db:"total"`
	Score       int    `json:"score" db:"score"`
	TimeSeconds int    `json:"time_seconds" db:"time_seconds"`
}

type MediaUpload struct {
	ID           string    `json:"id" db:"id"`
	URL          string    `json:"url" db:"url"`
	Kind         string    `json:"kind" db:"kind"`
	ContentType  string    `json:"content_type" db:"content_type"`
	FileName     string    `json:"file_name" db:"file_name"`
	SizeBytes    int64     `json:"size_bytes" db:"size_bytes"`
	UploaderName string    `json:"uploader_name" db:"uploader_name"`
	CreatedAt    time.Time `json:"created_at" db:"created_at"`
}

type Page[T any] struct {
	Items      []T     `json:"items"`
	NextCursor *string `json:"next_cursor"`
	Total      int     `json:"total"`
}
