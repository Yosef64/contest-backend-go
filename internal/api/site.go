package api

import (
	"fmt"
	"html/template"
	"net/http"
	"net/mail"
	"net/url"
	"regexp"
	"strconv"
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
	return httpx.OK(c, gin.H{"about": settings["about"], "links": links, "ads": adsConfig(settings)})
}

func (s *Server) adminSiteSettings(c *gin.Context) error {
	settings, err := s.siteSettings(c)
	if err != nil {
		return err
	}
	out := gin.H{"about": settings["about"], "price_academic": settings["price_academic"],
		"price_training": settings["price_training"], "price_both": settings["price_both"], "enrollment_days": settings["enrollment_days"],
		"ads_enabled": settings["ads_enabled"], "ads_cards": settings["ads_cards"],
		"ads_free_minutes": settings["ads_free_minutes"], "ads_paid_minutes": settings["ads_paid_minutes"],
		"chapa_enabled": s.cfg.ChapaEnabled()}
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
	allowed := map[string]bool{"about": true, "price_academic": true, "price_training": true, "price_both": true, "enrollment_days": true,
		"ads_enabled": true, "ads_cards": true, "ads_free_minutes": true, "ads_paid_minutes": true}
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
		case k == "price_academic" || k == "price_training" || k == "price_both":
			if n, err := strconv.ParseFloat(v, 64); err != nil || n < 0 || n > 1000000 {
				fields[k] = "Enter a price in birr, e.g. 499."
			}
		case k == "ads_enabled" || k == "ads_cards":
			if v != "on" && v != "off" {
				fields[k] = "Choose on or off."
			}
		case k == "ads_free_minutes" || k == "ads_paid_minutes":
			if n, err := strconv.Atoi(v); err != nil || n < 1 || n > 240 {
				fields[k] = "Enter minutes between 1 and 240."
			}
		case k == "enrollment_days":
			if n, err := strconv.Atoi(v); err != nil || n < 1 || n > 1095 {
				fields[k] = "Enter a number of days between 1 and 1095."
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
	keys := make([]string, 0, len(clean))
	values := make([]string, 0, len(clean))
	for k, v := range clean {
		keys = append(keys, k)
		values = append(values, v)
	}
	// One statement (one round trip): every setting plus the audit entry.
	if _, err := s.pool.Exec(c, `WITH saved AS (
			INSERT INTO site_settings (key, value) SELECT * FROM unnest($1::text[], $2::text[])
			ON CONFLICT (key) DO UPDATE SET value = excluded.value, updated_at = now()
		)
		INSERT INTO audit_events (admin_id, action, resource) VALUES ($3, 'site_settings', 'site')`,
		keys, values, currentAdmin(c).ID); err != nil {
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
	// Each chapter's own access decides; a chapter without a policy is
	// premium (academic enrollment) until an admin makes it free.
	rows, err := s.pool.Query(c, `SELECT ch.id, coalesce(p.access, '`+premiumModule+`'), coalesce(p.referral_enabled, false),
			(SELECT count(*) FROM referral_visits r WHERE r.owner_id = $2 AND r.chapter_id = ch.id)::int,
			coalesce(p.access, '`+premiumModule+`') <> 'free' AND EXISTS (SELECT 1 FROM enrollment_requests e WHERE e.student_id = $2
				AND e.module = coalesce(p.access, '`+premiumModule+`') AND e.status = 'approved' AND e.expires_at > now())
		FROM chapters ch
		LEFT JOIN learning_policies p ON p.resource_type = 'chapter' AND p.resource_id = ch.id
		WHERE ch.course_id = ANY($1) AND ch.status = 'published'`, courseIDs, st.ID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	locks := map[string]chapterLock{}
	for rows.Next() {
		var id, access string
		var referral, paid bool
		var visits int
		if err := rows.Scan(&id, &access, &referral, &visits, &paid); err != nil {
			return nil, err
		}
		share := fmt.Sprintf("%s/r/%s/%s", s.cfg.PublicBaseURL, url.PathEscape(st.ID), url.PathEscape(id))
		switch {
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

// adsConfig is what the app needs to show ads: full-screen ads every N
// minutes of use (fewer for paid plans) and "Sponsored" cards in lists.
// Defaults: on, 3 minutes for free students, 5 for paid ones.
func adsConfig(settings map[string]string) gin.H {
	minutes := func(key string, def int) int {
		if n, err := strconv.Atoi(settings[key]); err == nil && n > 0 {
			return n
		}
		return def
	}
	return gin.H{
		"enabled":      settings["ads_enabled"] != "off",
		"cards":        settings["ads_cards"] != "off",
		"free_minutes": minutes("ads_free_minutes", 3),
		"paid_minutes": minutes("ads_paid_minutes", 5),
	}
}
