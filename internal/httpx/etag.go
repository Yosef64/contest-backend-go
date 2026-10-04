package httpx

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"
)

// OKCached writes body as JSON with an ETag. When the client already holds
// that version (If-None-Match), it answers 304 Not Modified with no body, so
// the app can keep its cached copy. The ETag is a hash of the body, or of
// key when given (for bodies with parts that change on every request).
//
// Responses are per student, so they are marked private.
func OKCached(c *gin.Context, body any, key ...string) error {
	data, err := json.Marshal(body)
	if err != nil {
		return err
	}
	source := data
	if len(key) > 0 {
		source = []byte(strings.Join(key, "\x00"))
	}
	sum := sha256.Sum256(source)
	etag := `"` + hex.EncodeToString(sum[:12]) + `"`
	c.Header("ETag", etag)
	c.Header("Cache-Control", "private, no-cache")
	if match := c.GetHeader("If-None-Match"); match != "" {
		for _, m := range strings.Split(match, ",") {
			if strings.TrimSpace(m) == etag {
				c.Status(http.StatusNotModified)
				return nil
			}
		}
	}
	c.Data(http.StatusOK, "application/json; charset=utf-8", data)
	return nil
}
