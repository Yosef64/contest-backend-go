//go:build integration

package api

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/golang-jwt/jwt/v5"

	"victor-contest-go/internal/auth"
	"victor-contest-go/internal/httpx"
)

// try runs a handler and returns its status and body, errors included.
func try(t *testing.T, h func(*gin.Context) error, body string, set map[string]any) (int, map[string]any) {
	t.Helper()
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = httptest.NewRequest("POST", "/", strings.NewReader(body))
	c.Request.Header.Set("Content-Type", "application/json")
	for k, v := range set {
		c.Set(k, v)
	}
	if err := h(c); err != nil {
		var he *httpx.Error
		if errors.As(err, &he) {
			return he.Status, map[string]any{"error": he.Message, "fields": he.Fields}
		}
		return 500, map[string]any{"error": err.Error()}
	}
	out := map[string]any{}
	_ = json.Unmarshal(w.Body.Bytes(), &out)
	return w.Code, out
}

func testCtx() *gin.Context {
	c, _ := gin.CreateTestContext(httptest.NewRecorder())
	c.Request = httptest.NewRequest("GET", "/", nil)
	return c
}

func TestIntegrationOneAccountPerEmail(t *testing.T) {
	s := integrationServer(t)
	ctx := context.Background()
	for _, q := range []string{
		`DELETE FROM google_accounts WHERE email LIKE '%@one.test'`,
		`DELETE FROM password_resets WHERE login LIKE '%@one.test'`,
		`DELETE FROM student_credentials WHERE login LIKE '%@one.test'`,
		`DELETE FROM students WHERE name LIKE 'ONE %'`,
	} {
		exec(t, s, q)
	}
	student := func(id, provider, subject, name string) {
		exec(t, s, `INSERT INTO students (id, auth_provider, auth_subject, name, grade) VALUES ($1, $2, $3, $4, '10')`, id, provider, subject, name)
	}
	signUp := func(email string) (int, map[string]any) {
		return try(t, s.signUpPassword, `{"name":"ONE Student","login":"`+email+`","password":"Passw0rd!x"}`, nil)
	}
	credential := func(email string) (studentID string, proven bool, ok bool) {
		err := s.pool.QueryRow(ctx, `SELECT student_id, email_verified_at IS NOT NULL FROM student_credentials WHERE login = $1`, email).Scan(&studentID, &proven)
		return studentID, proven, err == nil
	}
	resetWith := func(email, password string) (int, map[string]any) {
		code, _, err := s.issueReset(ctx, email, nil)
		if err != nil {
			t.Fatal(err)
		}
		return try(t, s.resetPassword, `{"login":"`+email+`","code":"`+code+`","password":"`+password+`"}`, nil)
	}
	studentOf := func(body map[string]any) string {
		// The test server's tokens have no lifetime: read without checking it.
		claims := &auth.Claims{}
		if _, _, err := jwt.NewParser().ParseUnverified(body["token"].(string), claims); err != nil {
			t.Fatal(err)
		}
		return claims.Subject
	}

	// Google first: password sign-up is refused; a password is added with
	// an emailed code and opens the same account.
	id, err := s.googleStudent(testCtx(), "a@one.test", "sub-a")
	if err != nil || id != "g_sub-a" {
		t.Fatalf("google first: %q %v", id, err)
	}
	student(id, "google", "sub-a", "ONE A")
	if code, body := signUp("a@one.test"); code != 422 || !strings.Contains(fmt.Sprint(body["fields"]), "already used with Google") {
		t.Fatalf("sign-up with a Google email should be refused: %d %v", code, body)
	}
	if _, _, ok := credential("a@one.test"); ok {
		t.Fatal("no second account")
	}
	code, body := resetWith("a@one.test", "NewPassw0rd!")
	if code != 200 || studentOf(body) != "g_sub-a" || body["student"] == nil {
		t.Fatalf("adding a password: %d %v", code, body)
	}
	if sid, proven, ok := credential("a@one.test"); !ok || sid != "g_sub-a" || !proven {
		t.Fatalf("password belongs to the Google account, proven: %q %v %v", sid, proven, ok)
	}
	code, body = try(t, s.signInPassword, `{"login":"a@one.test","password":"NewPassw0rd!"}`, nil)
	if code != 200 || studentOf(body) != "g_sub-a" || body["student"] == nil {
		t.Fatalf("password sign-in opens the Google account: %d %v", code, body)
	}

	// Password first, never proven: Google opens that account and the
	// password goes (someone may have registered another person's email).
	if code, body := signUp("b@one.test"); code != 200 {
		t.Fatalf("sign-up: %d %v", code, body)
	}
	pb, _, _ := credential("b@one.test")
	student(pb, "password", "b@one.test", "ONE B")
	id, err = s.googleStudent(testCtx(), "b@one.test", "sub-b")
	if err != nil || id != pb {
		t.Fatalf("google after password: %q want %q %v", id, pb, err)
	}
	if _, _, ok := credential("b@one.test"); ok {
		t.Fatal("unproven password should be removed")
	}
	if again, _ := s.googleStudent(testCtx(), "b@one.test", "sub-b"); again != pb {
		t.Fatal("next Google sign-in opens the same account")
	}
	if st, err := s.store.StudentByID(ctx, pb); err != nil || st.Name != "ONE B" {
		t.Fatalf("progress kept: %v", err)
	}

	// Password first, proven by an emailed code: kept alongside Google.
	signUp("c@one.test")
	pc, _, _ := credential("c@one.test")
	student(pc, "password", "c@one.test", "ONE C")
	if code, body := resetWith("c@one.test", "Proven0rd!x"); code != 200 {
		t.Fatalf("reset: %d %v", code, body)
	}
	if id, _ := s.googleStudent(testCtx(), "c@one.test", "sub-c"); id != pc {
		t.Fatalf("google opens the proven password account: %q", id)
	}
	if _, proven, ok := credential("c@one.test"); !ok || !proven {
		t.Fatal("proven password stays")
	}

	// A Google account that already has its own profile is never merged
	// into a separate password account.
	signUp("d@one.test")
	pd, _, _ := credential("d@one.test")
	student("g_sub-d", "google", "sub-d", "ONE D google")
	if id, _ := s.googleStudent(testCtx(), "d@one.test", "sub-d"); id != "g_sub-d" {
		t.Fatalf("existing Google account stays its own: %q", id)
	}
	if sid, _, ok := credential("d@one.test"); !ok || sid != pd {
		t.Fatal("the separate password account is left alone")
	}

	// Deleting the account frees the email.
	// (The app's DELETE /me runs the same cleanup; it needs media storage.)
	if err := s.store.DeleteStudent(ctx, "g_sub-a"); err != nil {
		t.Fatal(err)
	}
	var left int
	_ = s.pool.QueryRow(ctx, `SELECT count(*) FROM google_accounts WHERE email = 'a@one.test'`).Scan(&left)
	if _, _, ok := credential("a@one.test"); ok || left != 0 {
		t.Fatalf("deleted account's email still taken: credential %v, google %d", ok, left)
	}
	if code, _ := signUp("a@one.test"); code != 200 {
		t.Fatal("the email can be used again")
	}
}
