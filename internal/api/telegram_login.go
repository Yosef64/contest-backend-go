package api

import (
	"html/template"
	"net/http"
	"net/url"
	"time"

	"github.com/gin-gonic/gin"

	"victor-contest-go/internal/auth"
)

// Telegram sign-in from the app: the app opens telegramLoginPage in the
// browser; Telegram's login widget sends the signed login to
// telegramLoginReturn, which hands it back to the app on a victory:// link.
// The app then posts it to /auth/telegram, which verifies it again.

var telegramPage = template.Must(template.New("tg").Parse(`<!doctype html>
<html lang="en"><head><meta charset="utf-8">
<meta name="viewport" content="width=device-width, initial-scale=1">
<title>Sign in to Victory</title>
<style>
  body{margin:0;min-height:100vh;display:grid;place-items:center;background:#f4f6f8;color:#14212b;
    font:16px/1.5 system-ui,-apple-system,Segoe UI,Roboto,sans-serif}
  main{max-width:360px;padding:32px 24px;text-align:center}
  h1{font-size:22px;margin:0 0 8px} p{margin:0 0 24px;color:#51606b}
  a.btn{display:inline-block;padding:12px 22px;border-radius:999px;background:#229ED9;color:#fff;
    text-decoration:none;font-weight:600}
  .err{color:#b42318}
</style></head><body><main>
{{if .Error}}
  <h1>Sign-in didn't work</h1><p class="err">{{.Error}}</p>
  <a class="btn" href="{{.LoginURL}}">Try again</a>
{{else if .AppURL}}
  <h1>Signed in</h1><p>Returning you to Victory…</p>
  <a class="btn" href="{{.AppURL}}">Open Victory</a>
  <script>location.href = {{.AppURL}};</script>
{{else}}
  <h1>Sign in to Victory</h1><p>Continue with your Telegram account.</p>
  <script async src="https://telegram.org/js/telegram-widget.js?22"
    data-telegram-login="{{.Bot}}" data-size="large" data-radius="20"
    data-auth-url="{{.ReturnURL}}" data-request-access="write"></script>
{{end}}
</main></body></html>`))

type telegramPageData struct {
	Bot, LoginURL, ReturnURL, Error string
	AppURL                          template.URL
}

func (s *Server) telegramPageData() telegramPageData {
	base := s.cfg.PublicBaseURL + "/api/v1/auth/telegram"
	return telegramPageData{Bot: s.cfg.TelegramBotUsername, LoginURL: base + "/login", ReturnURL: base + "/return"}
}

func (s *Server) renderTelegramPage(c *gin.Context, status int, d telegramPageData) {
	c.Header("Cache-Control", "no-store")
	c.Status(status)
	c.Header("Content-Type", "text/html; charset=utf-8")
	_ = telegramPage.Execute(c.Writer, d)
}

func (s *Server) telegramLoginPage(c *gin.Context) {
	d := s.telegramPageData()
	if d.Bot == "" || s.cfg.TelegramBotToken == "" {
		d.Error = "Telegram sign-in isn't set up yet."
		s.renderTelegramPage(c, http.StatusServiceUnavailable, d)
		return
	}
	s.renderTelegramPage(c, http.StatusOK, d)
}

// telegramLoginReturn checks the widget's signed login and passes it to the
// app unchanged (only Telegram's fields, nothing else).
func (s *Server) telegramLoginReturn(c *gin.Context) {
	d := s.telegramPageData()
	fields := map[string]string{}
	for k, v := range c.Request.URL.Query() {
		if len(v) > 0 {
			fields[k] = v[0]
		}
	}
	if _, err := auth.VerifyTelegramLogin(s.cfg.TelegramBotToken, fields, 24*time.Hour); err != nil {
		d.Error = "Telegram couldn't confirm your sign-in. Try again."
		s.renderTelegramPage(c, http.StatusUnauthorized, d)
		return
	}
	q := url.Values{}
	for k, v := range fields {
		q.Set(k, v)
	}
	d.AppURL = template.URL("victory://telegram-login?" + q.Encode())
	s.renderTelegramPage(c, http.StatusOK, d)
}
