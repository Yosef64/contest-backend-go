package httpx

import (
	"strconv"
	"time"

	"github.com/gin-gonic/gin"
)

// ServerTimeHeader carries the server's clock (Unix milliseconds) at the
// moment a response is sent, so apps can correct a phone whose clock is
// wrong. Stamped as late as possible: slow handlers don't skew it.
const ServerTimeHeader = "X-Server-Time"

// ServerTime stamps every response with ServerTimeHeader.
func ServerTime(now func() time.Time) gin.HandlerFunc {
	return func(c *gin.Context) {
		w := &timeWriter{ResponseWriter: c.Writer, now: now}
		c.Writer = w
		c.Next()
		w.stamp() // bodiless replies (204, 304) are sent after the handlers

	}
}

type timeWriter struct {
	gin.ResponseWriter
	now func() time.Time
}

func (w *timeWriter) stamp() {
	if !w.Written() {
		w.Header().Set(ServerTimeHeader, strconv.FormatInt(w.now().UnixMilli(), 10))
	}
}

func (w *timeWriter) WriteHeaderNow() {
	w.stamp()
	w.ResponseWriter.WriteHeaderNow()
}

func (w *timeWriter) Write(b []byte) (int, error) {
	w.stamp()
	return w.ResponseWriter.Write(b)
}

func (w *timeWriter) WriteString(s string) (int, error) {
	w.stamp()
	return w.ResponseWriter.WriteString(s)
}
