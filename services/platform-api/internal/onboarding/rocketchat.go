package onboarding

import (
	"context"
	"encoding/json"
	"errors"
	"net/url"
	"strings"
	"time"

	"github.com/mark8ly/platform-api/internal/outbox"
	"github.com/mark8ly/platform-api/internal/rocketchat"
)

const RocketChatOutboxKind = "rocketchat.onboarding_completed"

// RocketChatConfig is server-only; credentials must come from OpenBao.
type RocketChatConfig = rocketchat.Config

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

// NewRocketChatHandler delivers a stable message ID, verifying previous delivery on replay.
func NewRocketChatHandler(cfg RocketChatConfig) (outbox.Handler, error) {
	post, err := rocketchat.NewPoster(cfg)
	if err != nil {
		return nil, err
	}
	return func(ctx context.Context, payload json.RawMessage) error {
		var p signupNotification
		if json.Unmarshal(payload, &p) != nil || p.TenantID == "" || p.StoreID == "" {
			return errors.New("rocketchat: invalid signup event")
		}
		return post(ctx, "mark8ly-onboarding-"+p.TenantID, p.message())
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
			b.WriteString("\n**" + f[0] + ":** " + rocketchat.Text(f[1]))
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
