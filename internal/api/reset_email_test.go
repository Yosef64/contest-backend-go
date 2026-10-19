package api

import (
	"bytes"
	"io"
	"mime"
	"mime/multipart"
	"net/mail"
	"strings"
	"testing"
	"time"
)

func TestResetEmail(t *testing.T) {
	d := resetEmailData{Name: firstName("<b>Abebe</b> Kebede"), Email: "abebe@example.com", Code: "041369", Minutes: 15, Year: 2026}
	d.support(pageData{Email: "help@victory.et"})
	raw, err := resetEmail("Victory Academy <no-reply@victory.et>", "no-reply@victory.et", d, time.Date(2026, 10, 19, 9, 0, 0, 0, time.UTC))
	if err != nil {
		t.Fatal(err)
	}
	m, err := mail.ReadMessage(bytes.NewReader(raw))
	if err != nil {
		t.Fatal(err)
	}
	if got := m.Header.Get("Subject"); got != "041369 is your Victory password reset code" {
		t.Errorf("subject %q", got)
	}
	for _, h := range []string{"Date", "Message-ID", "To", "From"} {
		if m.Header.Get(h) == "" {
			t.Errorf("missing %s", h)
		}
	}
	_, params, err := mime.ParseMediaType(m.Header.Get("Content-Type"))
	if err != nil {
		t.Fatal(err)
	}
	// Text first, then the HTML with its inline logo.
	parts := multipart.NewReader(m.Body, params["boundary"])
	text, err := parts.NextPart()
	if err != nil {
		t.Fatal(err)
	}
	body, _ := io.ReadAll(text)
	if !strings.Contains(string(body), "041369") || !strings.Contains(string(body), "help@victory.et") {
		t.Errorf("text part: %s", body)
	}
	related, err := parts.NextPart()
	if err != nil {
		t.Fatal(err)
	}
	_, rp, _ := mime.ParseMediaType(related.Header.Get("Content-Type"))
	inner := multipart.NewReader(related, rp["boundary"])
	html, err := inner.NextPart()
	if err != nil {
		t.Fatal(err)
	}
	page, _ := io.ReadAll(html)
	for _, want := range []string{"041369", "Hi &lt;b&gt;Abebe&lt;/b&gt;,", "mailto:help@victory.et", "cid:victory-logo"} {
		if !strings.Contains(string(page), want) {
			t.Errorf("HTML part lacks %q", want)
		}
	}
	logo, err := inner.NextPart()
	if err != nil || logo.Header.Get("Content-Id") != "<victory-logo>" {
		t.Fatalf("logo part: %v %v", err, logo.Header)
	}
}

func TestNoAccountEmail(t *testing.T) {
	d := resetEmailData{Email: "someone@example.com", NoAccount: true, Year: 2026}
	raw, err := resetEmail("Victory Academy <no-reply@victory.et>", "no-reply@victory.et", d, time.Now())
	if err != nil {
		t.Fatal(err)
	}
	m, err := mail.ReadMessage(bytes.NewReader(raw))
	if err != nil {
		t.Fatal(err)
	}
	if got := m.Header.Get("Subject"); got != "Your Victory password reset request" {
		t.Errorf("subject %q", got)
	}
	body, _ := io.ReadAll(m.Body)
	if !strings.Contains(string(body), "Continue with Google") || strings.Contains(string(body), "expires in") {
		t.Errorf("notice should point to Google sign-in and carry no code")
	}
}

func TestNoAccountNoticeOncePerHour(t *testing.T) {
	l := &noticeLimiter{sent: map[string]time.Time{}}
	now := time.Now()
	if !l.allow("a@x.com", now) || l.allow("a@x.com", now.Add(30*time.Minute)) {
		t.Fatal("second notice within the hour should be refused")
	}
	if !l.allow("b@x.com", now) || !l.allow("a@x.com", now.Add(61*time.Minute)) {
		t.Fatal("other addresses, and the same one after an hour, are allowed")
	}
}
