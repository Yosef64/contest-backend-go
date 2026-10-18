package api

import (
	"html/template"
	"log/slog"
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"
	"github.com/jackc/pgx/v5"

	"victor-contest-go/internal/auth"
	"victor-contest-go/internal/httpx"
)

// Account deletion and the privacy policy (Google Play requires both).

// DELETE /me {confirm: true} — deletes the signed-in student's account and
// learning data: profile, progress, answers, results, study plans, group
// exams they created, password sign-in and profile photo. Payments, plans
// and agents' commissions are kept for the academy's accounts, without the
// student.
func (s *Server) deleteMyAccount(c *gin.Context) error {
	var in struct {
		Confirm bool `json:"confirm"`
	}
	if err := httpx.Bind(c, &in); err != nil {
		return err
	}
	if !in.Confirm {
		return httpx.Field("confirm", "Confirm that you want to delete your account.")
	}
	id := auth.Caller(c).Subject
	err := pgx.BeginFunc(c, s.pool, func(tx pgx.Tx) error {
		// Tables without a link to students that removes them.
		for _, q := range []string{
			`DELETE FROM student_credentials WHERE student_id = $1`,
			`DELETE FROM habit_logs WHERE student_id = $1`,
		} {
			if _, err := tx.Exec(c, q, id); err != nil {
				return err
			}
		}
		tag, err := tx.Exec(c, `DELETE FROM students WHERE id = $1`, id)
		if err == nil && tag.RowsAffected() == 0 {
			return httpx.NotFound("Account")
		}
		return err
	})
	if err != nil {
		return err
	}
	if err := s.media.DeleteAvatar(c, id); err != nil {
		slog.Warn("deleted account's photo", "student", id, "err", err)
	}
	slog.Info("student deleted their account", "student", id)
	return httpx.NoContent(c)
}

type pageData struct {
	Email, Phone, Telegram string
}

func (s *Server) pageContacts(c *gin.Context) pageData {
	settings, err := s.siteSettings(c)
	if err != nil {
		return pageData{}
	}
	tg := strings.TrimSpace(settings["telegram_channel"])
	if tg != "" && !strings.HasPrefix(tg, "http") {
		tg = "https://t.me/" + strings.TrimPrefix(tg, "@")
	}
	return pageData{Email: strings.TrimSpace(settings["support_email"]), Phone: strings.TrimSpace(settings["support_phone"]), Telegram: tg}
}

func renderPage(c *gin.Context, page *template.Template, data pageData) {
	c.Header("Content-Type", "text/html; charset=utf-8")
	c.Header("Cache-Control", "public, max-age=300")
	c.Status(http.StatusOK)
	_ = page.Execute(c.Writer, data)
}

// GET /account-deletion — how to delete a Victory account (the link Google
// Play shows).
func (s *Server) accountDeletionPage(c *gin.Context) {
	renderPage(c, deletionPage, s.pageContacts(c))
}

// GET /privacy — the privacy policy.
func (s *Server) privacyPage(c *gin.Context) {
	renderPage(c, privacyPolicy, s.pageContacts(c))
}

const pageStyle = `<meta charset="utf-8"><meta name="viewport" content="width=device-width,initial-scale=1">
<style>
:root{--g:#13814f;--ink:#121715;--muted:#5b626a;--canvas:#eceeef}
*{box-sizing:border-box}body{margin:0;background:var(--canvas);color:var(--ink);font:16px/1.6 Urbanist,system-ui,-apple-system,Segoe UI,Roboto,sans-serif;padding:16px}
main{background:#fff;border-radius:24px;padding:28px;max-width:760px;margin:24px auto}
h1{font-size:30px;line-height:1.2;letter-spacing:-.6px;margin:0 0 6px}h2{font-size:20px;margin:28px 0 6px}
p,li{color:var(--muted)}a{color:var(--g)}small{color:var(--muted)}ol,ul{padding-left:22px}
</style>`

const contactBlock = `{{if or .Email .Phone .Telegram}}<ul>
{{if .Email}}<li>Email: <a href="mailto:{{.Email}}">{{.Email}}</a></li>{{end}}
{{if .Phone}}<li>Phone: {{.Phone}}</li>{{end}}
{{if .Telegram}}<li>Telegram: <a href="{{.Telegram}}">{{.Telegram}}</a></li>{{end}}
</ul>{{else}}<p>Contact Victory Academy through the app's Profile &rsaquo; About page.</p>{{end}}`

var deletionPage = template.Must(template.New("deletion").Parse(`<!doctype html><html lang="en"><head>` + pageStyle + `
<title>Delete your Victory account</title></head><body><main>
<h1>Delete your Victory account</h1>
<p>Victory Academy (the Victory app) lets you delete your account and its data at any time.</p>
<h2>In the app</h2>
<ol><li>Open Victory and sign in.</li><li>Go to <strong>Profile</strong> and tap <strong>Delete account</strong>.</li>
<li>Confirm. Your account is deleted at once and you are signed out.</li></ol>
<h2>Without the app</h2>
<p>Write to us from the email address or phone number on your account and ask us to delete it. We delete it within 7 days and tell you when it is done.</p>
` + contactBlock + `
<h2>What is deleted</h2>
<p>Your profile (name, phone, email, school, grade, city, photo), your sign-in, your lesson progress, exam answers and results, rankings, study plans, AI tutor history and group exams you created.</p>
<h2>What is kept</h2>
<p>Records of payments and plans, which the academy must keep for its accounts, with your name and account removed from them.</p>
</main></body></html>`))

var privacyPolicy = template.Must(template.New("privacy").Parse(`<!doctype html><html lang="en"><head>` + pageStyle + `
<title>Victory privacy policy</title></head><body><main>
<h1>Privacy policy</h1>
<small>Victory Academy &middot; the Victory app &middot; last updated 18 October 2026</small>
<p>This policy explains what the Victory app collects, why, who else handles it, and your choices.</p>
<h2>What we collect</h2>
<ul>
<li><strong>Account:</strong> your name, phone number, email or Google / Telegram sign-in, gender, age, school, grade, department and stream, city and region, and a profile photo if you add one.</li>
<li><strong>Learning:</strong> lessons and chapters you complete, practice and exam answers, scores and rankings, study plans and reminders, questions you ask the AI tutor, and group exams you create or join.</li>
<li><strong>Exam mode:</strong> while an exam runs in exam mode, whether you left the app (breaks), so your proctor or group host can see it.</li>
<li><strong>Payments:</strong> the plan you choose, payment references and any proof of payment you upload. Card and mobile-money details are handled by the payment provider, not stored by us.</li>
<li><strong>Device:</strong> a notification token to send you announcements, and the advertising ID used to show ads.</li>
</ul>
<h2>Why we use it</h2>
<p>To run your account, show your courses and exams, record and rank your results, unlock the plan you paid for, send reminders and announcements, answer AI tutor questions, show ads that keep the app affordable, and give your school (if you joined one) reports on its students.</p>
<h2>Who else handles it</h2>
<ul>
<li>Our hosting and database providers (Amazon Web Services, Neon) store the app's data.</li>
<li>Cloudinary stores pictures and videos, including profile photos.</li>
<li>Google Firebase sends notifications and live updates for group exams (no personal data is in those updates).</li>
<li>Google AdMob shows ads and uses the advertising ID.</li>
<li>Chapa processes online payments.</li>
<li>Anthropic processes the questions you ask the AI tutor to answer them.</li>
<li>Your school's staff see their own students' results, if you joined a school with its code.</li>
</ul>
<p>We do not sell your personal data.</p>
<h2>How long we keep it</h2>
<p>Until you delete your account. Payment and plan records are kept for the academy's accounts, without your name.</p>
<h2>Your choices</h2>
<ul>
<li>Change your profile in the app at any time.</li>
<li>Turn notifications off in your phone's settings.</li>
<li>Reset or limit your advertising ID in your phone's settings.</li>
<li>Delete your account in the app (Profile &rsaquo; Delete account) or as described on the <a href="/account-deletion">account deletion page</a>.</li>
</ul>
<h2>Security</h2>
<p>Data travels encrypted (HTTPS) and passwords are stored only as secure hashes. Lessons are watermarked with your name to discourage copying.</p>
<h2>Students under 18</h2>
<p>Victory is for secondary-school and university students. If you are under 18, use it with your parent's or guardian's knowledge.</p>
<h2>Contact</h2>
` + contactBlock + `
</main></body></html>`))
