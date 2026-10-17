//go:build firestore

package live

// A write to the real Firestore database, then a read the way a phone reads
// (no credentials), which also checks the security rules. Run with the
// Firebase variables set:
//
//	go test -tags firestore ./internal/live

import (
	"encoding/json"
	"net/http"
	"os"
	"strings"
	"testing"
	"time"
)

func TestFirestoreRoom(t *testing.T) {
	project := os.Getenv("FIREBASE_PROJECT_ID")
	r := New(project, os.Getenv("FIREBASE_CLIENT_EMAIL"), os.Getenv("FIREBASE_PRIVATE_KEY"), os.Getenv("FIRESTORE_DATABASE"))
	if r == nil {
		t.Skip("Firebase not configured")
	}
	id := "selftest"
	if err := r.Update(t.Context(), id, Room{Status: "waiting", Members: 2, Submitted: 0}); err != nil {
		t.Fatal(err)
	}
	t.Log("write ok")

	res, err := http.Get(r.base + id)
	if err != nil {
		t.Fatal(err)
	}
	defer res.Body.Close()
	var doc struct {
		Fields map[string]map[string]string `json:"fields"`
	}
	_ = json.NewDecoder(res.Body).Decode(&doc)
	if res.StatusCode != 200 || doc.Fields["members"]["integerValue"] != "2" {
		t.Fatalf("anonymous read: %d %v (are the security rules published?)", res.StatusCode, doc.Fields)
	}
	t.Log("anonymous read ok")

	// Writing without credentials must be refused.
	req, _ := http.NewRequest(http.MethodPatch, r.base+id, strings.NewReader(`{"fields":{"members":{"integerValue":"99"}}}`))
	res, err = http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	res.Body.Close()
	if res.StatusCode < 400 {
		t.Fatalf("anonymous write was allowed: %d", res.StatusCode)
	}
	// Nor can anyone list the rooms.
	res, err = http.Get(strings.TrimSuffix(r.base, "/") + "?pageSize=1")
	if err != nil {
		t.Fatal(err)
	}
	res.Body.Close()
	if res.StatusCode < 400 {
		t.Fatalf("anonymous listing was allowed: %d", res.StatusCode)
	}
	t.Log("anonymous write and listing refused", time.Now().Format(time.TimeOnly))

	req, _ = http.NewRequestWithContext(t.Context(), http.MethodDelete, r.base+id, nil)
	if res, err = r.client.Do(req); err != nil {
		t.Fatal(err)
	}
	res.Body.Close()
	if res.StatusCode != 200 {
		t.Fatalf("delete test room: %d", res.StatusCode)
	}
}
