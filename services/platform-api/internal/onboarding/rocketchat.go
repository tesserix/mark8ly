package onboarding

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

	"github.com/mark8ly/platform-api/internal/outbox"
)

const RocketChatOutboxKind = "rocketchat.onboarding_completed"

// RocketChatConfig is server-only; credentials must come from OpenBao.
type RocketChatConfig struct {
	URL, UserID, Token, RoomID string
	Client                     *http.Client
}

type signupNotification struct {
	TenantID       string    `json:"tenant_id"`
	StoreID        string    `json:"store_id"`
	OwnerName      string    `json:"owner_name"`
	OwnerEmail     string    `json:"owner_email"`
	BusinessName   string    `json:"business_name"`
	Slug           string    `json:"slug"`
	Country        string    `json:"country"`
	Currency       string    `json:"currency"`
	Timezone       string    `json:"timezone"`
	SignedUpAt     time.Time `json:"signed_up_at"`
	Classification string    `json:"classification"`
	AdminURL       string    `json:"admin_url"`
	StorefrontURL  string    `json:"storefront_url"`
}

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

// NewRocketChatHandler delivers a stable message ID, verifying previous delivery on replay.
func NewRocketChatHandler(cfg RocketChatConfig) (outbox.Handler, error) {
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
	return func(ctx context.Context, payload json.RawMessage) error {
		ctx, cancel := context.WithTimeout(ctx, 10*time.Second)
		defer cancel()
		var p signupNotification
		if json.Unmarshal(payload, &p) != nil || p.TenantID == "" || p.StoreID == "" {
			return errors.New("rocketchat: invalid signup event")
		}
		msg := chatMessage{ID: "mark8ly-onboarding-" + p.TenantID, RoomID: cfg.RoomID, Text: p.message()}
		prior, status, err := request(ctx, http.MethodGet, "chat.getMessage?msgId="+url.QueryEscape(msg.ID), nil)
		if err != nil {
			return err
		}
		if status == http.StatusOK && prior.Success {
			if prior.Message.ID == msg.ID && prior.Message.RoomID == msg.RoomID && prior.Message.User.ID == cfg.UserID && prior.Message.Text == msg.Text {
				return nil
			}
			return errors.New("rocketchat: existing message does not match signup")
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

func (p signupNotification) message() string {
	var b strings.Builder
	b.WriteString("**New mark8ly onboarding completed**\n")
	for _, f := range [][2]string{
		{"Owner", p.OwnerName}, {"Email", p.OwnerEmail}, {"Business / store", p.BusinessName}, {"Store slug", p.Slug},
		{"Signed up (UTC)", p.SignedUpAt.UTC().Format(time.RFC3339)}, {"Country", p.Country}, {"Currency", p.Currency}, {"Timezone", p.Timezone},
		{"Classification", p.Classification}, {"Tenant ID", p.TenantID}, {"Store ID", p.StoreID},
	} {
		if f[1] != "" {
			b.WriteString("\n**" + f[0] + ":** " + chatText(f[1]))
		}
	}
	for _, link := range [][2]string{{"Admin", p.AdminURL}, {"Storefront", p.StorefrontURL}} {
		u, err := url.Parse(link[1])
		if err == nil && u.Scheme == "https" && u.User == nil && (u.Hostname() == "mark8ly.com" || strings.HasSuffix(u.Hostname(), ".mark8ly.com")) && !strings.ContainsAny(link[1], "()<>\r\n") {
			b.WriteString("\n\n[" + link[0] + "](" + u.String() + ")")
		}
	}
	return b.String()
}

func chatText(value string) string {
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
			safe.WriteRune('\u200b')
		}
	}
	return strings.NewReplacer("\\", "\\\\", "*", "\\*", "_", "\\_", "`", "\\`", "[", "\\[", "]", "\\]", "<", "‹", ">", "›").Replace(safe.String())
}
