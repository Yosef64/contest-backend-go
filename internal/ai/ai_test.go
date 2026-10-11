package ai

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestCompleteSendsTheRequestAndReadsText(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/messages" || r.Header.Get("x-api-key") != "k" || r.Header.Get("anthropic-version") == "" {
			t.Errorf("bad request: %s %v", r.URL.Path, r.Header)
		}
		var body request
		_ = json.NewDecoder(r.Body).Decode(&body)
		if body.Model != "m" || body.System != "sys" || len(body.Messages) != 1 || body.MaxTokens != 50 {
			t.Errorf("bad body: %+v", body)
		}
		_, _ = w.Write([]byte(`{"content":[{"type":"text","text":"Hello "},{"type":"text","text":"there"}]}`))
	}))
	defer srv.Close()
	c := New("k", "m", srv.URL)
	got, err := c.Complete(context.Background(), "sys", []Message{{Role: "user", Content: "hi"}}, 50)
	if err != nil || got != "Hello there" {
		t.Fatalf("got %q, %v", got, err)
	}
}

func TestJSONToleratesSurroundingText(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`{"content":[{"type":"text","text":"Sure!\n` + "```json" + `\n{\"summary\":\"ok\",\"n\":2}\n` + "```" + `"}]}`))
	}))
	defer srv.Close()
	var out struct {
		Summary string `json:"summary"`
		N       int    `json:"n"`
	}
	if err := New("k", "m", srv.URL).JSON(context.Background(), "s", "p", 10, &out); err != nil || out.Summary != "ok" || out.N != 2 {
		t.Fatalf("got %+v, %v", out, err)
	}
}

func TestErrorsAndDisabled(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusTooManyRequests)
		_, _ = w.Write([]byte(`{"type":"error","error":{"type":"rate_limit_error","message":"slow down"}}`))
	}))
	defer srv.Close()
	if _, err := New("k", "m", srv.URL).Complete(context.Background(), "", []Message{{Role: "user", Content: "x"}}, 5); err == nil {
		t.Fatal("expected an error")
	}
	if _, err := New("", "m", srv.URL).Complete(context.Background(), "", nil, 5); !errors.Is(err, ErrDisabled) {
		t.Fatalf("expected ErrDisabled, got %v", err)
	}
}
