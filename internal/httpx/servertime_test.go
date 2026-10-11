package httpx

import (
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
)

func TestServerTimeStampsEveryResponse(t *testing.T) {
	gin.SetMode(gin.TestMode)
	fixed := time.UnixMilli(1791400000123)
	r := gin.New()
	r.Use(ServerTime(func() time.Time { return fixed }))
	r.GET("/json", func(c *gin.Context) { c.JSON(http.StatusOK, gin.H{"ok": true}) })
	r.GET("/empty", func(c *gin.Context) { c.Status(http.StatusNoContent) })
	r.GET("/cached", func(c *gin.Context) { c.Status(http.StatusNotModified) })
	for _, path := range []string{"/json", "/empty", "/cached", "/missing"} {
		w := httptest.NewRecorder()
		r.ServeHTTP(w, httptest.NewRequest(http.MethodGet, path, nil))
		if got := w.Header().Get(ServerTimeHeader); got != "1791400000123" {
			t.Fatalf("%s: %s = %q", path, ServerTimeHeader, got)
		}
	}
}
