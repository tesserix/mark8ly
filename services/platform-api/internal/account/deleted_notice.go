package account

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/mark8ly/platform-api/internal/outbox"
	"github.com/mark8ly/platform-api/internal/rocketchat"
)

// TenantDeletedNoticeOutboxKind is the operator Rocket.Chat notice for a
// merchant self-serve delete — the counterpart of the onboarding notice.
const TenantDeletedNoticeOutboxKind = "rocketchat.tenant_deleted"

// tenantCounter is the subset of marketplaceapi.VendorClient that reports
// what a tenant's purge would delete, per table, without deleting anything.
type tenantCounter interface {
	CountTenant(ctx context.Context, tenantID string, storeIDs []string) (map[string]int64, error)
}

// deletedNotice is captured BEFORE the teardown: the purge hard-deletes every
// tenant row, audit_logs included, so nothing can answer "what had they
// built?" afterwards. Counts is nil when the pre-delete count failed — the
// delete must never wait on, or fail because of, an operator notice.
type deletedNotice struct {
	TenantID   string           `json:"tenant_id"`
	Name       string           `json:"name"`
	OwnerEmail string           `json:"owner_email"`
	StoreSlugs []string         `json:"store_slugs"`
	SignedUpAt time.Time        `json:"signed_up_at"`
	DeletedAt  time.Time        `json:"deleted_at"`
	Counts     map[string]int64 `json:"counts,omitempty"`
}

// NewTenantDeletedNoticeHandler posts the notice under a per-tenant message
// ID, so an outbox replay is verified rather than posted twice.
func NewTenantDeletedNoticeHandler(post rocketchat.Poster) outbox.Handler {
	return func(ctx context.Context, payload json.RawMessage) error {
		var n deletedNotice
		if json.Unmarshal(payload, &n) != nil || n.TenantID == "" {
			return errors.New("rocketchat: invalid tenant deleted event")
		}
		return post(ctx, "mark8ly-deleted-"+n.TenantID, n.message())
	}
}

func (n deletedNotice) message() string {
	var b strings.Builder
	b.WriteString("**mark8ly account deleted**\n")
	days := ""
	if !n.SignedUpAt.IsZero() {
		days = fmt.Sprintf("%d", int(n.DeletedAt.Sub(n.SignedUpAt).Hours()/24))
	}
	for _, f := range [][2]string{
		{"Business", n.Name}, {"Email", n.OwnerEmail}, {"Store slug", strings.Join(n.StoreSlugs, ", ")},
		{"Signed up (UTC)", utc(n.SignedUpAt)}, {"Deleted (UTC)", utc(n.DeletedAt)}, {"Days active", days},
		{"Tenant ID", n.TenantID},
	} {
		if f[1] != "" {
			b.WriteString("\n**" + f[0] + ":** " + rocketchat.Text(f[1]))
		}
	}
	if n.Counts == nil {
		b.WriteString("\n\n_Activity counts unavailable (pre-delete count failed)._")
		return b.String()
	}
	yes := func(table string) string {
		if n.Counts[table] > 0 {
			return "yes"
		}
		return "no"
	}
	var total int64
	for _, c := range n.Counts {
		total += c
	}
	fmt.Fprintf(&b, "\n\n**Products:** %d · **Orders:** %d · **Customers:** %d", n.Counts["products"], n.Counts["orders"], n.Counts["customer_profiles"])
	fmt.Fprintf(&b, "\n**Payments connected:** %s · **Custom domain:** %s · **Branding saved:** %s", yes("payment_gateway_configs"), yes("custom_domains"), yes("store_branding"))
	fmt.Fprintf(&b, "\n**Total rows:** %d", total)
	return b.String()
}

func utc(t time.Time) string {
	if t.IsZero() {
		return ""
	}
	return t.UTC().Format(time.RFC3339)
}
