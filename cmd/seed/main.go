// Command seed loads the starter curriculum (courses, chapters) and question
// bank into an empty database. It never overwrites existing content.
//
//	go run ./cmd/seed            # refuses if any course exists
package main

import (
	"context"
	_ "embed"
	"encoding/json"
	"fmt"
	"log/slog"
	"os"

	"github.com/jackc/pgx/v5"
	"github.com/joho/godotenv"

	"victor-contest-go/internal/content"
	"victor-contest-go/internal/db"
)

//go:embed seed.json
var seedJSON []byte

type seedFile struct {
	Courses []struct {
		Grade       string `json:"grade"`
		Subject     string `json:"subject"`
		Title       string `json:"title"`
		Description string `json:"description"`
		Chapters    []struct {
			Number  int    `json:"number"`
			Title   string `json:"title"`
			Summary string `json:"summary"`
			Minutes int    `json:"minutes"`
			HTML    string `json:"html"`
		} `json:"chapters"`
	} `json:"courses"`
	Questions []struct {
		Text        string   `json:"text"`
		Options     []string `json:"options"`
		AnswerIndex int      `json:"answer_index"`
		Explanation string   `json:"explanation"`
		Subject     string   `json:"subject"`
		Chapter     string   `json:"chapter"`
		Grade       string   `json:"grade"`
	} `json:"questions"`
}

func main() {
	_ = godotenv.Load()
	if err := run(context.Background()); err != nil {
		slog.Error("seed failed", "err", err)
		os.Exit(1)
	}
}

func run(ctx context.Context) error {
	var seed seedFile
	if err := json.Unmarshal(seedJSON, &seed); err != nil {
		return err
	}
	pool, err := db.Open(ctx, os.Getenv("DATABASE_URL"))
	if err != nil {
		return err
	}
	defer pool.Close()
	if _, err := db.Migrate(ctx, pool); err != nil {
		return err
	}
	var courses int
	if err := pool.QueryRow(ctx, `SELECT count(*) FROM courses`).Scan(&courses); err != nil {
		return err
	}
	if courses > 0 {
		return fmt.Errorf("database already has %d courses; seed only runs on an empty database", courses)
	}

	var nChapters int
	err = pgx.BeginFunc(ctx, pool, func(tx pgx.Tx) error {
		for i, c := range seed.Courses {
			var id string
			if err := tx.QueryRow(ctx, `INSERT INTO courses (grade, subject, title, description, position) VALUES ($1, $2, $3, $4, $5) RETURNING id`,
				c.Grade, c.Subject, c.Title, c.Description, i+1).Scan(&id); err != nil {
				return err
			}
			for _, ch := range c.Chapters {
				html := content.SanitizeHTML(ch.HTML)
				var chID string
				if err := tx.QueryRow(ctx, `INSERT INTO chapters (course_id, position, title, summary, minutes, html, status, updated_by)
					VALUES ($1, $2, $3, $4, $5, $6, 'published', 'Victory Team') RETURNING id`,
					id, ch.Number, ch.Title, ch.Summary, ch.Minutes, html).Scan(&chID); err != nil {
					return err
				}
				if _, err := tx.Exec(ctx, `INSERT INTO chapter_versions (chapter_id, version, title, html, author) VALUES ($1, 1, $2, $3, 'Victory Team')`,
					chID, ch.Title, html); err != nil {
					return err
				}
				nChapters++
			}
		}
		for _, q := range seed.Questions {
			if _, err := tx.Exec(ctx, `INSERT INTO questions (text, options, answer_index, explanation, subject, chapter, grade) VALUES ($1, $2, $3, $4, $5, $6, $7)`,
				q.Text, q.Options, q.AnswerIndex, q.Explanation, q.Subject, q.Chapter, q.Grade); err != nil {
				return err
			}
		}
		return nil
	})
	if err != nil {
		return err
	}
	slog.Info("seeded", "courses", len(seed.Courses), "chapters", nChapters, "questions", len(seed.Questions))
	return nil
}
