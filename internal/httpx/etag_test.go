package httpx

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
)

func serve(t *testing.T, ifNoneMatch string, key ...string) *httptest.ResponseRecorder {
	t.Helper()
	gin.SetMode(gin.TestMode)
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = httptest.NewRequest(http.MethodGet, "/x", nil)
	if ifNoneMatch != "" {
		c.Request.Header.Set("If-None-Match", ifNoneMatch)
	}
	if err := OKCached(c, gin.H{"a": 1}, key...); err != nil {
		t.Fatal(err)
	}
	c.Writer.WriteHeaderNow()
	return w
}

func TestOKCached(t *testing.T) {
	first := serve(t, "")
	etag := first.Header().Get("ETag")
	if first.Code != 200 || etag == "" || first.Body.String() != `{"a":1}` {
		t.Fatalf("first response: %d %q %s", first.Code, etag, first.Body)
	}
	if again := serve(t, etag); again.Code != http.StatusNotModified || again.Body.Len() != 0 {
		t.Fatalf("matching ETag should give empty 304, got %d %q", again.Code, again.Body)
	}
	if other := serve(t, `"stale"`); other.Code != 200 {
		t.Fatalf("stale ETag should give 200, got %d", other.Code)
	}
	if keyed := serve(t, "", "chapter", "3"); keyed.Header().Get("ETag") == etag {
		t.Fatal("explicit key should drive the ETag")
	}
}
