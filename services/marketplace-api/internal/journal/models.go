// Package journal provides the domain model and repository behind
// mark8ly.com's marketing-site email capture (#153). The first caller is
// the Journal "coming soon" page's "Notify me when the first piece goes
// up" field; the table's `source` column lets other capture points reuse
// it later without a new migration.
//
// Subscribers are a platform-level record with no tenant_id — see
// migrations/000124_journal_subscribers.up.sql for why this table sits
// outside the tenant model that every other table in this service uses.
package journal

import (
	"strings"
	"time"

	"github.com/google/uuid"
)

// SourceJournal identifies signups captured from the /blog "coming soon"
// page. A named constant rather than a literal at the call site, so a
// second capture point can define its own SourceXxx and reuse this same
// table with no migration required.
const SourceJournal = "journal"

// MaxEmailLength matches the journal_subscribers.email column width
// (migration 000124) and RFC 5321's practical mailbox length ceiling.
const MaxEmailLength = 254

// Subscriber maps to the journal_subscribers table.
type Subscriber struct {
	ID        uuid.UUID `gorm:"column:id;type:uuid;primaryKey;default:gen_random_uuid()" json:"id"`
	Email     string    `gorm:"column:email;type:varchar(254);not null" json:"email"`
	Source    string    `gorm:"column:source;type:varchar(40);not null;default:journal" json:"source"`
	CreatedAt time.Time `gorm:"column:created_at;not null;default:now()" json:"created_at"`
	// UnsubscribeToken is the bearer credential mailed to the
	// subscriber (out of band — this service never sends the
	// confirmation email itself) that authorises deleting this row via
	// POST /journal/unsubscribe. Never serialized to JSON: it must
	// never come back in any API response after creation.
	UnsubscribeToken string `gorm:"column:unsubscribe_token;type:char(64);not null" json:"-"`
}

// TableName pins the GORM table name explicitly rather than relying on
// pluralization inference.
func (Subscriber) TableName() string { return "journal_subscribers" }

// SubscribeInput is the JSON binding struct for the public subscribe
// endpoint. The `email` binding tag rejects malformed or oversized input
// at the HTTP boundary before it ever reaches the repository; NormalizeEmail
// is applied afterward so storage matches the unique index's assumptions.
type SubscribeInput struct {
	Email string `json:"email" binding:"required,email,max=254"`
}

// NormalizeEmail trims surrounding whitespace and lowercases the address
// so that "Foo@Bar.com" and "foo@bar.com" collide on the unique email
// index instead of creating two rows for the same mailbox.
func NormalizeEmail(email string) string {
	return strings.ToLower(strings.TrimSpace(email))
}
