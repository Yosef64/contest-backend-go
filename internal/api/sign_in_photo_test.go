package api

import "testing"

func TestSignInPhoto(t *testing.T) {
	cases := []struct {
		acc  account
		want string
	}{
		{account{Provider: "google", PhotoURL: "https://lh3.googleusercontent.com/a/abc=s96-c"}, "https://lh3.googleusercontent.com/a/abc=s256-c"},
		{account{Provider: "google", PhotoURL: "https://lh3.googleusercontent.com/a/abc"}, "https://lh3.googleusercontent.com/a/abc"},
		{account{Provider: "google", PhotoURL: "http://insecure.example/a.png"}, ""},
		{account{Provider: "google"}, ""},
		{account{Provider: "telegram", PhotoURL: "https://t.me/i/userpic/320/abc.jpg"}, "https://t.me/i/userpic/320/abc.jpg"},
		{account{Provider: "password", PhotoURL: "https://example.com/a.jpg"}, ""},
	}
	for _, tc := range cases {
		if got := signInPhoto(tc.acc); got != tc.want {
			t.Errorf("signInPhoto(%+v) = %q, want %q", tc.acc, got, tc.want)
		}
	}
}
