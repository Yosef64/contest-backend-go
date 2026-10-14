// Package push sends notifications to the app through Firebase Cloud
// Messaging (HTTP v1). Phones subscribe to topics (see the app's Push
// class): "all", "dept_<department>", "school_<id>" and "student_<id>".
package push

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"regexp"
	"strings"

	"golang.org/x/oauth2"
	"golang.org/x/oauth2/google"
	"golang.org/x/oauth2/jwt"
)

const scope = "https://www.googleapis.com/auth/firebase.messaging"

// Sender sends to FCM as a Firebase service account.
type Sender struct {
	projectID string
	client    *http.Client
}

// New returns nil when push isn't configured (no project or key).
// privateKey is the service account's PEM key; "\n" escapes are accepted
// so it can live in one environment variable.
func New(projectID, clientEmail, privateKey string) (*Sender, error) {
	if projectID == "" || clientEmail == "" || privateKey == "" {
		return nil, nil
	}
	cfg := &jwt.Config{
		Email:      clientEmail,
		PrivateKey: []byte(strings.ReplaceAll(privateKey, `\n`, "\n")),
		Scopes:     []string{scope},
		TokenURL:   google.JWTTokenURL,
	}
	return &Sender{projectID: projectID, client: oauth2.NewClient(context.Background(), cfg.TokenSource(context.Background()))}, nil
}

var unsafe = regexp.MustCompile(`[^A-Za-z0-9_.~%-]`)

// Topic names a topic the app subscribes to, e.g. Topic("school", id).
func Topic(kind, value string) string {
	if kind == "all" {
		return "all"
	}
	return kind + "_" + unsafe.ReplaceAllString(value, "_")
}

// Audience is who an announcement is for: everyone, a department, a
// school, or students of a department at a school.
func Audience(department string, schoolID *string) (topic, condition string) {
	var parts []string
	if department != "" {
		parts = append(parts, Topic("dept", department))
	}
	if schoolID != nil && *schoolID != "" {
		parts = append(parts, Topic("school", *schoolID))
	}
	switch len(parts) {
	case 0:
		return "all", ""
	case 1:
		return parts[0], ""
	default:
		return "", fmt.Sprintf("'%s' in topics && '%s' in topics", parts[0], parts[1])
	}
}

// Message is one notification.
type Message struct {
	Topic, Condition string
	Title, Body      string
	Data             map[string]string
}

// Send delivers a message. Android shows it on the "Announcements" channel.
func (s *Sender) Send(ctx context.Context, m Message) error {
	if s == nil {
		return nil
	}
	if m.Topic == "" && m.Condition == "" {
		return errors.New("push: no topic or condition")
	}
	msg := map[string]any{
		"notification": map[string]string{"title": m.Title, "body": m.Body},
		"android": map[string]any{
			"priority":     "high",
			"notification": map[string]string{"channel_id": "victory_announcements"},
		},
		"apns": map[string]any{"payload": map[string]any{"aps": map[string]any{"sound": "default"}}},
	}
	if m.Topic != "" {
		msg["topic"] = m.Topic
	} else {
		msg["condition"] = m.Condition
	}
	if len(m.Data) > 0 {
		msg["data"] = m.Data
	}
	body, err := json.Marshal(map[string]any{"message": msg})
	if err != nil {
		return err
	}
	url := "https://fcm.googleapis.com/v1/projects/" + s.projectID + "/messages:send"
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(body))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	res, err := s.client.Do(req)
	if err != nil {
		return err
	}
	defer res.Body.Close()
	if res.StatusCode != http.StatusOK {
		detail, _ := io.ReadAll(io.LimitReader(res.Body, 500))
		return fmt.Errorf("push: FCM %d: %s", res.StatusCode, detail)
	}
	return nil
}
