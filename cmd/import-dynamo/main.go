// Command import-dynamo copies the old mini-app data from DynamoDB into the
// v1 Postgres schema. It only reads DynamoDB (Scan) and never writes to it.
//
//	go run ./cmd/import-dynamo                 # dry run: report what would be imported
//	go run ./cmd/import-dynamo -apply          # write to DATABASE_URL in one transaction
//	go run ./cmd/import-dynamo -endpoint http://localhost:8000   # DynamoDB Local / export copy
//
// Rows are inserted with ON CONFLICT DO NOTHING, so re-running is safe and
// never overwrites data created in v1. Transformations:
//   - grades "1".."4" become "9".."12"
//   - question answers become 0-based
//   - students become Telegram identities (id = Telegram ID, as v1 sign-in expects)
//   - submissions get an answers map rebuilt from score + missed_questions
//   - admin plaintext passwords are bcrypt-hashed (ask admins to change them)
//
// Articles, comments, feedback, payments, notifications, achievements and
// page views are retired features and are not imported.
package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"log/slog"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	awsconfig "github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/feature/dynamodb/attributevalue"
	"github.com/aws/aws-sdk-go-v2/service/dynamodb"
	"github.com/aws/aws-sdk-go-v2/service/dynamodb/types"
	"github.com/jackc/pgx/v5"
	"github.com/joho/godotenv"

	"victor-contest-go/internal/api"
	"victor-contest-go/internal/auth"
	"victor-contest-go/internal/db"
)

type oldAdmin struct {
	ID         string `dynamodbav:"id"`
	Email      string `dynamodbav:"email"`
	IsApproved bool   `dynamodbav:"is_approved"`
	Name       string `dynamodbav:"name"`
	Password   string `dynamodbav:"password"`
}

type oldStudent struct {
	ID          string    `dynamodbav:"id"`
	TelegramID  string    `dynamodbav:"telegram_id"`
	Name        string    `dynamodbav:"name"`
	Age         string    `dynamodbav:"age"`
	Grade       string    `dynamodbav:"grade"`
	School      string    `dynamodbav:"school"`
	City        string    `dynamodbav:"city"`
	Region      string    `dynamodbav:"region"`
	ImgURL      string    `dynamodbav:"imgurl"`
	IsSuspended bool      `dynamodbav:"isSuspended"`
	PhoneNumber string    `dynamodbav:"phoneNumber"`
	Gender      string    `dynamodbav:"gender"`
	CreatedAt   time.Time `dynamodbav:"created_at"`
}

type oldQuestion struct {
	ID             string   `dynamodbav:"id"`
	QuestionText   string   `dynamodbav:"question_text"`
	Answer         int      `dynamodbav:"answer"` // 1-based
	QuestionImg    string   `dynamodbav:"question_img"`
	Explanation    string   `dynamodbav:"explanation"`
	Subject        string   `dynamodbav:"subject"`
	Grade          string   `dynamodbav:"grade"`
	Chapter        string   `dynamodbav:"chapter"`
	MultipleChoice []string `dynamodbav:"multiple_choice"`
}

type oldContest struct {
	ID          string   `dynamodbav:"id"`
	Title       string   `dynamodbav:"title"`
	Description string   `dynamodbav:"description"`
	StartTime   string   `dynamodbav:"start_time"`
	EndTime     string   `dynamodbav:"end_time"`
	Subject     string   `dynamodbav:"subject"`
	Grade       string   `dynamodbav:"grade"`
	Prize       string   `dynamodbav:"prize"`
	Questions   []string `dynamodbav:"questions"`
}

type oldRegistration struct {
	ContestID    string    `dynamodbav:"contest_id"`
	StudentID    string    `dynamodbav:"student_id"`
	RegisteredAt time.Time `dynamodbav:"registered_at"`
}

type oldSubmission struct {
	ContestID       string    `dynamodbav:"contest_id"`
	StudentID       string    `dynamodbav:"student_id"`
	Score           float64   `dynamodbav:"score"`
	SubmissionTime  time.Time `dynamodbav:"submission_time"`
	TimeSpend       string    `dynamodbav:"time_spend"`
	MissedQuestions []struct {
		ID             string `dynamodbav:"id"`
		SelectedAnswer int    `dynamodbav:"selected_answer"` // 1-based, 0 = unanswered
	} `dynamodbav:"missed_questions"`
}

func main() {
	apply := flag.Bool("apply", false, "write to Postgres (default is a dry run)")
	region := flag.String("region", "eu-north-1", "DynamoDB region")
	endpoint := flag.String("endpoint", "", "DynamoDB endpoint override, e.g. DynamoDB Local")
	flag.Parse()

	_ = godotenv.Load()
	log := slog.New(slog.NewTextHandler(os.Stderr, nil))
	if err := run(context.Background(), log, *apply, *region, *endpoint); err != nil {
		log.Error("import failed", "err", err)
		os.Exit(1)
	}
}

func run(ctx context.Context, log *slog.Logger, apply bool, region, endpoint string) error {
	cfg, err := awsconfig.LoadDefaultConfig(ctx, awsconfig.WithRegion(region))
	if err != nil {
		return fmt.Errorf("aws config: %w", err)
	}
	dyn := dynamodb.NewFromConfig(cfg, func(o *dynamodb.Options) {
		if endpoint != "" {
			o.BaseEndpoint = aws.String(endpoint)
		}
	})

	var (
		admins   []oldAdmin
		students []oldStudent
		qs       []oldQuestion
		contests []oldContest
		regs     []oldRegistration
		subs     []oldSubmission
	)
	for _, t := range []struct {
		name string
		out  any
	}{
		{"admin", &admins}, {"student", &students}, {"question", &qs},
		{"contests", &contests}, {"contest_registeration", &regs}, {"submissions", &subs},
	} {
		if err := scan(ctx, dyn, t.name, t.out); err != nil {
			return fmt.Errorf("scan %s: %w", t.name, err)
		}
	}

	p := plan(log, admins, students, qs, contests, regs, subs)
	p.report(os.Stdout)
	if !apply {
		fmt.Println("\nDry run only. Re-run with -apply to write to DATABASE_URL.")
		return nil
	}

	pool, err := db.Open(ctx, os.Getenv("DATABASE_URL"))
	if err != nil {
		return err
	}
	defer pool.Close()
	if _, err := db.Migrate(ctx, pool); err != nil {
		return err
	}
	tx, err := pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx) //nolint:errcheck
	inserted, err := p.write(ctx, tx)
	if err != nil {
		return err
	}
	if err := tx.Commit(ctx); err != nil {
		return err
	}
	fmt.Println("\nInserted (existing rows skipped):")
	for _, k := range tableOrder {
		fmt.Printf("  %-14s %d\n", k, inserted[k])
	}
	return nil
}

func scan(ctx context.Context, dyn *dynamodb.Client, table string, out any) error {
	var items []map[string]types.AttributeValue
	pager := dynamodb.NewScanPaginator(dyn, &dynamodb.ScanInput{TableName: aws.String(table)})
	for pager.HasMorePages() {
		page, err := pager.NextPage(ctx)
		if err != nil {
			return err
		}
		items = append(items, page.Items...)
	}
	return attributevalue.UnmarshalListOfMaps(items, out)
}

// ---------------------------------------------------------------------------

var tableOrder = []string{"admins", "students", "questions", "contests", "contest_qs", "registrations", "submissions"}

type row []any

type importPlan struct {
	rows    map[string][]row
	skipped map[string][]string
}

func (p *importPlan) add(table string, r row) { p.rows[table] = append(p.rows[table], r) }
func (p *importPlan) skip(table, why string, id any) {
	p.skipped[table] = append(p.skipped[table], fmt.Sprintf("%v: %s", id, why))
}

func plan(log *slog.Logger, admins []oldAdmin, students []oldStudent, qs []oldQuestion,
	contests []oldContest, regs []oldRegistration, subs []oldSubmission,
) *importPlan {
	p := &importPlan{rows: map[string][]row{}, skipped: map[string][]string{}}

	seenEmail := map[string]bool{}
	for _, a := range admins {
		email := strings.ToLower(strings.TrimSpace(a.Email))
		if email == "" || a.Password == "" || seenEmail[email] {
			p.skip("admins", "missing email/password or duplicate email", a.ID)
			continue
		}
		seenEmail[email] = true
		hash, err := auth.HashPassword(a.Password)
		if err != nil {
			p.skip("admins", err.Error(), a.ID)
			continue
		}
		p.add("admins", row{a.ID, or(a.Name, email), email, hash, a.IsApproved})
	}

	studentID := map[string]string{} // old id -> new id
	for _, s := range students {
		id := strings.TrimSpace(or(s.TelegramID, s.ID))
		grade := api.NormalizeGrade(s.Grade)
		if id == "" || grade == "" {
			p.skip("students", fmt.Sprintf("missing id or unknown grade %q", s.Grade), s.ID)
			continue
		}
		if _, dup := studentID[s.ID]; dup {
			p.skip("students", "duplicate id", s.ID)
			continue
		}
		studentID[s.ID] = id
		studentID[id] = id
		age, _ := strconv.Atoi(strings.TrimSpace(s.Age))
		created := s.CreatedAt
		if created.IsZero() {
			created = time.Now()
		}
		p.add("students", row{id, "telegram", id, or(s.Name, "Student"), s.PhoneNumber, strings.ToLower(s.Gender),
			max(age, 0), s.School, grade, s.City, s.Region, s.ImgURL, s.IsSuspended, created})
	}

	answer := map[string]int{} // question id -> 0-based answer
	for _, q := range qs {
		grade := api.NormalizeGrade(q.Grade)
		opts := nonEmpty(q.MultipleChoice)
		idx := q.Answer - 1
		switch {
		case grade == "":
			p.skip("questions", fmt.Sprintf("unknown grade %q", q.Grade), q.ID)
			continue
		case len(opts) < 2 || len(opts) > 6 || len(opts) != len(q.MultipleChoice):
			p.skip("questions", fmt.Sprintf("needs 2-6 non-empty options, has %d", len(q.MultipleChoice)), q.ID)
			continue
		case idx < 0 || idx >= len(opts):
			p.skip("questions", fmt.Sprintf("answer %d out of range", q.Answer), q.ID)
			continue
		case strings.TrimSpace(q.QuestionText) == "":
			p.skip("questions", "empty text", q.ID)
			continue
		}
		answer[q.ID] = idx
		p.add("questions", row{q.ID, q.QuestionText, opts, idx, q.Explanation, or(q.Subject, "General"),
			or(q.Chapter, "General"), grade, q.QuestionImg})
	}

	contestQs := map[string][]string{}
	imported := map[string]bool{}
	for _, c := range contests {
		grade := api.NormalizeGrade(c.Grade)
		start, err1 := parseTime(c.StartTime)
		end, err2 := parseTime(c.EndTime)
		if grade == "" || err1 != nil || err2 != nil || !end.After(start) {
			p.skip("contests", "bad grade or start/end time", c.ID)
			continue
		}
		imported[c.ID] = true
		p.add("contests", row{c.ID, or(c.Title, "Contest"), c.Description, or(c.Subject, "General"), grade, start, end, c.Prize})
		pos := 0
		for _, qid := range c.Questions {
			if _, ok := answer[qid]; !ok {
				p.skip("contest_qs", "question not imported", c.ID+"/"+qid)
				continue
			}
			pos++
			p.add("contest_qs", row{c.ID, qid, pos})
			contestQs[c.ID] = append(contestQs[c.ID], qid)
		}
	}

	for _, r := range regs {
		sid, ok := studentID[r.StudentID]
		if !ok || !imported[r.ContestID] {
			p.skip("registrations", "student or contest not imported", r.ContestID+"/"+r.StudentID)
			continue
		}
		at := r.RegisteredAt
		if at.IsZero() {
			at = time.Now()
		}
		p.add("registrations", row{r.ContestID, sid, at})
	}

	seenSub := map[string]bool{}
	for _, s := range subs {
		sid, ok := studentID[s.StudentID]
		qids := contestQs[s.ContestID]
		key := s.ContestID + "/" + sid
		switch {
		case !ok || len(qids) == 0:
			p.skip("submissions", "student or contest not imported", s.ContestID+"/"+s.StudentID)
			continue
		case seenSub[key]:
			p.skip("submissions", "duplicate (v1 allows one per student)", key)
			continue
		}
		seenSub[key] = true
		// The old table stored only wrong answers; everything else was correct.
		answers := map[string]int{}
		for _, qid := range qids {
			answers[qid] = answer[qid]
		}
		for _, m := range s.MissedQuestions {
			if m.SelectedAnswer > 0 {
				answers[m.ID] = m.SelectedAnswer - 1
			} else {
				delete(answers, m.ID)
			}
		}
		correct := 0
		for qid, a := range answers {
			if a == answer[qid] {
				correct++
			}
		}
		if int(s.Score) != correct {
			log.Warn("score differs from rebuilt answers; keeping rebuilt value", "submission", key, "old", s.Score, "rebuilt", correct)
		}
		raw, _ := json.Marshal(answers)
		at := s.SubmissionTime
		if at.IsZero() {
			at = time.Now()
		}
		p.add("submissions", row{s.ContestID, sid, raw, correct, len(qids), parseDuration(s.TimeSpend), at})
	}
	return p
}

func (p *importPlan) report(w *os.File) {
	fmt.Fprintln(w, "Import plan (DynamoDB → Postgres):")
	for _, t := range tableOrder {
		fmt.Fprintf(w, "  %-14s %5d to import, %4d skipped\n", t, len(p.rows[t]), len(p.skipped[t]))
	}
	for _, t := range tableOrder {
		for i, s := range p.skipped[t] {
			if i == 10 {
				fmt.Fprintf(w, "    … %d more in %s\n", len(p.skipped[t])-10, t)
				break
			}
			fmt.Fprintf(w, "    skip %s %s\n", t, s)
		}
	}
}

var inserts = map[string]string{
	"admins": `INSERT INTO admins (id, name, email, password_hash, is_approved) VALUES ($1,$2,$3,$4,$5) ON CONFLICT DO NOTHING`,
	"students": `INSERT INTO students (id, auth_provider, auth_subject, name, phone_number, gender, age, school, grade, city, region, photo_url, is_suspended, created_at)
		VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13,$14) ON CONFLICT DO NOTHING`,
	"questions": `INSERT INTO questions (id, text, options, answer_index, explanation, subject, chapter, grade, image_url)
		VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9) ON CONFLICT DO NOTHING`,
	"contests":      `INSERT INTO contests (id, title, description, subject, grade, start_time, end_time, prize) VALUES ($1,$2,$3,$4,$5,$6,$7,$8) ON CONFLICT DO NOTHING`,
	"contest_qs":    `INSERT INTO contest_questions (contest_id, question_id, position) VALUES ($1,$2,$3) ON CONFLICT DO NOTHING`,
	"registrations": `INSERT INTO registrations (contest_id, student_id, registered_at) VALUES ($1,$2,$3) ON CONFLICT DO NOTHING`,
	"submissions": `INSERT INTO submissions (contest_id, student_id, answers, correct, total, time_seconds, submitted_at)
		VALUES ($1,$2,$3,$4,$5,$6,$7) ON CONFLICT DO NOTHING`,
}

func (p *importPlan) write(ctx context.Context, tx pgx.Tx) (map[string]int64, error) {
	n := map[string]int64{}
	for _, t := range tableOrder {
		for _, r := range p.rows[t] {
			tag, err := tx.Exec(ctx, inserts[t], r...)
			if err != nil {
				return nil, fmt.Errorf("%s %v: %w", t, r[0], err)
			}
			n[t] += tag.RowsAffected()
		}
	}
	return n, nil
}

// ---------------------------------------------------------------------------

func or(s, fallback string) string {
	if strings.TrimSpace(s) == "" {
		return fallback
	}
	return strings.TrimSpace(s)
}

func nonEmpty(in []string) []string {
	out := make([]string, 0, len(in))
	for _, s := range in {
		if s = strings.TrimSpace(s); s != "" {
			out = append(out, s)
		}
	}
	return out
}

func parseTime(s string) (time.Time, error) {
	for _, layout := range []string{time.RFC3339Nano, "2006-01-02T15:04", "2006-01-02 15:04:05", "2006-01-02T15:04:05"} {
		if t, err := time.Parse(layout, strings.TrimSpace(s)); err == nil {
			return t, nil
		}
	}
	return time.Time{}, fmt.Errorf("unrecognised time %q", s)
}

// parseDuration reads the old "HH:MM:SS" (or plain seconds) time_spend.
func parseDuration(s string) int {
	parts := strings.Split(strings.TrimSpace(s), ":")
	total := 0
	for _, part := range parts {
		v, err := strconv.Atoi(part)
		if err != nil || v < 0 {
			return 0
		}
		total = total*60 + v
	}
	return total
}
