package api

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"net/http"
	"net/http/httptest"
	"net/url"
	"sort"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/gin-gonic/gin"

	"victor-contest-go/internal/config"
)

func signTelegram(token string, fields map[string]string) string {
	keys := make([]string, 0, len(fields))
	for k := range fields {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	lines := make([]string, len(keys))
	for i, k := range keys {
		lines[i] = k + "=" + fields[k]
	}
	secret := sha256.Sum256([]byte(token))
	mac := hmac.New(sha256.New, secret[:])
	mac.Write([]byte(strings.Join(lines, "\n")))
	return hex.EncodeToString(mac.Sum(nil))
}

func TestTelegramLoginPages(t *testing.T) {
	gin.SetMode(gin.TestMode)
	s := &Server{cfg: config.Config{TelegramBotToken: "123:abc", TelegramBotUsername: "VictoryLearningBot", PublicBaseURL: "https://api.example"}}
	r := gin.New()
	r.GET("/login", s.telegramLoginPage)
	r.GET("/return", s.telegramLoginReturn)
	get := func(path string) *httptest.ResponseRecorder {
		w := httptest.NewRecorder()
		r.ServeHTTP(w, httptest.NewRequest(http.MethodGet, path, nil))
		return w
	}

	w := get("/login")
	if w.Code != 200 || !strings.Contains(w.Body.String(), `data-telegram-login="VictoryLearningBot"`) ||
		!strings.Contains(w.Body.String(), `data-auth-url="https://api.example/api/v1/auth/telegram/return"`) {
		t.Fatalf("login page: %d %s", w.Code, w.Body)
	}

	fields := map[string]string{"id": "42", "first_name": "Abel", "auth_date": strconv.FormatInt(time.Now().Unix(), 10)}
	q := url.Values{}
	for k, v := range fields {
		q.Set(k, v)
	}
	q.Set("hash", signTelegram("123:abc", fields))
	w = get("/return?" + q.Encode())
	if w.Code != 200 || !strings.Contains(w.Body.String(), "victory://telegram-login?") || !strings.Contains(w.Body.String(), "first_name=Abel") {
		t.Fatalf("return page: %d %s", w.Code, w.Body)
	}

	q.Set("first_name", "Mallory")
	if w = get("/return?" + q.Encode()); w.Code != http.StatusUnauthorized || strings.Contains(w.Body.String(), "victory://") {
		t.Fatalf("tampered login accepted: %d", w.Code)
	}
}
