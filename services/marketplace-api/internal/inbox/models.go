// Package inbox aggregates mark8ly's queues that are waiting on a human into
// one shape the platform console can render without per-product knowledge.
//
// Each queue is a Provider. The aggregator fans out across them, merges, and
// paginates; it holds no per-kind knowledge, so adding a queue is one file
// plus one registration.
package inbox

import (
	"time"
)

// Severity is derived from an item's due date at read time, never stored.
const (
	SeverityNormal   = "normal"
	SeverityWarning  = "warning"
	SeverityCritical = "critical"
)

// warningWindow is how far ahead of DueAt an item starts reading as warning.
const warningWindow = 24 * time.Hour

// Action is something an operator may invoke on an item.
//
// Actions are derived from the item's own STATE, not from the caller's
// capability: the console's capability vocabulary is not settled, and
// platformadmin/middleware.go's CapabilityValueChecked is false for that
// reason. #287 declined to invent capability names; so does this. When the
// vocabulary lands, filter here and flip that switch.
type Action struct {
	ID          string `json:"id"`
	Label       string `json:"label"`
	Destructive bool   `json:"destructive"`
}

// Item is one unit of work waiting on a human, in the shape every kind shares.
type Item struct {
	ID           string     `json:"id"`
	Kind         string     `json:"kind"`
	Title        string     `json:"title"`
	Subtitle     string     `json:"subtitle"`
	WaitingSince time.Time  `json:"waiting_since"`
	DueAt        *time.Time `json:"due_at,omitempty"`
	Severity     string     `json:"severity"`
	Href         string     `json:"href"`
	Actions      []Action   `json:"actions"`
}

// Filter narrows a listing. An empty Kind means every kind.
type Filter struct {
	Kind     string
	TenantID string
	Status   string
	Page     int
	Limit    int
}

// DeriveSeverity maps a due date to a severity at read time.
//
// An item at or past its due date is critical; one inside warningWindow is
// warning; everything else, including an item with no due date at all, is
// normal. Only sea_manual_review carries a due date today.
func DeriveSeverity(dueAt *time.Time, now time.Time) string {
	if dueAt == nil {
		return SeverityNormal
	}
	if !now.Before(*dueAt) {
		return SeverityCritical
	}
	if dueAt.Sub(now) <= warningWindow {
		return SeverityWarning
	}
	return SeverityNormal
}
