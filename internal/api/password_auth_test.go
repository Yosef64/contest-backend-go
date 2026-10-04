package api

import "testing"

func TestNormalizeLogin(t *testing.T) {
	cases := map[string]string{
		"Student@Example.com": "student@example.com",
		"0911223344":          "+251911223344",
		"+251 911 22 33 44":   "+251911223344",
		"251711223344":        "+251711223344",
		"911-223-344":         "+251911223344",
	}
	for in, want := range cases {
		got, _, _, ok := normalizeLogin(in)
		if !ok || got != want {
			t.Errorf("normalizeLogin(%q) = %q, %v; want %q", in, got, ok, want)
		}
	}
	for _, bad := range []string{"", "12345", "0811223344", "a@b", "x <a@b.com>", "+1 202 555 0100"} {
		if _, _, _, ok := normalizeLogin(bad); ok {
			t.Errorf("normalizeLogin(%q) accepted", bad)
		}
	}
}

func TestValidPassword(t *testing.T) {
	for pw, want := range map[string]bool{"secret123": true, "short1": false, "onlyletters": false, "12345678": false} {
		if validPassword(pw) != want {
			t.Errorf("validPassword(%q) != %v", pw, want)
		}
	}
}

func TestLoginLimiter(t *testing.T) {
	l := &loginLimiter{}
	for i := 0; i < limitPerLogin; i++ {
		l.fail("login:a", "ip:1")
	}
	if !l.blocked("login:a") {
		t.Fatal("login should be blocked after the per-login limit")
	}
	if l.blocked("ip:1") {
		t.Fatal("address should allow more failures than a single login")
	}
	l.reset("login:a")
	if l.blocked("login:a") {
		t.Fatal("reset should clear the login")
	}
}

func TestAcademicLevel(t *testing.T) {
	cases := []struct {
		grade, dept, stream          string
		wantGrade, wantDept, wantStr string
		ok                           bool
	}{
		{"9", "", "natural", "9", "", "", true},         // no stream before grade 11
		{"9", "entrance", "natural", "9", "", "", true}, // old clients: never promoted
		{"11", "entrance", "social", "11", "", "social", true},
		{"11", "", "", "11", "", "", false}, // grade 11 needs a stream
		{"12", "", "natural", "12", "entrance", "natural", true},
		{"10", "remedial", "social", "12", "remedial", "social", true},
		{"", "freshman", "natural", "12", "freshman", "natural", true},
		{"9", "postgrad", "", "9", "postgrad", "", false},
	}
	for _, c := range cases {
		g, d, s, fields := academicLevel(c.grade, c.dept, c.stream)
		if (len(fields) == 0) != c.ok || (c.ok && (g != c.wantGrade || d != c.wantDept || s != c.wantStr)) {
			t.Errorf("academicLevel(%q,%q,%q) = %q,%q,%q,%v", c.grade, c.dept, c.stream, g, d, s, fields)
		}
	}
}
