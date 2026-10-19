package api

import (
	"bytes"
	"crypto/rand"
	_ "embed"
	"encoding/base64"
	"encoding/hex"
	"html/template"
	"mime"
	"mime/multipart"
	"mime/quotedprintable"
	"net/textproto"
	"strconv"
	"strings"
	"time"
)

// The password reset email: an HTML message with the eagle logo attached
// inline (no remote images, so it shows without "display images"), and a
// plain-text copy for mail apps that don't show HTML.

//go:embed email/eagle.png
var emailLogo []byte

type resetEmailData struct {
	Name, Email, Code    string
	Minutes, Year        int
	Support, SupportLink string
}

// support is how to reach the academy: the support email, or else Telegram.
func (d *resetEmailData) support(contacts pageData) {
	switch {
	case contacts.Email != "":
		d.Support, d.SupportLink = contacts.Email, "mailto:"+contacts.Email
	case contacts.Telegram != "":
		d.Support, d.SupportLink = "Telegram", contacts.Telegram
	}
}

var resetEmailHTML = template.Must(template.New("reset").Parse(`<!doctype html>
<html lang="en"><head><meta charset="utf-8"><meta name="viewport" content="width=device-width,initial-scale=1">
<meta name="color-scheme" content="light"><meta name="supported-color-schemes" content="light">
<title>Reset your Victory password</title>
<style>@media only screen and (max-width:480px){.card{padding:28px 22px 24px !important}.code{font-size:30px !important;letter-spacing:6px !important}}</style></head>
<body style="margin:0;padding:0;background:#f2f4f6;-webkit-text-size-adjust:100%;">
<div style="display:none;max-height:0;overflow:hidden;opacity:0;color:#f2f4f6;">Your code is {{.Code}}. It expires in {{.Minutes}} minutes.</div>
<table role="presentation" width="100%" cellpadding="0" cellspacing="0" border="0" style="background:#f2f4f6;">
<tr><td align="center" style="padding:32px 16px;">
<table role="presentation" width="100%" cellpadding="0" cellspacing="0" border="0" style="max-width:560px;">
<tr><td style="padding:0 8px 20px;">
  <table role="presentation" cellpadding="0" cellspacing="0" border="0"><tr>
    <td style="vertical-align:middle;"><img src="cid:victory-logo" width="40" height="40" alt="" style="display:block;border:0;"></td>
    <td style="vertical-align:middle;padding-left:10px;font:700 20px/1 'Segoe UI',Roboto,Helvetica,Arial,sans-serif;color:#0d4e7f;letter-spacing:-0.3px;">Victory</td>
  </tr></table>
</td></tr>
<tr><td class="card" style="background:#ffffff;border-radius:16px;border:1px solid #e3e7eb;padding:40px 40px 32px;font-family:'Segoe UI',Roboto,Helvetica,Arial,sans-serif;">
  <h1 style="margin:0 0 20px;font-size:24px;line-height:32px;font-weight:600;color:#121715;">Reset your password</h1>
  <p style="margin:0 0 16px;font-size:15px;line-height:24px;color:#3c434a;">Hi {{.Name}},</p>
  <p style="margin:0 0 24px;font-size:15px;line-height:24px;color:#3c434a;">We received a request to reset the password for your Victory account <strong style="color:#121715;">{{.Email}}</strong>. Enter this code in the Victory app to choose a new password:</p>
  <table role="presentation" width="100%" cellpadding="0" cellspacing="0" border="0"><tr>
    <td align="center" style="background:#f0f6fb;border:1px solid #d6e6f2;border-radius:12px;padding:20px 12px;">
      <div class="code" style="font:700 34px/40px 'Segoe UI',Roboto,Helvetica,Arial,sans-serif;font-variant-numeric:tabular-nums;letter-spacing:8px;color:#0d4e7f;padding-left:8px;">{{.Code}}</div>
    </td>
  </tr></table>
  <p style="margin:16px 0 28px;font-size:13px;line-height:20px;color:#5b626a;text-align:center;">This code expires in {{.Minutes}} minutes and can be used once.</p>
  <table role="presentation" width="100%" cellpadding="0" cellspacing="0" border="0"><tr><td style="border-top:1px solid #e3e7eb;padding-top:24px;">
    <p style="margin:0 0 12px;font-size:14px;line-height:22px;color:#3c434a;"><strong style="color:#121715;">Didn't request this?</strong> You can safely ignore this email. Your password won't change unless someone enters this code.</p>
    <p style="margin:0;font-size:14px;line-height:22px;color:#3c434a;"><strong style="color:#121715;">Keep this code private.</strong> Victory Academy will never ask you for it by phone, message or email.</p>
  </td></tr></table>
</td></tr>
<tr><td style="padding:24px 8px 0;font-family:'Segoe UI',Roboto,Helvetica,Arial,sans-serif;font-size:12px;line-height:19px;color:#80878e;">
  <p style="margin:0 0 8px;">You're receiving this email because a password reset was requested for your Victory account.{{if .Support}} Need help? Contact us at <a href="{{.SupportLink}}" style="color:#0d4e7f;text-decoration:underline;">{{.Support}}</a>.{{end}}</p>
  <p style="margin:0;">&copy; {{.Year}} Victory Academy &middot; Addis Ababa, Ethiopia</p>
</td></tr>
</table>
</td></tr></table>
</body></html>`))

func resetEmailText(d resetEmailData) string {
	var b strings.Builder
	b.WriteString("Reset your password\n\n")
	b.WriteString("Hi " + d.Name + ",\n\n")
	b.WriteString("We received a request to reset the password for your Victory account " + d.Email + ". ")
	b.WriteString("Enter this code in the Victory app to choose a new password:\n\n")
	b.WriteString("    " + d.Code + "\n\n")
	b.WriteString("This code expires in " + strconv.Itoa(d.Minutes) + " minutes and can be used once.\n\n")
	b.WriteString("Didn't request this? You can safely ignore this email. Your password won't change unless someone enters this code.\n\n")
	b.WriteString("Keep this code private. Victory Academy will never ask you for it by phone, message or email.\n\n")
	b.WriteString("--\n")
	if d.Support != "" {
		b.WriteString("Need help? " + strings.TrimPrefix(d.SupportLink, "mailto:") + "\n")
	}
	b.WriteString("Victory Academy, Addis Ababa, Ethiopia\n")
	return strings.ReplaceAll(b.String(), "\n", "\r\n")
}

// resetEmail builds the whole message, with the headers mail providers
// expect (without Date and Message-ID, Gmail tends to file it as spam).
func resetEmail(from, fromAddress string, d resetEmailData, now time.Time) ([]byte, error) {
	var html bytes.Buffer
	if err := resetEmailHTML.Execute(&html, d); err != nil {
		return nil, err
	}
	id := make([]byte, 12)
	_, _ = rand.Read(id)
	domain := fromAddress[strings.LastIndex(fromAddress, "@")+1:]

	var msg bytes.Buffer
	alt := multipart.NewWriter(&msg)
	header := func(k, v string) { msg.WriteString(k + ": " + v + "\r\n") }
	header("From", from)
	header("To", d.Email)
	header("Subject", mime.QEncoding.Encode("utf-8", d.Code+" is your Victory password reset code"))
	header("Date", now.Format(time.RFC1123Z))
	header("Message-ID", "<"+hex.EncodeToString(id)+"@"+domain+">")
	header("Auto-Submitted", "auto-generated")
	header("X-Auto-Response-Suppress", "All")
	header("MIME-Version", "1.0")
	header("Content-Type", "multipart/alternative; boundary="+alt.Boundary())
	msg.WriteString("\r\n")

	part := func(w *multipart.Writer, contentType, body string) error {
		p, err := w.CreatePart(textproto.MIMEHeader{
			"Content-Type":              {contentType + "; charset=utf-8"},
			"Content-Transfer-Encoding": {"quoted-printable"},
		})
		if err != nil {
			return err
		}
		q := quotedprintable.NewWriter(p)
		if _, err := q.Write([]byte(body)); err != nil {
			return err
		}
		return q.Close()
	}
	if err := part(alt, "text/plain", resetEmailText(d)); err != nil {
		return nil, err
	}
	// The HTML and its inline logo.
	var related bytes.Buffer
	rel := multipart.NewWriter(&related)
	if err := part(rel, "text/html", html.String()); err != nil {
		return nil, err
	}
	img, err := rel.CreatePart(textproto.MIMEHeader{
		"Content-Type":              {"image/png"},
		"Content-Transfer-Encoding": {"base64"},
		"Content-ID":                {"<victory-logo>"},
		"Content-Disposition":       {`inline; filename="victory.png"`},
	})
	if err != nil {
		return nil, err
	}
	encoded := base64.StdEncoding.EncodeToString(emailLogo)
	for len(encoded) > 76 {
		_, _ = img.Write([]byte(encoded[:76] + "\r\n"))
		encoded = encoded[76:]
	}
	_, _ = img.Write([]byte(encoded + "\r\n"))
	if err := rel.Close(); err != nil {
		return nil, err
	}
	p, err := alt.CreatePart(textproto.MIMEHeader{"Content-Type": {"multipart/related; boundary=" + rel.Boundary()}})
	if err != nil {
		return nil, err
	}
	if _, err := p.Write(related.Bytes()); err != nil {
		return nil, err
	}
	if err := alt.Close(); err != nil {
		return nil, err
	}
	return msg.Bytes(), nil
}

// firstName greets "Hi Abebe," rather than with the full name.
func firstName(name string) string {
	if f := strings.Fields(name); len(f) > 0 {
		return f[0]
	}
	return "there"
}
