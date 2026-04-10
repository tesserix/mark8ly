package push

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
)

const expoPushURL = "https://exp.host/--/api/v2/push/send"

// Sender sends push notifications via the Expo Push API.
type Sender struct {
	httpClient *http.Client
}

// NewSender creates a Sender with the given HTTP client.
func NewSender(client *http.Client) *Sender {
	return &Sender{httpClient: client}
}

type pushMessage struct {
	To    string            `json:"to"`
	Title string            `json:"title"`
	Body  string            `json:"body"`
	Data  map[string]string `json:"data,omitempty"`
}

type pushResponse struct {
	Data []pushResult `json:"data"`
}

type pushResult struct {
	Status  string      `json:"status"`
	Details interface{} `json:"details,omitempty"`
}

// SendResult contains the outcome of a push send operation.
type SendResult struct {
	StaleTokens []string // tokens that returned DeviceNotRegistered
}

// Send sends a push notification to the given Expo push tokens.
// Returns any stale tokens that should be removed.
func (s *Sender) Send(tokens []string, title, body, deepLink string) (*SendResult, error) {
	if len(tokens) == 0 {
		return &SendResult{}, nil
	}

	result := &SendResult{}

	// Batch in groups of 100 (Expo limit).
	for i := 0; i < len(tokens); i += 100 {
		end := i + 100
		if end > len(tokens) {
			end = len(tokens)
		}
		batch := tokens[i:end]

		messages := make([]pushMessage, len(batch))
		for j, token := range batch {
			msg := pushMessage{
				To:    token,
				Title: title,
				Body:  body,
			}
			if deepLink != "" {
				msg.Data = map[string]string{"deep_link": deepLink}
			}
			messages[j] = msg
		}

		payload, err := json.Marshal(messages)
		if err != nil {
			return nil, fmt.Errorf("marshal push messages: %w", err)
		}

		req, err := http.NewRequest("POST", expoPushURL, bytes.NewReader(payload))
		if err != nil {
			return nil, fmt.Errorf("create push request: %w", err)
		}
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("Accept", "application/json")

		resp, err := s.httpClient.Do(req)
		if err != nil {
			return nil, fmt.Errorf("send push request: %w", err)
		}

		if resp.StatusCode != http.StatusOK {
			resp.Body.Close()
			return nil, fmt.Errorf("expo push API returned %d", resp.StatusCode)
		}

		var pushResp pushResponse
		if err := json.NewDecoder(resp.Body).Decode(&pushResp); err != nil {
			resp.Body.Close()
			return nil, fmt.Errorf("decode push response: %w", err)
		}
		resp.Body.Close()

		// Identify stale tokens.
		for j, r := range pushResp.Data {
			if r.Status == "error" {
				details, ok := r.Details.(map[string]interface{})
				if ok && details["error"] == "DeviceNotRegistered" {
					result.StaleTokens = append(result.StaleTokens, batch[j])
				}
			}
		}
	}

	return result, nil
}
