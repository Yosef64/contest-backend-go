// Package ai is a small client for the Anthropic Messages API, used for the
// AI tutor, study analysis, review plans and report summaries.
package ai

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"
)

// ErrDisabled: no API key configured.
var ErrDisabled = errors.New("ai is not configured")

type Client struct {
	Key     string
	Model   string
	BaseURL string
	HTTP    *http.Client
}

func New(key, model, baseURL string) *Client {
	return &Client{Key: key, Model: model, BaseURL: strings.TrimRight(baseURL, "/"), HTTP: &http.Client{Timeout: 90 * time.Second}}
}

func (c *Client) Enabled() bool { return c != nil && c.Key != "" }

type Message struct {
	Role    string `json:"role"` // "user" or "assistant"
	Content string `json:"content"`
}

type request struct {
	Model     string    `json:"model"`
	MaxTokens int       `json:"max_tokens"`
	System    string    `json:"system,omitempty"`
	Messages  []Message `json:"messages"`
}

type response struct {
	Content []struct {
		Type string `json:"type"`
		Text string `json:"text"`
	} `json:"content"`
	Error *struct {
		Type    string `json:"type"`
		Message string `json:"message"`
	} `json:"error"`
}

// Complete sends a conversation and returns the model's text reply.
func (c *Client) Complete(ctx context.Context, system string, messages []Message, maxTokens int) (string, error) {
	if !c.Enabled() {
		return "", ErrDisabled
	}
	body, _ := json.Marshal(request{Model: c.Model, MaxTokens: maxTokens, System: system, Messages: messages})
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.BaseURL+"/v1/messages", bytes.NewReader(body))
	if err != nil {
		return "", err
	}
	req.Header.Set("x-api-key", c.Key)
	req.Header.Set("anthropic-version", "2023-06-01")
	req.Header.Set("content-type", "application/json")
	res, err := c.HTTP.Do(req)
	if err != nil {
		return "", fmt.Errorf("ai: %w", err)
	}
	defer res.Body.Close()
	raw, _ := io.ReadAll(io.LimitReader(res.Body, 4<<20))
	var out response
	_ = json.Unmarshal(raw, &out)
	if res.StatusCode >= 300 {
		msg := res.Status
		if out.Error != nil {
			msg = out.Error.Message
		}
		return "", fmt.Errorf("ai %d: %s", res.StatusCode, msg)
	}
	var text strings.Builder
	for _, b := range out.Content {
		if b.Type == "text" {
			text.WriteString(b.Text)
		}
	}
	return strings.TrimSpace(text.String()), nil
}

// JSON asks for a JSON answer and decodes it into out. The model is told to
// reply with JSON only; a fenced block or leading text is tolerated.
func (c *Client) JSON(ctx context.Context, system, prompt string, maxTokens int, out any) error {
	text, err := c.Complete(ctx, system+"\n\nReply with a single JSON object only, no other text.", []Message{{Role: "user", Content: prompt}}, maxTokens)
	if err != nil {
		return err
	}
	start, end := strings.Index(text, "{"), strings.LastIndex(text, "}")
	if start < 0 || end < start {
		return fmt.Errorf("ai: no JSON in reply")
	}
	return json.Unmarshal([]byte(text[start:end+1]), out)
}
