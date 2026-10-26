//go:build integration

package onboarding

import (
	"encoding/json"
	"github.com/google/uuid"
	"strings"
	"testing"
	"time"

	"github.com/mark8ly/platform-api/internal/outbox"
	"github.com/mark8ly/platform-api/internal/store"
	"github.com/mark8ly/platform-api/internal/tenant"
	"github.com/mark8ly/platform-api/pkg/testdb"
)

func TestIntegration_RocketChatSignupCommitsOnceWithoutSecrets(t *testing.T) {
	db := testdb.NewDB(t, "outbox_events", "onboarding_sessions", "stores", "tenants")
	repo := NewRepository(db)
	svc := NewService(Config{DB: db, Repo: repo, TenantRepo: tenant.NewRepository(db), StoreRepo: store.NewRepository(db), RocketChatEnabled: true, AdminURLTemplate: "https://{slug}-admin.mark8ly.com", StorefrontURLTemplate: "https://{slug}.mark8ly.com"})
	now := time.Now()
	draftCanary := uuid.NewString()
	draft, err := json.Marshal(map[string]string{"password": draftCanary})
	if err != nil {
		t.Fatal(err)
	}
	session := &Session{Email: "founder@example.invalid", Status: StatusInProgress, EmailVerifiedAt: &now, Draft: draft, Classification: "external"}
	if err := repo.Create(t.Context(), session); err != nil {
		t.Fatal(err)
	}
	req := CompleteRequest{SessionID: session.ID, OwnerUserID: "test-owner", OwnerEmail: session.Email, FirstName: "Jane", LastName: "Doe", BusinessName: "Example Store", Slug: "example", CountryCode: "AU", CurrencyCode: "AUD", Timezone: "Australia/Sydney", Password: uuid.NewString(), TaxID: "never-post-tax-id"}
	result, err := svc.Complete(t.Context(), req)
	if err != nil {
		t.Fatal(err)
	}
	var events []outbox.Event
	if err := db.Where("kind = ?", RocketChatOutboxKind).Find(&events).Error; err != nil {
		t.Fatal(err)
	}
	if len(events) != 1 {
		t.Fatalf("got %d chat events, want 1", len(events))
	}
	var p signupNotification
	if err := json.Unmarshal(events[0].Payload, &p); err != nil {
		t.Fatal(err)
	}
	if p.TenantID != result.TenantID || p.OwnerName != "Jane Doe" || p.OwnerEmail != session.Email || p.StoreID == "" || p.SignedUpAt.IsZero() {
		t.Fatalf("incorrect signup payload: %+v", p)
	}
	for _, value := range []string{req.Password, req.TaxID, draftCanary} {
		if strings.Contains(string(events[0].Payload), value) {
			t.Fatal("event contains a forbidden request field")
		}
	}
	if _, err := svc.Complete(t.Context(), req); err == nil {
		t.Fatal("repeated completion accepted")
	}
	var count int64
	db.Model(&outbox.Event{}).Where("kind = ?", RocketChatOutboxKind).Count(&count)
	if count != 1 {
		t.Fatalf("repeat created %d events", count)
	}
	second := &Session{Email: "another@example.invalid", Status: StatusInProgress, EmailVerifiedAt: &now, Draft: json.RawMessage(`{}`), Classification: "external"}
	if err := repo.Create(t.Context(), second); err != nil {
		t.Fatal(err)
	}
	req.SessionID = second.ID
	req.OwnerEmail = second.Email
	req.OwnerUserID = "another-owner"
	if _, err := svc.Complete(t.Context(), req); err == nil {
		t.Fatal("duplicate slug accepted")
	}
	db.Model(&outbox.Event{}).Where("kind = ?", RocketChatOutboxKind).Count(&count)
	if count != 1 {
		t.Fatal("rolled-back signup left a chat event")
	}
}
