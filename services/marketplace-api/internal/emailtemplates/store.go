package emailtemplates

// store.go — CRUD over the email_templates table for the platform admin
// contract surface.
//
// The Loader (loader.go) reads this same table on the SEND path and
// deliberately narrows to `status = 'published'`. This store does not: an
// authoring surface has to be able to see and save a draft, and a draft is
// precisely the row the send path must ignore.
//
// The UPSERT mirrors tesserix-home's cross-DB one
// (apps/web/lib/db/email-templates.ts) field for field, including the
// version bump and the updated_by stamp, so moving the console onto the
// federated surface does not change what lands in the row.

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"gorm.io/gorm"
)

// Status values the table's CHECK constraint allows (migration 000085).
const (
	StatusPublished = "published"
	StatusDraft     = "draft"
)

// ErrNoDB is returned by every Store method when the store was built with
// a nil *gorm.DB. Returned rather than panicking so a mis-wired process
// surfaces a 500 with a logged cause instead of a crashed request.
var ErrNoDB = errors.New("emailtemplates: no database connection")

// Variable is one entry of a template's declared variable schema — what
// the authoring UI offers an operator as an available placeholder. Stored
// as the `variables` jsonb column.
type Variable struct {
	Name     string `json:"name"`
	Type     string `json:"type"`
	Required bool   `json:"required"`
}

// Row is a stored template. Distinct from dbTemplate (loader.go), which is
// the send path's projection and carries only what a render needs.
type Row struct {
	Key       string
	Subject   string
	HTMLBody  string
	TextBody  string
	Variables []Variable
	Status    string
	Version   int
	UpdatedAt time.Time
	UpdatedBy string
}

// UpsertInput is one authored save.
//
// UpdatedBy and Capability come from the signed platform-admin request
// (platformadmin.CtxOperatorID / CtxCapability), never from the body: an
// attribution a caller can choose is not an attribution.
type UpsertInput struct {
	Key        string
	Subject    string
	HTMLBody   string
	TextBody   string
	Variables  []Variable
	Status     string
	UpdatedBy  string
	Capability string
}

// Store reads and writes email_templates.
type Store struct {
	db *gorm.DB
}

// NewStore binds a Store to a GORM connection. db may be nil; every method
// then returns ErrNoDB.
func NewStore(db *gorm.DB) *Store { return &Store{db: db} }

// scanRow mirrors the column list of the SELECTs below. Variables is
// []byte because the column is jsonb — decoded once, in toRow, so a
// malformed value degrades to an empty schema rather than failing a read
// the operator needs in order to fix it.
type scanRow struct {
	Key       string
	Subject   string
	HTMLBody  string
	TextBody  string
	Variables []byte
	Status    string
	Version   int
	UpdatedAt time.Time
	UpdatedBy *string
}

const selectColumns = `key, subject, html_body, text_body, variables, status, version, updated_at, updated_by`

func toRow(r scanRow) Row {
	row := Row{
		Key:       r.Key,
		Subject:   r.Subject,
		HTMLBody:  r.HTMLBody,
		TextBody:  r.TextBody,
		Variables: decodeVariables(r.Variables),
		Status:    r.Status,
		Version:   r.Version,
		UpdatedAt: r.UpdatedAt,
	}
	if r.UpdatedBy != nil {
		row.UpdatedBy = *r.UpdatedBy
	}
	return row
}

// decodeVariables never returns nil: a nil slice marshals to null, which
// defeats a caller's `?? []` exactly when a template declares no
// variables — the common case for the keys that have never been authored.
func decodeVariables(raw []byte) []Variable {
	out := make([]Variable, 0)
	if len(raw) == 0 {
		return out
	}
	var decoded []Variable
	if err := json.Unmarshal(raw, &decoded); err != nil {
		return out
	}
	for _, v := range decoded {
		if strings.TrimSpace(v.Name) == "" {
			continue
		}
		if v.Type == "" {
			v.Type = "string"
		}
		out = append(out, v)
	}
	return out
}

// List returns every stored template, published and draft alike, ordered
// by key.
func (s *Store) List(ctx context.Context) ([]Row, error) {
	if s == nil || s.db == nil {
		return nil, ErrNoDB
	}
	var scanned []scanRow
	err := s.db.WithContext(ctx).
		Raw(`SELECT ` + selectColumns + ` FROM email_templates ORDER BY key ASC`).
		Scan(&scanned).Error
	if err != nil {
		return nil, fmt.Errorf("emailtemplates: list: %w", err)
	}
	rows := make([]Row, 0, len(scanned))
	for _, r := range scanned {
		rows = append(rows, toRow(r))
	}
	return rows, nil
}

// Get returns one stored template. The bool reports existence; a missing
// row is not an error, because "no row" is a legitimate state for a
// registered key (the embedded default is what sends).
func (s *Store) Get(ctx context.Context, key string) (Row, bool, error) {
	if s == nil || s.db == nil {
		return Row{}, false, ErrNoDB
	}
	var scanned scanRow
	err := s.db.WithContext(ctx).
		Raw(`SELECT `+selectColumns+` FROM email_templates WHERE key = ?`, key).
		Scan(&scanned).Error
	if err != nil {
		return Row{}, false, fmt.Errorf("emailtemplates: get %q: %w", key, err)
	}
	if scanned.Key == "" {
		return Row{}, false, nil
	}
	return toRow(scanned), true, nil
}

// Upsert saves an authored template and records the change, both on ONE
// transaction.
//
// The revision insert is not decoration. audit_logs — where every other
// write on the platform admin surface records itself — is tenant
// partitioned (`tenant_id NOT NULL`, migration 000035; 000101 relaxed
// store_id and deliberately did not relax this), and an email template is
// estate-wide: it belongs to no tenant, so no audit_logs row can be
// written for it. Rather than let a write that changes what EVERY
// merchant receives run with no operator attribution at all, the record
// lives here, in the same transaction as the change it accounts for — so
// a failed record rolls the change back instead of leaving the two out of
// step.
//
// It stores structural facts only (key, resulting version and status, the
// operator, the capability presented) and never the authored copy: the
// live row already holds the copy, and a trail that duplicates the data it
// exists to account for is a second copy of the problem. It is therefore
// NOT a version history and offers no rollback — tesserix-home#588 rules
// that out explicitly, and nothing here provides it.
func (s *Store) Upsert(ctx context.Context, in UpsertInput) (Row, error) {
	if s == nil || s.db == nil {
		return Row{}, ErrNoDB
	}
	status := in.Status
	if status != StatusDraft {
		status = StatusPublished
	}
	vars := in.Variables
	if vars == nil {
		vars = []Variable{}
	}
	encoded, err := json.Marshal(vars)
	if err != nil {
		return Row{}, fmt.Errorf("emailtemplates: encode variables: %w", err)
	}

	var saved Row
	txErr := s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var scanned scanRow
		if err := tx.Raw(`
			INSERT INTO email_templates
				(key, subject, html_body, text_body, variables, status, version, updated_at, updated_by)
			VALUES (?, ?, ?, ?, ?::jsonb, ?, 1, now(), ?)
			ON CONFLICT (key) DO UPDATE SET
				subject    = EXCLUDED.subject,
				html_body  = EXCLUDED.html_body,
				text_body  = EXCLUDED.text_body,
				variables  = EXCLUDED.variables,
				status     = EXCLUDED.status,
				version    = email_templates.version + 1,
				updated_at = now(),
				updated_by = EXCLUDED.updated_by
			RETURNING `+selectColumns,
			in.Key, in.Subject, in.HTMLBody, in.TextBody, string(encoded), status, in.UpdatedBy,
		).Scan(&scanned).Error; err != nil {
			return fmt.Errorf("emailtemplates: upsert %q: %w", in.Key, err)
		}
		if scanned.Key == "" {
			return fmt.Errorf("emailtemplates: upsert %q returned no row", in.Key)
		}
		saved = toRow(scanned)

		var capability *string
		if c := strings.TrimSpace(in.Capability); c != "" {
			capability = &c
		}
		if err := tx.Exec(`
			INSERT INTO email_template_revisions (key, version, status, changed_by, capability)
			VALUES (?, ?, ?, ?, ?)
		`, saved.Key, saved.Version, saved.Status, in.UpdatedBy, capability).Error; err != nil {
			return fmt.Errorf("emailtemplates: record revision %q: %w", in.Key, err)
		}
		return nil
	})
	if txErr != nil {
		return Row{}, txErr
	}
	return saved, nil
}
