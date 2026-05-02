package notification

import (
	"context"
	"strings"
	"testing"

	"github.com/mark8ly/platform-api/pkg/testdb"
)

// db_loader_integration_test.go — exercises the full DB round-trip
// against a real Postgres. Skipped when TEST_DATABASE_URL is unset
// (matches the rest of the repo's integration-test conventions).
//
// The byte-identity assertion in TestLoader_DB_MatchesEmbedded is the
// safety property that proves the seed migration produces output
// indistinguishable from the embedded path. If this test fails after
// editing a seed row, the DB drifted from the embedded canon and a
// real send would produce different output than today.

func TestLoader_DB_SeedAndRender(t *testing.T) {
	db := testdb.NewTx(t)
	loader := NewLoader(db)

	// Seed the DB with embedded templates.
	if err := loader.SeedFromEmbedded(context.Background()); err != nil {
		t.Fatalf("seed: %v", err)
	}

	// Render via DB path.
	msg, err := loader.Render(context.Background(), "welcome", "u@e.com", "n@m.com", "tenant-1", WelcomeVars{
		BusinessName:  "Acme Co",
		OwnerName:     "Pat",
		AdminURL:      "https://acme-admin.mark8ly.com",
		StorefrontURL: "https://acme.mark8ly.com",
		SupportEmail:  "help@mark8ly.com",
	})
	if err != nil {
		t.Fatalf("render: %v", err)
	}
	if !strings.Contains(msg.Subject, "Acme Co") {
		t.Errorf("Subject = %q, want to contain Acme Co (subject-template not applied?)", msg.Subject)
	}
	if !strings.Contains(msg.HTMLBody, "https://acme-admin.mark8ly.com") {
		t.Errorf("HTMLBody missing AdminURL")
	}
	if msg.TenantID != "tenant-1" {
		t.Errorf("TenantID = %q, want tenant-1 (loader did not forward)", msg.TenantID)
	}
}

// TestLoader_DB_MatchesEmbedded is the byte-identity safety net. For
// every seeded template + a fixed Vars payload, the DB-rendered output
// must equal the embedded-rendered output. Drift here means an
// operator (or a bad seed) silently changed what mark8ly sends.
func TestLoader_DB_MatchesEmbedded(t *testing.T) {
	db := testdb.NewTx(t)
	dbLoader := NewLoader(db)
	if err := dbLoader.SeedFromEmbedded(context.Background()); err != nil {
		t.Fatalf("seed: %v", err)
	}
	embeddedLoader := NewLoader(nil) // forces fallback path

	cases := []struct {
		key  string
		vars any
	}{
		{
			key: "welcome",
			vars: WelcomeVars{
				BusinessName:  "Acme Co",
				OwnerName:     "Pat",
				AdminURL:      "https://a.example",
				StorefrontURL: "https://s.example",
				SupportEmail:  "help@example.com",
			},
		},
		{
			key: "email_verification",
			vars: EmailVerificationVars{
				BusinessName: "Beta",
				VerifyURL:    "https://verify.example/?t=tok",
				ExpiresIn:    "24 hours",
				SupportEmail: "help@example.com",
			},
		},
		{
			key: "invitation",
			vars: InvitationVars{
				TenantName:   "Gamma",
				Role:         "admin",
				Inviter:      "Pat",
				AcceptURL:    "https://accept.example/?t=inv",
				ExpiresIn:    "72 hours",
				SupportEmail: "help@example.com",
			},
		},
		{
			key: "password_reset",
			vars: PasswordResetVars{
				ResetURL:     "https://reset.example/?oob=oob",
				ExpiresIn:    "1 hour",
				SupportEmail: "help@example.com",
			},
		},
	}

	for _, tc := range cases {
		t.Run(tc.key, func(t *testing.T) {
			gotDB, err := dbLoader.Render(context.Background(), tc.key, "u@e.com", "n@m.com", "", tc.vars)
			if err != nil {
				t.Fatalf("db render: %v", err)
			}
			gotEmb, err := embeddedLoader.Render(context.Background(), tc.key, "u@e.com", "n@m.com", "", tc.vars)
			if err != nil {
				t.Fatalf("embedded render: %v", err)
			}
			if gotDB.Subject != gotEmb.Subject {
				t.Errorf("Subject drifted:\n DB: %q\n EMB: %q", gotDB.Subject, gotEmb.Subject)
			}
			if gotDB.HTMLBody != gotEmb.HTMLBody {
				t.Errorf("HTMLBody drifted (lengths %d vs %d)", len(gotDB.HTMLBody), len(gotEmb.HTMLBody))
			}
			if gotDB.TextBody != gotEmb.TextBody {
				t.Errorf("TextBody drifted:\n DB: %q\n EMB: %q", gotDB.TextBody, gotEmb.TextBody)
			}
		})
	}
}

// TestLoader_DB_SeedIsIdempotent verifies SeedFromEmbedded doesn't
// overwrite an operator-edited row. The seed runs on every boot, so
// it MUST be safe to run repeatedly.
func TestLoader_DB_SeedIsIdempotent(t *testing.T) {
	db := testdb.NewTx(t)
	loader := NewLoader(db)
	if err := loader.SeedFromEmbedded(context.Background()); err != nil {
		t.Fatalf("seed first: %v", err)
	}
	// Simulate an operator edit.
	if err := db.Exec(
		`UPDATE email_templates SET subject = 'OPERATOR-EDITED', updated_by = 'ops@mark8ly.com' WHERE key = 'welcome'`,
	).Error; err != nil {
		t.Fatalf("operator edit: %v", err)
	}
	loader.InvalidateAll()

	// Re-seed. Operator edit must survive.
	if err := loader.SeedFromEmbedded(context.Background()); err != nil {
		t.Fatalf("seed second: %v", err)
	}

	var subj string
	if err := db.Raw(`SELECT subject FROM email_templates WHERE key = 'welcome'`).Scan(&subj).Error; err != nil {
		t.Fatalf("post-seed select: %v", err)
	}
	if subj != "OPERATOR-EDITED" {
		t.Errorf("subject = %q, want OPERATOR-EDITED — re-seed clobbered an operator edit", subj)
	}
}

// TestLoader_DB_DraftStatusIgnored verifies that a row with status='draft'
// is treated as missing — drafts must NEVER affect live sends. The loader
// falls back to embedded so the production flow is unaffected.
func TestLoader_DB_DraftStatusIgnored(t *testing.T) {
	db := testdb.NewTx(t)
	loader := NewLoader(db)

	// Insert a draft row with deliberately-different content.
	err := db.Exec(`
		INSERT INTO email_templates (key, subject, html_body, text_body, status)
		VALUES ('welcome', 'DRAFT-SUBJECT', '<p>DRAFT-HTML</p>', 'DRAFT-TEXT', 'draft')
	`).Error
	if err != nil {
		t.Fatalf("insert draft: %v", err)
	}

	msg, err := loader.Render(context.Background(), "welcome", "u@e.com", "n@m.com", "", WelcomeVars{
		BusinessName:  "Acme",
		AdminURL:      "x",
		StorefrontURL: "y",
		SupportEmail:  "z",
	})
	if err != nil {
		t.Fatalf("render: %v", err)
	}
	if strings.Contains(msg.Subject, "DRAFT") {
		t.Errorf("draft leaked into live render: subject=%q", msg.Subject)
	}
	if strings.Contains(msg.HTMLBody, "DRAFT-HTML") {
		t.Errorf("draft HTML leaked into live render")
	}
}

// TestLoader_DB_PublishedTakesPrecedence verifies the happy path: a
// published row IS used when present.
func TestLoader_DB_PublishedTakesPrecedence(t *testing.T) {
	db := testdb.NewTx(t)
	loader := NewLoader(db)

	err := db.Exec(`
		INSERT INTO email_templates (key, subject, html_body, text_body, status)
		VALUES ('welcome', 'PUBLISHED-{{.BusinessName}}', '<p>PUBLISHED-{{.BusinessName}}</p>', 'PUBLISHED', 'published')
	`).Error
	if err != nil {
		t.Fatalf("insert published: %v", err)
	}

	msg, err := loader.Render(context.Background(), "welcome", "u@e.com", "n@m.com", "", WelcomeVars{
		BusinessName:  "Acme",
		AdminURL:      "x",
		StorefrontURL: "y",
		SupportEmail:  "z",
	})
	if err != nil {
		t.Fatalf("render: %v", err)
	}
	if msg.Subject != "PUBLISHED-Acme" {
		t.Errorf("Subject = %q, want PUBLISHED-Acme (DB row not used)", msg.Subject)
	}
	if !strings.Contains(msg.HTMLBody, "PUBLISHED-Acme") {
		t.Errorf("HTMLBody = %q, want to contain PUBLISHED-Acme", msg.HTMLBody)
	}
}
