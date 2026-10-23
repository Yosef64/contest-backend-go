package httpx

import (
	"compress/gzip"
	"strings"
	"sync"

	"github.com/gin-gonic/gin"
)

// Gzip compresses text responses (JSON, HTML, SVG, plain text) for clients
// that accept it. Lesson HTML and question lists shrink several times over,
// which matters on slow, metered mobile data. Images and other binary
// bodies, and replies without a body (204, 304), are sent as they are.
func Gzip() gin.HandlerFunc {
	return func(c *gin.Context) {
		if !strings.Contains(c.GetHeader("Accept-Encoding"), "gzip") {
			c.Next()
			return
		}
		w := &gzipWriter{ResponseWriter: c.Writer}
		c.Writer = w
		defer w.close()
		c.Next()
	}
}

var gzipPool = sync.Pool{New: func() any {
	gz, _ := gzip.NewWriterLevel(nil, gzip.DefaultCompression)
	return gz
}}

type gzipWriter struct {
	gin.ResponseWriter
	gz      *gzip.Writer
	decided bool
}

// compressible: text bodies not already encoded by the handler.
func (w *gzipWriter) compressible() bool {
	h := w.Header()
	if h.Get("Content-Encoding") != "" {
		return false
	}
	t := h.Get("Content-Type")
	return strings.HasPrefix(t, "application/json") || strings.HasPrefix(t, "text/") ||
		strings.HasPrefix(t, "image/svg+xml") || strings.HasPrefix(t, "application/javascript")
}

// decide runs before the first byte of body: headers can still change.
func (w *gzipWriter) decide() {
	if w.decided {
		return
	}
	w.decided = true
	if !w.compressible() {
		return
	}
	h := w.Header()
	h.Set("Content-Encoding", "gzip")
	h.Add("Vary", "Accept-Encoding")
	h.Del("Content-Length")
	gz := gzipPool.Get().(*gzip.Writer)
	gz.Reset(w.ResponseWriter)
	w.gz = gz
}

func (w *gzipWriter) Write(b []byte) (int, error) {
	w.decide()
	if w.gz == nil {
		return w.ResponseWriter.Write(b)
	}
	return w.gz.Write(b)
}

func (w *gzipWriter) WriteString(s string) (int, error) {
	return w.Write([]byte(s))
}

func (w *gzipWriter) close() {
	if w.gz == nil {
		return
	}
	_ = w.gz.Close()
	gzipPool.Put(w.gz)
	w.gz = nil
}
