package api

import (
	"fmt"
	"html/template"
	"net/http"
	"net/mail"
	"net/url"
	"regexp"
	"strings"

	"github.com/gin-gonic/gin"

	"victor-contest-go/internal/httpx"
	"victor-contest-go/internal/store"
)

// Site settings: About text and official channels -------------------------------

// siteLinks lists the official channels in display order.
var siteLinks = []struct{ Key, Label string }{
	{"telegram_channel", "Telegram channel"},
	{"telegram_bot", "Telegram bot"},
	{"youtube", "YouTube"},
	{"tiktok", "TikTok"},
	{"facebook", "Facebook"},
	{"instagram", "Instagram"},
	{"website", "Website"},
	{"support_email", "Support email"},
	{"support_phone", "Support phone"},
}

const maxAbout = 4000

var supportPhone = regexp.MustCompile(`^\+?[0-9 ]{7,16}$`)

func (s *Server) siteSettings(c *gin.Context) (map[string]string, error) {
	rows, err := s.pool.Query(c, `SELECT key, value FROM site_settings`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := map[string]string{}
	for rows.Next() {
		var k, v string
		if err := rows.Scan(&k, &v); err != nil {
			return nil, err
		}
		out[k] = v
	}
	return out, rows.Err()
}

// publicSite is shown on the app's About page (no sign-in needed).
func (s *Server) publicSite(c *gin.Context) error {
	settings, err := s.siteSettings(c)
	if err != nil {
		return err
	}
	links := []gin.H{}
	for _, l := range siteLinks {
		v := settings[l.Key]
		if v == "" {
			continue
		}
		href := v
		switch l.Key {
		case "support_email":
			href = "mailto:" + v
		case "support_phone":
			href = "tel:" + strings.ReplaceAll(v, " ", "")
		}
		links = append(links, gin.H{"key": l.Key, "label": l.Label, "value": v, "url": href})
	}
	return httpx.OK(c, gin.H{"about": settings["about"], "links": links})
}

func (s *Server) adminSiteSettings(c *gin.Context) error {
	settings, err := s.siteSettings(c)
	if err != nil {
		return err
	}
	out := gin.H{"about": settings["about"]}
	for _, l := range siteLinks {
		out[l.Key] = settings[l.Key]
	}
	return httpx.OK(c, gin.H{"settings": out})
}

func (s *Server) adminSaveSiteSettings(c *gin.Context) error {
	var in map[string]string
	if err := c.ShouldBindJSON(&in); err != nil {
		return httpx.BadRequest("Send the settings as a JSON object of strings.")
	}
	allowed := map[string]bool{"about": true}
	for _, l := range siteLinks {
		allowed[l.Key] = true
	}
	fields := map[string]string{}
	clean := map[string]string{}
	for k, v := range in {
		if !allowed[k] {
			fields[k] = "Unknown setting."
			continue
		}
		v = strings.TrimSpace(v)
		switch {
		case v == "":
		case k == "about":
			if len(v) > maxAbout {
				fields[k] = fmt.Sprintf("Keep this under %d characters.", maxAbout)
			}
		case k == "support_email":
			if a, err := mail.ParseAddress(v); err != nil || a.Address != v {
				fields[k] = "Enter an email address."
			}
		case k == "support_phone":
			if !supportPhone.MatchString(v) {
				fields[k] = "Enter a phone number."
			}
		default:
			if u, err := url.Parse(v); err != nil || u.Scheme != "https" || u.Host == "" {
				fields[k] = "Enter a full https:// link."
			}
		}
		clean[k] = v
	}
	if len(fields) > 0 {
		return httpx.Invalid(fields)
	}
	tx, err := s.pool.Begin(c)
	if err != nil {
		return err
	}
	defer tx.Rollback(c) //nolint:errcheck
	for k, v := range clean {
		if _, err := tx.Exec(c, `INSERT INTO site_settings (key, value) VALUES ($1, $2)
			ON CONFLICT (key) DO UPDATE SET value = $2, updated_at = now()`, k, v); err != nil {
			return err
		}
	}
	if _, err := tx.Exec(c, `INSERT INTO audit_events (admin_id, action, resource) VALUES ($1, 'site_settings', 'site')`, currentAdmin(c).ID); err != nil {
		return err
	}
	if err := tx.Commit(c); err != nil {
		return err
	}
	return s.adminSiteSettings(c)
}

// Referral locks ------------------------------------------------------------------

const referralsNeeded = 2

// chapterLock tells the app why a chapter in a course is locked for this
// student and, for referral locks, how far the student has got.
type chapterLock struct {
	Reason   string `json:"reason"` // "referral" or "enrollment"
	Visits   int    `json:"visits"`
	Needed   int    `json:"needed"`
	ShareURL string `json:"share_url,omitempty"`
}

// chapterLocks mirrors checkLearningAccess for every published chapter of
// the given courses in one query, so course pages can show locks up front.
func (s *Server) chapterLocks(c *gin.Context, st store.Student, courseIDs ...string) (map[string]chapterLock, error) {
	// A chapter's own policy wins; without one the chapter follows its
	// course, which is premium unless the admin made it free.
	rows, err := s.pool.Query(c, `SELECT ch.id, p.access IS NOT NULL, coalesce(p.access, 'free'), coalesce(p.referral_enabled, false),
			(SELECT count(*) FROM referral_visits r WHERE r.owner_id = $2 AND r.chapter_id = ch.id)::int,
			coalesce(p.access <> 'free' AND EXISTS (SELECT 1 FROM enrollment_requests e WHERE e.student_id = $2
				AND e.module = p.access AND e.status = 'approved' AND e.expires_at > now()), false),
			coalesce(cp.access, '`+premiumModule+`') <> 'free' AND NOT EXISTS (SELECT 1 FROM enrollment_requests e WHERE e.student_id = $2
				AND e.module = coalesce(cp.access, '`+premiumModule+`') AND e.status = 'approved' AND e.expires_at > now())
		FROM chapters ch
		LEFT JOIN learning_policies p ON p.resource_type = 'chapter' AND p.resource_id = ch.id
		LEFT JOIN learning_policies cp ON cp.resource_type = 'course' AND cp.resource_id = ch.course_id
		WHERE ch.course_id = ANY($1) AND ch.status = 'published'`, courseIDs, st.ID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	locks := map[string]chapterLock{}
	for rows.Next() {
		var id, access string
		var own, referral, paid, courseLocked bool
		var visits int
		if err := rows.Scan(&id, &own, &access, &referral, &visits, &paid, &courseLocked); err != nil {
			return nil, err
		}
		share := fmt.Sprintf("%s/r/%s/%s", s.cfg.PublicBaseURL, url.PathEscape(st.ID), url.PathEscape(id))
		switch {
		case !own:
			if courseLocked {
				locks[id] = chapterLock{Reason: "enrollment"}
			}
		case access != "free" && paid:
		case access != "free" && !referral:
			locks[id] = chapterLock{Reason: "enrollment"}
		case referral && visits < referralsNeeded:
			locks[id] = chapterLock{Reason: "referral", Visits: visits, Needed: referralsNeeded, ShareURL: share}
		}
	}
	return locks, rows.Err()
}

var referralPage = template.Must(template.New("r").Parse(`<!doctype html>
<html lang="en"><head><meta charset="utf-8"><meta name="viewport" content="width=device-width,initial-scale=1">
<title>Join {{.Inviter}} on Victory</title>
<style>
:root{--g:#13814f;--ink:#121715;--muted:#71777f;--canvas:#eceeef}
*{box-sizing:border-box}body{margin:0;background:var(--canvas);color:var(--ink);font:16px/1.5 Urbanist,system-ui,-apple-system,Segoe UI,Roboto,sans-serif;display:grid;min-height:100vh;place-items:center;padding:16px}
main{background:#fff;border-radius:28px;padding:28px;max-width:420px;width:100%}
h1{font-size:28px;line-height:1.2;letter-spacing:-.7px;margin:0 0 8px;font-weight:600}
p{color:var(--muted);margin:0 0 20px}
.btn{display:block;text-align:center;text-decoration:none;border-radius:999px;padding:14px 18px;font-weight:600;margin-top:10px}
.primary{background:var(--g);color:#fff}.ghost{border:1px solid #e6e8ea;color:var(--ink)}
small{display:block;color:var(--muted);margin-top:18px;font-size:13px}
</style></head><body><main>
<h1>{{.Inviter}} invited you to Victory</h1>
<p>{{if .Chapter}}Open the app to help unlock <strong>{{.Chapter}}</strong> and start learning with courses, live contests and leaderboards.{{else}}Courses, live contests and leaderboards for Grade 12 entrance, remedial and freshman students.{{end}}</p>
{{if .AppLink}}<a class="btn primary" href="{{.AppLink}}">Open in the Victory app</a>{{end}}
{{range .Links}}<a class="btn ghost" href="{{.URL}}" rel="noopener">{{.Label}}</a>{{end}}
<small>Sign in or create a free account in the app; your visit counts once you are signed in.</small>
</main></body></html>`))

// referralLanding is the public page behind a shared referral link. It
// sends the visitor into the app, where the visit is recorded once they are
// signed in, and points them to the official Telegram channels.
func (s *Server) referralLanding(c *gin.Context) {
	ownerID, chapterID := c.Param("owner"), c.Param("chapter")
	data := struct {
		Inviter, Chapter string
		AppLink          template.URL
		Links            []struct{ Label, URL string }
	}{Inviter: "A friend"}

	var name, title string
	var enabled bool
	err := s.pool.QueryRow(c, `SELECT st.name, ch.title, EXISTS (SELECT 1 FROM learning_policies p
			WHERE p.resource_type = 'chapter' AND p.resource_id = ch.id AND p.referral_enabled)
		FROM students st, chapters ch WHERE st.id = $1 AND ch.id = $2 AND ch.status = 'published'`,
		ownerID, chapterID).Scan(&name, &title, &enabled)
	if err == nil {
		data.Inviter = strings.Fields(name + " ")[0]
		if enabled {
			data.Chapter = title
			q := url.Values{"owner": {ownerID}, "chapter": {chapterID}}
			data.AppLink = template.URL("victory://referral?" + q.Encode())
		}
	}
	if data.AppLink == "" {
		data.AppLink = "victory://open"
	}
	if settings, err := s.siteSettings(c); err == nil {
		for _, key := range []string{"telegram_channel", "telegram_bot"} {
			if v := settings[key]; strings.HasPrefix(v, "https://") {
				label := map[string]string{"telegram_channel": "Join our Telegram channel", "telegram_bot": "Open the Victory Telegram bot"}[key]
				data.Links = append(data.Links, struct{ Label, URL string }{label, v})
			}
		}
	}
	c.Header("Content-Type", "text/html; charset=utf-8")
	c.Header("Cache-Control", "no-store")
	c.Header("Referrer-Policy", "no-referrer")
	c.Status(http.StatusOK)
	_ = referralPage.Execute(c.Writer, data)
}

// appAnnouncements returns the active notices for this student's department
// and school, newest first (the app shows them as a dismissible banner).
func (s *Server) appAnnouncements(c *gin.Context) error {
	st, err := s.me(c)
	if err != nil {
		return err
	}
	rows, err := s.store.ServiceRows(c, `SELECT jsonb_build_object('id', a.id, 'title', a.title, 'body', a.body, 'created_at', a.created_at)
		FROM announcements a
		WHERE a.active
		  AND (a.department = '' OR a.department = (SELECT department FROM academic_profiles WHERE student_id = $1))
		  AND (a.school_id IS NULL OR a.school_id = (SELECT school_id FROM academic_profiles WHERE student_id = $1))
		ORDER BY a.created_at DESC LIMIT 5`, st.ID)
	if err != nil {
		return err
	}
	return httpx.OK(c, gin.H{"announcements": rows})
}
