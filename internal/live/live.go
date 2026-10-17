// Package live tells phones that something changed, through Cloud
// Firestore. A phone in a group exam room listens to its room's document
// (group_rooms/<group id>) instead of asking the API every few seconds;
// when the document changes, it fetches the room from the API once.
//
// The documents hold no personal data: the room's status, its member and
// submission counts, and a version that changes on every update. The API
// stays the only source of truth. Security rules let anyone read a room
// document by its ID (a random UUID only members know) and nobody write.
package live

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"golang.org/x/oauth2"
	"golang.org/x/oauth2/google"
	"golang.org/x/oauth2/jwt"
)

const scope = "https://www.googleapis.com/auth/datastore"

// Rooms writes group exam room documents.
type Rooms struct {
	base   string
	client *http.Client
}

// New returns nil when Firebase isn't configured (no project or key); a nil
// Rooms writes nothing. privateKey is the service account's PEM key ("\n"
// escapes are accepted). database is the Firestore database ID, usually
// "(default)".
func New(projectID, clientEmail, privateKey, database string) *Rooms {
	if projectID == "" || clientEmail == "" || privateKey == "" {
		return nil
	}
	if database == "" {
		database = "(default)"
	}
	cfg := &jwt.Config{
		Email:      clientEmail,
		PrivateKey: []byte(strings.ReplaceAll(privateKey, `\n`, "\n")),
		Scopes:     []string{scope},
		TokenURL:   google.JWTTokenURL,
	}
	return &Rooms{
		base:   fmt.Sprintf("https://firestore.googleapis.com/v1/projects/%s/databases/%s/documents/group_rooms/", url.PathEscape(projectID), url.PathEscape(database)),
		client: oauth2.NewClient(context.Background(), cfg.TokenSource(context.Background())),
	}
}

// Room is what a room document says.
type Room struct {
	Status    string
	Members   int
	Submitted int
}

// Update writes the room's document. It waits at most 3 seconds: a slow or
// failed write only means phones notice the change at their next check.
func (r *Rooms) Update(ctx context.Context, groupID string, room Room) error {
	if r == nil {
		return nil
	}
	ctx, cancel := context.WithTimeout(ctx, 3*time.Second)
	defer cancel()
	body, _ := json.Marshal(map[string]any{"fields": map[string]any{
		"status":    map[string]string{"stringValue": room.Status},
		"members":   map[string]string{"integerValue": strconv.Itoa(room.Members)},
		"submitted": map[string]string{"integerValue": strconv.Itoa(room.Submitted)},
		"version":   map[string]string{"integerValue": strconv.FormatInt(time.Now().UnixMilli(), 10)},
	}})
	req, err := http.NewRequestWithContext(ctx, http.MethodPatch, r.base+url.PathEscape(groupID), bytes.NewReader(body))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	res, err := r.client.Do(req)
	if err != nil {
		return err
	}
	defer res.Body.Close()
	if res.StatusCode >= 300 {
		msg, _ := io.ReadAll(io.LimitReader(res.Body, 300))
		return fmt.Errorf("firestore %d: %s", res.StatusCode, msg)
	}
	return nil
}
