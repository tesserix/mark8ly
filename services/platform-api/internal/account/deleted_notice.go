package account

import (
	"context"
	"encoding/json"
	"errors"
	"strconv"
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
	// Reason and ReasonDetail are the merchant's optional exit answer.
	Reason       string `json:"reason,omitempty"`
	ReasonDetail string `json:"reason_detail,omitempty"`
}

// exitReasonLabels are the one-tap answers the admin offers on the delete
// step. The codes are the wire contract with apps/admin.
var exitReasonLabels = map[string]string{
	"no_orders":      "Not getting orders",
	"too_much_setup": "Too much setup",
	"price":          "Price",
	"other_platform": "Using another platform",
	"just_testing":   "Just testing",
	"other":          "Other",
}

// ExitReason is the merchant's optional answer to "why are you leaving?".
type ExitReason struct {
	Code   string
	Detail string
}

// NewExitReason keeps only a known code and at most 500 runes of free text,
// so an unexpected answer is dropped rather than failing the delete.
func NewExitReason(code, detail string) ExitReason {
	if _, ok := exitReasonLabels[code]; !ok {
		code = ""
	}
	d := []rune(strings.TrimSpace(detail))
	if len(d) > 500 {
		d = d[:500]
	}
	return ExitReason{Code: code, Detail: string(d)}
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
		days = strconv.Itoa(int(n.DeletedAt.Sub(n.SignedUpAt).Hours() / 24))
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
	if n.Reason != "" || n.ReasonDetail != "" {
		reason := exitReasonLabels[n.Reason]
		if reason == "" {
			reason = "Not given"
		}
		b.WriteString("\n\n**Reason:** " + rocketchat.Text(reason))
		if n.ReasonDetail != "" {
			b.WriteString("\n**They said:** " + rocketchat.Text(n.ReasonDetail))
		}
	} else {
		b.WriteString("\n\n**Reason:** not given")
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
	b.WriteString("\n\n**Products:** " + strconv.FormatInt(n.Counts["products"], 10) +
		" · **Orders:** " + strconv.FormatInt(n.Counts["orders"], 10) +
		" · **Customers:** " + strconv.FormatInt(n.Counts["customer_profiles"], 10))
	b.WriteString("\n**Payments connected:** " + yes("payment_gateway_configs") +
		" · **Custom domain:** " + yes("custom_domains") + " · **Branding saved:** " + yes("store_branding"))
	b.WriteString("\n**Total rows:** " + strconv.FormatInt(total, 10))
	return b.String()
}

func utc(t time.Time) string {
	if t.IsZero() {
		return ""
	}
	return t.UTC().Format(time.RFC3339)
}
