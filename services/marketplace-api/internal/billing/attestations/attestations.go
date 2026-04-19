// Package attestations persists the Apple Guideline 4.2.6 acknowledgment
// captured at white-label mobile-app add-on purchase (spec §13.2 / §14.2).
//
// Schema invariants enforced by migration 000075:
//   - Append-only via BEFORE UPDATE trigger raising "append-only ..." error.
//   - Role-level REVOKE DELETE from marketplace_user (the app role) — the
//     app cannot delete these rows; only the DDL-owning migrator role can.
//
// Both guards are required independently: a DROP TRIGGER by itself would not
// bypass the DELETE revoke, and vice-versa.
package attestations

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"
)

// Type enumerates the legally-meaningful attestation kinds captured at
// purchase time. Stored as a CHECK-constrained VARCHAR in the DB.
type Type string

// TypeApple426 records the user's acknowledgment of Apple Guideline 4.2.6
// (multi-tenant white-label app; may face first-review rejection).
const TypeApple426 Type = "apple_4_2_6"

// Input is the payload for Record. All fields except IPAddress / UserAgent
// are required; StripeInvoiceID is UNIQUE and will error on duplicate.
type Input struct {
	TenantID         uuid.UUID
	StoreID          uuid.UUID
	SubscriptionID   uuid.UUID
	AttestationType  Type
	AttestedByUserID uuid.UUID
	AttestationText  string
	IPAddress        string // optional; empty string stores NULL
	UserAgent        string // optional
	StripeInvoiceID  string // required; UNIQUE key into Stripe invoice
}

// Row is the persisted shape returned from FindByStripeInvoice. Renamed
// from "Record" to avoid colliding with the Record function below.
type Row struct {
	ID               uuid.UUID `gorm:"column:id;type:uuid;primaryKey;default:gen_random_uuid()"`
	TenantID         uuid.UUID `gorm:"column:tenant_id;type:uuid;not null"`
	StoreID          uuid.UUID `gorm:"column:store_id;type:uuid;not null"`
	SubscriptionID   uuid.UUID `gorm:"column:subscription_id;type:uuid;not null"`
	AttestationType  Type      `gorm:"column:attestation_type;type:varchar(40);not null"`
	AttestedAt       time.Time `gorm:"column:attested_at;not null;default:now()"`
	AttestedByUserID uuid.UUID `gorm:"column:attested_by_user_id;type:uuid;not null"`
	AttestationText  string    `gorm:"column:attestation_text;not null"`
	IPAddress        *string   `gorm:"column:ip_address;type:inet"`
	UserAgent        *string   `gorm:"column:user_agent"`
	StripeInvoiceID  string    `gorm:"column:stripe_invoice_id;not null"`
}

// TableName maps Row to the app_contract_attestations table.
func (Row) TableName() string { return "app_contract_attestations" }

// ErrDuplicateInvoice is returned when Record is called with a StripeInvoiceID
// that already exists in the table (UNIQUE violation).
var ErrDuplicateInvoice = errors.New("attestations: stripe_invoice_id already recorded")

// Record inserts a new attestation row. The StripeInvoiceID UNIQUE constraint
// makes Record idempotent-safe for webhook retries: a replay returns
// ErrDuplicateInvoice, which callers can handle without failing the request.
//
// Returns the new row ID on success.
func Record(ctx context.Context, db *gorm.DB, in Input) (uuid.UUID, error) {
	if in.AttestationType == "" {
		return uuid.Nil, fmt.Errorf("attestations: Record: attestation_type is required")
	}
	if in.StripeInvoiceID == "" {
		return uuid.Nil, fmt.Errorf("attestations: Record: stripe_invoice_id is required")
	}
	if in.AttestationText == "" {
		return uuid.Nil, fmt.Errorf("attestations: Record: attestation_text is required")
	}

	r := Row{
		ID:               uuid.New(),
		TenantID:         in.TenantID,
		StoreID:          in.StoreID,
		SubscriptionID:   in.SubscriptionID,
		AttestationType:  in.AttestationType,
		AttestedAt:       time.Now().UTC(),
		AttestedByUserID: in.AttestedByUserID,
		AttestationText:  in.AttestationText,
		StripeInvoiceID:  in.StripeInvoiceID,
	}
	if in.IPAddress != "" {
		v := in.IPAddress
		r.IPAddress = &v
	}
	if in.UserAgent != "" {
		v := in.UserAgent
		r.UserAgent = &v
	}

	if err := db.WithContext(ctx).Create(&r).Error; err != nil {
		// Postgres unique_violation SQLSTATE is 23505. Rather than string-match
		// the driver message, we wrap and let the caller inspect via errors.Is.
		if isUniqueViolation(err) {
			return uuid.Nil, fmt.Errorf("%w: %s", ErrDuplicateInvoice, in.StripeInvoiceID)
		}
		return uuid.Nil, fmt.Errorf("attestations: insert: %w", err)
	}
	return r.ID, nil
}

// FindByStripeInvoice returns the attestation row tied to a given Stripe
// invoice. Used by the purchase webhook to correlate the invoice.paid event
// back to the acknowledgement that authorized the invoice.
func FindByStripeInvoice(ctx context.Context, db *gorm.DB, invoiceID string) (Row, error) {
	if invoiceID == "" {
		return Row{}, fmt.Errorf("attestations: FindByStripeInvoice: invoiceID is required")
	}
	var r Row
	err := db.WithContext(ctx).
		Where("stripe_invoice_id = ?", invoiceID).
		First(&r).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return Row{}, err
	}
	if err != nil {
		return Row{}, fmt.Errorf("attestations: FindByStripeInvoice(%s): %w", invoiceID, err)
	}
	return r, nil
}

// isUniqueViolation detects PG SQLSTATE 23505 across the common GORM/PGX
// error shapes without importing the driver directly (keeps this package
// driver-agnostic and easier to unit-test).
func isUniqueViolation(err error) bool {
	if err == nil {
		return false
	}
	msg := err.Error()
	// pgconn.PgError.Code==23505 ; lib/pq emits "pq: duplicate key value"
	return containsAny(msg, "SQLSTATE 23505", "duplicate key value", "unique constraint")
}

func containsAny(s string, subs ...string) bool {
	for _, sub := range subs {
		if indexOf(s, sub) >= 0 {
			return true
		}
	}
	return false
}

// indexOf is a tiny substring search — avoids importing strings just for this.
func indexOf(s, sub string) int {
	if sub == "" {
		return 0
	}
	for i := 0; i+len(sub) <= len(s); i++ {
		if s[i:i+len(sub)] == sub {
			return i
		}
	}
	return -1
}
