package httpx

import (
	"compress/gzip"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
)

func gzipServer() *gin.Engine {
	gin.SetMode(gin.TestMode)
	r := gin.New()
	r.Use(Gzip())
	r.GET("/json", func(c *gin.Context) { c.JSON(200, gin.H{"html": strings.Repeat("<p>lesson</p>", 200)}) })
	r.GET("/etag", func(c *gin.Context) { _ = OKCached(c, gin.H{"a": 1}) })
	r.GET("/png", func(c *gin.Context) { c.Data(200, "image/png", []byte("\x89PNG....")) })
	return r
}

func get(r *gin.Engine, path string, header ...string) *httptest.ResponseRecorder {
	req := httptest.NewRequest(http.MethodGet, path, nil)
	req.Header.Set("Accept-Encoding", "gzip")
	for i := 0; i+1 < len(header); i += 2 {
		req.Header.Set(header[i], header[i+1])
	}
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	return w
}

func TestGzipCompressesJSON(t *testing.T) {
	w := get(gzipServer(), "/json")
	if w.Header().Get("Content-Encoding") != "gzip" {
		t.Fatal("not compressed")
	}
	zr, err := gzip.NewReader(w.Body)
	if err != nil {
		t.Fatal(err)
	}
	body, _ := io.ReadAll(zr)
	if !strings.Contains(string(body), "lesson") || w.Body.Len() > len(body)/4 {
		t.Fatalf("bad body: %d compressed vs %d", w.Body.Len(), len(body))
	}
}

func TestGzipLeavesNotModifiedAndImagesAlone(t *testing.T) {
	r := gzipServer()
	etag := get(r, "/etag").Header().Get("ETag")
	w := get(r, "/etag", "If-None-Match", etag)
	if w.Code != http.StatusNotModified || w.Body.Len() != 0 || w.Header().Get("Content-Encoding") != "" {
		t.Fatalf("304 changed: %d %q %q", w.Code, w.Body.String(), w.Header().Get("Content-Encoding"))
	}
	if w := get(r, "/png"); w.Header().Get("Content-Encoding") != "" || w.Body.String() != "\x89PNG...." {
		t.Fatal("image was compressed")
	}
}
