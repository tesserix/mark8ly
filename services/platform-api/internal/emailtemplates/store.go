// Package emailtemplates is platform-api's half of the runtime email
// template registry, serving mark8ly#720 Task 5. It reads and writes the
// SAME email_templates table (migration 0013) that internal/notification's
// Loader already reads on the real send path, and mirrors marketplace-api's
// internal/emailtemplates package (services/marketplace-api/internal/
// emailtemplates) field for field — the console's platform admin surface
// treats both services' endpoints identically, so the two packages must
// agree on shape even though they live in different services with
// different production loaders behind them.
//
// Where the two tables diverge, this package follows platform-api's ACTUAL
// schema, not marketplace-api's:
//   - migration 0013 has no updated_at trigger and no per-column
//     constraint beyond `status`; both tables otherwise agree, column for
//     column (key, subject, html_body, text_body, variables jsonb, status,
//     version, updated_at, updated_by).
//   - There is no cross-DB grant here to preserve compatibility with
//     (mark8ly has never had an admin database user for platform-api's
//     database — that was always marketplace-api's cross-DB path, which
//     mark8ly#720 is replacing, not extending). Nothing in this package
//     depends on one.
package emailtemplates

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"gorm.io/gorm"
)

// Status values the table's CHECK constraint allows (migration 0013).
const (
	StatusPublished = "published"
	StatusDraft     = "draft"
)

// ErrNoDB is returned by every Store method when the store was built with
// a nil *gorm.DB. Returned rather than panicking so a mis-wired process
// surfaces a 500 with a logged cause instead of a crashed request.
var ErrNoDB = errors.New("emailtemplates: no database connection")

// Variable is one entry of a template's declared variable schema.
type Variable struct {
	Name     string `json:"name"`
	Type     string `json:"type"`
	Required bool   `json:"required"`
}

// Row is a stored template.
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

// UpsertInput is one authored save. UpdatedBy and Capability come from the
// signed platform-admin request (platformauth.CtxOperatorID / CtxCapability),
// never from the body.
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

// decodeVariables never returns nil: a nil slice marshals to null, and a
// console reading `variables.map(...)` would crash on exactly the
// templates that declare none.
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
// transaction — see the package doc comment and migration 0018 for why
// the revision insert is not optional decoration.
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
