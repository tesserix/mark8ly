// Package rocketchat posts operator notices to a Rocket.Chat room with a
// stable message ID, so an outbox replay never posts the same notice twice.
package rocketchat

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"
	"unicode"
)

// Config is server-only; credentials must come from OpenBao.
type Config struct {
	URL, UserID, Token, RoomID string
	Client                     *http.Client
}

// Poster delivers text under msgID, verifying any previous delivery on replay.
type Poster func(ctx context.Context, msgID, text string) error

type chatMessage struct {
	ID     string `json:"_id"`
	RoomID string `json:"rid"`
	Text   string `json:"msg"`
	User   struct {
		ID string `json:"_id"`
	} `json:"u,omitempty"`
}

type chatResponse struct {
	Success bool        `json:"success"`
	Message chatMessage `json:"message"`
}

// NewPoster validates cfg and returns a Poster bound to its room.
func NewPoster(cfg Config) (Poster, error) {
	u, err := url.Parse(cfg.URL)
	if err != nil || u.Scheme != "https" || u.Host == "" || u.User != nil || u.RawQuery != "" || u.Fragment != "" || (u.Path != "" && u.Path != "/") || strings.TrimSpace(cfg.UserID) == "" || strings.TrimSpace(cfg.Token) == "" || strings.TrimSpace(cfg.RoomID) == "" {
		return nil, errors.New("rocketchat: HTTPS origin and complete bot configuration required")
	}
	client := http.Client{Timeout: 5 * time.Second}
	if cfg.Client != nil {
		client = *cfg.Client
		client.Timeout = 5 * time.Second
	}
	client.CheckRedirect = func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }
	request := func(ctx context.Context, method, path string, body []byte) (chatResponse, int, error) {
		var result chatResponse
		req, err := http.NewRequestWithContext(ctx, method, strings.TrimRight(cfg.URL, "/")+"/api/v1/"+path, bytes.NewReader(body))
		if err != nil {
			return result, 0, errors.New("rocketchat: invalid request")
		}
		req.Header.Set("X-User-Id", cfg.UserID)
		req.Header.Set("X-Auth-Token", cfg.Token)
		req.Header.Set("Content-Type", "application/json")
		res, err := client.Do(req)
		if err != nil {
			return result, 0, errors.New("rocketchat: transport failed")
		}
		defer func() {
			// Closing the response cannot change whether Rocket.Chat accepted the message.
			_ = res.Body.Close()
		}()
		if res.StatusCode != http.StatusOK {
			return result, res.StatusCode, nil
		}
		if err := json.NewDecoder(io.LimitReader(res.Body, 64<<10)).Decode(&result); err != nil {
			return result, res.StatusCode, errors.New("rocketchat: invalid response")
		}
		return result, res.StatusCode, nil
	}
	return func(ctx context.Context, msgID, text string) error {
		ctx, cancel := context.WithTimeout(ctx, 10*time.Second)
		defer cancel()
		msg := chatMessage{ID: msgID, RoomID: cfg.RoomID, Text: text}
		prior, status, err := request(ctx, http.MethodGet, "chat.getMessage?msgId="+url.QueryEscape(msg.ID), nil)
		if err != nil {
			return err
		}
		if status == http.StatusOK && prior.Success {
			if prior.Message.ID == msg.ID && prior.Message.RoomID == msg.RoomID && prior.Message.User.ID == cfg.UserID && prior.Message.Text == msg.Text {
				return nil
			}
			return errors.New("rocketchat: existing message does not match notice")
		}
		if status != http.StatusBadRequest && status != http.StatusNotFound {
			return fmt.Errorf("rocketchat: lookup rejected (HTTP %d)", status)
		}
		body, err := json.Marshal(struct {
			Message     map[string]string `json:"message"`
			PreviewURLs []string          `json:"previewUrls"`
		}{Message: map[string]string{"_id": msg.ID, "rid": msg.RoomID, "msg": msg.Text}, PreviewURLs: []string{}})
		if err != nil {
			return errors.New("rocketchat: encode message")
		}
		result, status, err := request(ctx, http.MethodPost, "chat.sendMessage", body)
		if err != nil {
			return err
		}
		if status != http.StatusOK || !result.Success || result.Message.ID != msg.ID || result.Message.RoomID != msg.RoomID {
			return fmt.Errorf("rocketchat: delivery not confirmed (HTTP %d)", status)
		}
		return nil
	}, nil
}

// Text makes untrusted merchant input safe to embed in a markdown message:
// no control characters, no formatting, no live @mentions, at most 320 runes.
func Text(value string) string {
	runes := []rune(strings.Map(func(r rune) rune {
		if unicode.IsControl(r) {
			return ' '
		}
		return r
	}, value))
	if len(runes) > 320 {
		runes = runes[:320]
	}
	var safe strings.Builder
	for i, r := range runes {
		safe.WriteRune(r)
		if r == '@' && (i == 0 || (!unicode.IsLetter(runes[i-1]) && !unicode.IsDigit(runes[i-1]) && !strings.ContainsRune("._+-", runes[i-1]))) {
			safe.WriteRune('​')
		}
	}
	return strings.NewReplacer("\\", "\\\\", "*", "\\*", "_", "\\_", "`", "\\`", "[", "\\[", "]", "\\]", "<", "‹", ">", "›").Replace(safe.String())
}
