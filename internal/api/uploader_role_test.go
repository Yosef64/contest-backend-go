package api

import (
	"net/http"
	"testing"
)

func TestUploaderMay(t *testing.T) {
	for _, tc := range []struct {
		method, path string
		want         bool
	}{
		{http.MethodGet, "/api/v1/admin/auth/me", true},
		{http.MethodGet, "/api/v1/admin/questions", true},
		{http.MethodPost, "/api/v1/admin/questions", true},
		{http.MethodPost, "/api/v1/admin/questions/bulk", true},
		{http.MethodPut, "/api/v1/admin/questions/q1", true},
		{http.MethodDelete, "/api/v1/admin/questions/q1", false},
		{http.MethodPost, "/api/v1/admin/questions/batch", false},
		{http.MethodPost, "/api/v1/admin/media/sign", true},
		{http.MethodGet, "/api/v1/admin/media", true},
		{http.MethodDelete, "/api/v1/admin/media/m1", false},
		{http.MethodGet, "/api/v1/admin/subjects", true},
		{http.MethodPost, "/api/v1/admin/subjects", false},
		{http.MethodPut, "/api/v1/admin/auth/password", false},
		{http.MethodGet, "/api/v1/admin/students", false},
		{http.MethodGet, "/api/v1/admin/contests", false},
		{http.MethodGet, "/api/v1/admin/questionsx", false},
	} {
		if got := uploaderMay(tc.method, tc.path); got != tc.want {
			t.Errorf("%s %s = %v, want %v", tc.method, tc.path, got, tc.want)
		}
	}
}
