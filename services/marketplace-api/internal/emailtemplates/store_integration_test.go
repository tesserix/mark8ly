//go:build integration

package emailtemplates

import (
	"testing"

	"github.com/mark8ly/marketplace-api/pkg/testdb"
)

// store_integration_test.go — full round-trip against a real Postgres.
// Skipped when the test database is unset (testdb.NewTx).
//
// The SQL in store.go is the part unit tests cannot reach: the ?::jsonb
// cast, the RETURNING clause the version bump is read from, and the
// transaction that ties the revision row to the change it records.

func seedableInput(key string) UpsertInput {
	return UpsertInput{
		Key:        key,
		Subject:    "Order {{.OrderNumber}}",
		HTMLBody:   "<p>{{.OrderNumber}}</p>",
		TextBody:   "{{.OrderNumber}}",
		Variables:  []Variable{{Name: "OrderNumber", Type: "string", Required: true}},
		Status:     StatusPublished,
		UpdatedBy:  "op_1",
		Capability: "platform.email_templates.write",
	}
}

func TestStore_DB_UpsertInsertsThenBumpsVersion(t *testing.T) {
	db := testdb.NewTx(t)
	s := NewStore(db)

	first, err := s.Upsert(t.Context(), seedableInput("orderdoc_invoice"))
	if err != nil {
		t.Fatalf("first upsert: %v", err)
	}
	if first.Version != 1 {
		t.Fatalf("first insert version = %d, want 1", first.Version)
	}
	if len(first.Variables) != 1 || first.Variables[0].Name != "OrderNumber" {
		t.Fatalf("variables did not round-trip through jsonb: %+v", first.Variables)
	}

	in := seedableInput("orderdoc_invoice")
	in.Subject = "Updated {{.OrderNumber}}"
	in.UpdatedBy = "op_2"
	second, err := s.Upsert(t.Context(), in)
	if err != nil {
		t.Fatalf("second upsert: %v", err)
	}
	if second.Version != 2 {
		t.Fatalf("second upsert version = %d, want 2 — the bump the cross-DB UPSERT had", second.Version)
	}
	if second.Subject != "Updated {{.OrderNumber}}" || second.UpdatedBy != "op_2" {
		t.Fatalf("update did not overwrite subject/updated_by: %+v", second)
	}
}

// The revision row is what makes the write attributable at all: audit_logs
// is tenant partitioned and an email template key is estate-wide, so no
// audit_logs row can be written for this change.
func TestStore_DB_UpsertRecordsARevisionPerChange(t *testing.T) {
	db := testdb.NewTx(t)
	s := NewStore(db)

	if _, err := s.Upsert(t.Context(), seedableInput("giftcard_delivery")); err != nil {
		t.Fatalf("first upsert: %v", err)
	}
	in := seedableInput("giftcard_delivery")
	in.Status = StatusDraft
	in.UpdatedBy = "op_2"
	if _, err := s.Upsert(t.Context(), in); err != nil {
		t.Fatalf("second upsert: %v", err)
	}

	var rows []struct {
		Version    int
		Status     string
		ChangedBy  string
		Capability *string
	}
	err := db.Raw(`SELECT version, status, changed_by, capability
	               FROM email_template_revisions
	               WHERE key = ? ORDER BY version ASC`, "giftcard_delivery").Scan(&rows).Error
	if err != nil {
		t.Fatalf("read revisions: %v", err)
	}
	if len(rows) != 2 {
		t.Fatalf("got %d revisions, want one per change", len(rows))
	}
	if rows[0].Version != 1 || rows[0].ChangedBy != "op_1" || rows[0].Status != StatusPublished {
		t.Errorf("first revision = %+v", rows[0])
	}
	if rows[1].Version != 2 || rows[1].ChangedBy != "op_2" || rows[1].Status != StatusDraft {
		t.Errorf("second revision = %+v", rows[1])
	}
	if rows[0].Capability == nil || *rows[0].Capability != "platform.email_templates.write" {
		t.Errorf("capability not recorded: %+v", rows[0].Capability)
	}
}

// An empty capability must land as SQL NULL, not as an empty string:
// CapabilityValueChecked is false today, and "" would read as a
// capability that was asserted and happened to be blank.
func TestStore_DB_UpsertRecordsNullCapabilityWhenNonePresented(t *testing.T) {
	db := testdb.NewTx(t)
	in := seedableInput("orderdoc_credit_note")
	in.Capability = "  "
	if _, err := NewStore(db).Upsert(t.Context(), in); err != nil {
		t.Fatalf("upsert: %v", err)
	}

	var isNull bool
	err := db.Raw(`SELECT capability IS NULL FROM email_template_revisions WHERE key = ?`,
		"orderdoc_credit_note").Scan(&isNull).Error
	if err != nil {
		t.Fatalf("read revision: %v", err)
	}
	if !isNull {
		t.Fatal("capability stored as an empty string rather than NULL")
	}
}

// The send path narrows to status='published'; this store must not, or an
// operator could never see or resume a draft.
func TestStore_DB_ListAndGetIncludeDrafts(t *testing.T) {
	db := testdb.NewTx(t)
	s := NewStore(db)

	in := seedableInput("dunning_day_5")
	in.Status = StatusDraft
	if _, err := s.Upsert(t.Context(), in); err != nil {
		t.Fatalf("upsert: %v", err)
	}

	got, found, err := s.Get(t.Context(), "dunning_day_5")
	if err != nil || !found {
		t.Fatalf("Get() = (%+v, %v, %v), want the draft", got, found, err)
	}
	if got.Status != StatusDraft {
		t.Fatalf("status = %q, want draft", got.Status)
	}

	rows, err := s.List(t.Context())
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	var seen bool
	for _, r := range rows {
		if r.Key == "dunning_day_5" {
			seen = true
		}
	}
	if !seen {
		t.Fatal("List() omitted the draft row")
	}
}

// A missing row is not an error: "no row" is a legitimate state for a
// registered key, and returning an error would make the reader treat an
// unauthored template as a failure.
func TestStore_DB_GetReportsAMissingRowWithoutAnError(t *testing.T) {
	db := testdb.NewTx(t)
	got, found, err := NewStore(db).Get(t.Context(), "never_authored")
	if err != nil {
		t.Fatalf("Get() err = %v, want nil", err)
	}
	if found {
		t.Fatalf("Get() found = true for %+v", got)
	}
}

// The status CHECK constraint (migration 000085) rejects anything else, so
// a bad value must be normalised before it reaches SQL rather than
// surfacing as a constraint violation.
func TestStore_DB_UpsertNormalisesAnUnknownStatusToPublished(t *testing.T) {
	db := testdb.NewTx(t)
	in := seedableInput("orderdoc_receipt")
	in.Status = "LIVE"
	got, err := NewStore(db).Upsert(t.Context(), in)
	if err != nil {
		t.Fatalf("upsert: %v", err)
	}
	if got.Status != StatusPublished {
		t.Fatalf("status = %q, want published", got.Status)
	}
}
