package emailtemplates

import (
	"context"
	"strings"
	"testing"

	"github.com/mark8ly/marketplace-api/pkg/testdb"
)

// loader_integration_test.go — full DB round-trip against a real
// Postgres. Skipped when TEST_DATABASE_URL is unset.

func TestLoader_DB_SeedAndRender(t *testing.T) {
	db := testdb.NewTx(t)
	loader := NewLoader(db)
	loader.Register("welcome", EmbeddedFallback{
		Subject:  "Welcome {{.Name}}",
		HTMLBody: "<p>Hi {{.Name}}</p>",
		TextBody: "Hi {{.Name}}",
	})

	if err := loader.SeedFromEmbedded(context.Background()); err != nil {
		t.Fatalf("seed: %v", err)
	}

	got, err := loader.Render(context.Background(), "welcome", struct{ Name string }{Name: "Acme"})
	if err != nil {
		t.Fatalf("render: %v", err)
	}
	if got.Subject != "Welcome Acme" {
		t.Errorf("Subject = %q", got.Subject)
	}
}

// TestLoader_DB_MatchesEmbedded — byte-identity proof: DB-loaded
// rendering must equal embedded rendering for the same vars. This
// catches drift between the seed migration and the embedded fallback.
func TestLoader_DB_MatchesEmbedded(t *testing.T) {
	db := testdb.NewTx(t)
	dbLoader := NewLoader(db)
	embLoader := NewLoader(nil)

	fb := EmbeddedFallback{
		Subject:  "Refund for {{.OrderNumber}}{{if .IsFullRefund}} (full){{end}}",
		HTMLBody: "<p>Refund of {{.RefundAmount}} {{.CurrencyCode}}</p>",
		TextBody: "Refund of {{.RefundAmount}} {{.CurrencyCode}}",
	}
	dbLoader.Register("refund_email", fb)
	embLoader.Register("refund_email", fb)

	if err := dbLoader.SeedFromEmbedded(context.Background()); err != nil {
		t.Fatalf("seed: %v", err)
	}

	type vars struct {
		OrderNumber  string
		RefundAmount string
		CurrencyCode string
		IsFullRefund bool
	}
	cases := []vars{
		{OrderNumber: "ORD-1", RefundAmount: "10.00", CurrencyCode: "AUD", IsFullRefund: true},
		{OrderNumber: "ORD-2", RefundAmount: "3.50", CurrencyCode: "USD", IsFullRefund: false},
	}
	for _, v := range cases {
		dbR, err := dbLoader.Render(context.Background(), "refund_email", v)
		if err != nil {
			t.Fatalf("db render: %v", err)
		}
		embR, err := embLoader.Render(context.Background(), "refund_email", v)
		if err != nil {
			t.Fatalf("emb render: %v", err)
		}
		if dbR.Subject != embR.Subject {
			t.Errorf("subject drift: db=%q emb=%q", dbR.Subject, embR.Subject)
		}
		if dbR.HTMLBody != embR.HTMLBody {
			t.Errorf("html drift: db=%q emb=%q", dbR.HTMLBody, embR.HTMLBody)
		}
		if dbR.TextBody != embR.TextBody {
			t.Errorf("text drift: db=%q emb=%q", dbR.TextBody, embR.TextBody)
		}
	}
}

// TestLoader_DB_SeedIsIdempotent — re-running the seed must not
// clobber operator edits.
func TestLoader_DB_SeedIsIdempotent(t *testing.T) {
	db := testdb.NewTx(t)
	loader := NewLoader(db)
	loader.Register("k", EmbeddedFallback{Subject: "embedded", HTMLBody: "<p>e</p>", TextBody: "e"})

	if err := loader.SeedFromEmbedded(context.Background()); err != nil {
		t.Fatalf("seed: %v", err)
	}
	// Operator edits the row.
	if err := db.Exec(`UPDATE email_templates SET subject='OPERATOR-EDIT' WHERE key='k'`).Error; err != nil {
		t.Fatalf("operator edit: %v", err)
	}
	loader.InvalidateAll()

	// Re-seed.
	if err := loader.SeedFromEmbedded(context.Background()); err != nil {
		t.Fatalf("seed second: %v", err)
	}
	var subj string
	if err := db.Raw(`SELECT subject FROM email_templates WHERE key='k'`).Scan(&subj).Error; err != nil {
		t.Fatalf("post-seed select: %v", err)
	}
	if subj != "OPERATOR-EDIT" {
		t.Errorf("subject = %q, want OPERATOR-EDIT — re-seed clobbered an edit", subj)
	}
}

// TestLoader_DB_DraftStatusIgnored — draft rows must not affect
// live sends.
func TestLoader_DB_DraftStatusIgnored(t *testing.T) {
	db := testdb.NewTx(t)
	loader := NewLoader(db)
	loader.Register("welcome", EmbeddedFallback{
		Subject:  "EMBEDDED",
		HTMLBody: "<p>EMBEDDED</p>",
		TextBody: "EMBEDDED",
	})
	if err := db.Exec(`
		INSERT INTO email_templates (key, subject, html_body, text_body, status)
		VALUES ('welcome', 'DRAFT-SUBJECT', '<p>DRAFT</p>', 'DRAFT', 'draft')
	`).Error; err != nil {
		t.Fatalf("insert draft: %v", err)
	}
	got, err := loader.Render(context.Background(), "welcome", nil)
	if err != nil {
		t.Fatalf("render: %v", err)
	}
	if strings.Contains(got.Subject, "DRAFT") {
		t.Errorf("draft leaked: %q", got.Subject)
	}
	if got.Subject != "EMBEDDED" {
		t.Errorf("Subject = %q, want EMBEDDED (draft should fall back to embedded)", got.Subject)
	}
}

// TestLoader_DB_PublishedTakesPrecedence — happy path: published row
// IS used.
func TestLoader_DB_PublishedTakesPrecedence(t *testing.T) {
	db := testdb.NewTx(t)
	loader := NewLoader(db)
	loader.Register("welcome", EmbeddedFallback{
		Subject:  "EMBEDDED",
		HTMLBody: "<p>EMBEDDED</p>",
		TextBody: "EMBEDDED",
	})
	if err := db.Exec(`
		INSERT INTO email_templates (key, subject, html_body, text_body, status)
		VALUES ('welcome', 'PUBLISHED-{{.X}}', '<p>PUB</p>', 'PUB', 'published')
	`).Error; err != nil {
		t.Fatalf("insert published: %v", err)
	}
	got, err := loader.Render(context.Background(), "welcome", struct{ X string }{X: "ok"})
	if err != nil {
		t.Fatalf("render: %v", err)
	}
	if got.Subject != "PUBLISHED-ok" {
		t.Errorf("Subject = %q, want PUBLISHED-ok (DB row not used)", got.Subject)
	}
}
