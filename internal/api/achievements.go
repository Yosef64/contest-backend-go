package api

import (
	"time"

	"github.com/gin-gonic/gin"

	"victor-contest-go/internal/httpx"
)

// Daily streaks and achievement badges (guide: gamification). Days follow
// the Addis Ababa calendar. A day is active when the student opened the app
// or studied: completed a chapter, submitted an exam, logged a habit or
// finished a study session.

const addisDay = `(now() AT TIME ZONE 'Africa/Addis_Ababa')::date`

type badge struct {
	ID          string `json:"id"`
	Title       string `json:"title"`
	Description string `json:"description"`
	Icon        string `json:"icon"`
	Progress    int    `json:"progress"`
	Target      int    `json:"target"`
	Earned      bool   `json:"earned"`
}

// recordActivity marks today as active for the student.
func (s *Server) recordActivity(c *gin.Context, studentID string) error {
	_, err := s.pool.Exec(c, `INSERT INTO student_activity (student_id, day) VALUES ($1, `+addisDay+`) ON CONFLICT DO NOTHING`, studentID)
	return err
}

// POST /me/checkin — the app opened; returns the achievements.
func (s *Server) appCheckin(c *gin.Context) error {
	st, err := s.me(c)
	if err != nil {
		return err
	}
	if err = s.recordActivity(c, st.ID); err != nil {
		return err
	}
	return s.writeAchievements(c, st.ID)
}

// GET /me/achievements
func (s *Server) appAchievements(c *gin.Context) error {
	st, err := s.me(c)
	if err != nil {
		return err
	}
	return s.writeAchievements(c, st.ID)
}

func (s *Server) writeAchievements(c *gin.Context, studentID string) error {
	a, err := s.achievements(c, studentID)
	if err != nil {
		return err
	}
	return httpx.OK(c, a)
}

// achievements: the student's streak, recent active days and badges.
func (s *Server) achievements(c *gin.Context, studentID string) (gin.H, error) {
	rows, err := s.pool.Query(c, `WITH days AS (
			SELECT day FROM student_activity WHERE student_id = $1
			UNION SELECT (completed_at AT TIME ZONE 'Africa/Addis_Ababa')::date FROM chapter_progress WHERE student_id = $1
			UNION SELECT (submitted_at AT TIME ZONE 'Africa/Addis_Ababa')::date FROM submissions WHERE student_id = $1
			UNION SELECT day FROM habit_logs WHERE student_id = $1 AND cardinality(checked) > 0
			UNION SELECT (starts_at AT TIME ZONE 'Africa/Addis_Ababa')::date FROM study_sessions WHERE student_id = $1 AND done)
		SELECT day, `+addisDay+` FROM days WHERE day <= `+addisDay+` ORDER BY day DESC LIMIT 400`, studentID)
	if err != nil {
		return nil, err
	}
	var days []time.Time
	var today time.Time
	for rows.Next() {
		var d time.Time
		if err := rows.Scan(&d, &today); err != nil {
			rows.Close()
			return nil, err
		}
		days = append(days, d)
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return nil, err
	}
	current, best := streaks(days, today)

	var chapters, contests, nationals, sharp, sessions, habitDays int
	err = s.pool.QueryRow(c, `SELECT
			(SELECT count(*) FROM chapter_progress WHERE student_id = $1)::int,
			(SELECT count(*) FROM submissions s JOIN contests co ON co.id = s.contest_id WHERE s.student_id = $1 AND co.kind = 'contest')::int,
			(SELECT count(*) FROM submissions s JOIN contests co ON co.id = s.contest_id WHERE s.student_id = $1 AND co.kind <> 'contest')::int,
			(SELECT count(*) FROM submissions s JOIN contests co ON co.id = s.contest_id WHERE s.student_id = $1
				AND s.total > 0 AND s.correct * 10 >= s.total * 9 AND (co.kind <> 'contest' OR co.end_time < now()))::int,
			(SELECT count(*) FROM study_sessions WHERE student_id = $1 AND done)::int,
			(SELECT count(*) FROM habit_logs WHERE student_id = $1 AND cardinality(checked) > 0)::int`, studentID).
		Scan(&chapters, &contests, &nationals, &sharp, &sessions, &habitDays)
	if err != nil {
		return nil, err
	}
	b := func(id, title, desc, icon string, progress, target int) badge {
		return badge{id, title, desc, icon, min(progress, target), target, progress >= target}
	}
	badges := []badge{
		b("streak_3", "On a roll", "Be active 3 days in a row.", "flame", best, 3),
		b("streak_7", "Week warrior", "Be active 7 days in a row.", "flame", best, 7),
		b("streak_30", "Unstoppable", "Be active 30 days in a row.", "flame", best, 30),
		b("first_chapter", "First steps", "Complete your first chapter.", "book", chapters, 1),
		b("bookworm", "Bookworm", "Complete 10 chapters.", "book", chapters, 10),
		b("scholar", "Scholar", "Complete 25 chapters.", "book", chapters, 25),
		b("contender", "Contender", "Take part in a contest.", "trophy", contests, 1),
		b("past_papers", "Past paper pro", "Finish 3 past national exams.", "exam", nationals, 3),
		b("sharp", "Sharp shooter", "Score 90% or more in an exam.", "target", sharp, 1),
		b("planner", "Planner", "Finish 5 planned study sessions.", "calendar", sessions, 5),
		b("habits", "Habit builder", "Log your habits on 7 days.", "check", habitDays, 7),
	}

	recent := []string{}
	for _, d := range days {
		if today.Sub(d) < 14*24*time.Hour {
			recent = append(recent, d.Format("2006-01-02"))
		}
	}
	activeToday := len(days) > 0 && days[0].Equal(today)
	return gin.H{
		"streak":       current,
		"best_streak":  best,
		"active_today": activeToday,
		"recent_days":  recent,
		"today":        today.Format("2006-01-02"),
		"badges":       badges,
	}, nil
}

// streaks: days are distinct, newest first. The current streak runs back
// from today, or from yesterday while today has no activity yet.
func streaks(days []time.Time, today time.Time) (current, best int) {
	run := 0
	var prev time.Time
	for i, d := range days {
		if i > 0 && prev.Sub(d) == 24*time.Hour {
			run++
		} else {
			run = 1
		}
		best = max(best, run)
		prev = d
	}
	if len(days) == 0 || today.Sub(days[0]) > 24*time.Hour {
		return 0, best
	}
	current = 1
	for i := 1; i < len(days) && days[i-1].Sub(days[i]) == 24*time.Hour; i++ {
		current++
	}
	return current, best
}
