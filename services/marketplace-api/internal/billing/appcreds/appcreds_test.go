package appcreds_test

import (
	"context"
	"errors"
	"testing"

	"github.com/google/uuid"

	"github.com/mark8ly/marketplace-api/internal/billing/appcreds"
)

// Tests use the FakeSM — no real Secret Manager calls — and nil audit
// emitter (Service.emit no-ops on nil). Audit path is exercised by the
// integration tests in audit/*_test.go.

func newTestService(fake *appcreds.FakeSM) *appcreds.Service {
	return appcreds.NewService(appcreds.Config{
		ProjectID: "test-proj", SM: fake, Emitter: nil,
	})
}

func TestService_StoreLoadRoundTrip(t *testing.T) {
	fake := appcreds.NewFakeSM()
	svc := newTestService(fake)

	tenantID := uuid.New()
	storeID := uuid.New()
	p8 := []byte("-----BEGIN PRIVATE KEY-----\nfake\n-----END PRIVATE KEY-----")

	if err := svc.Store(context.Background(), appcreds.StoreInput{
		TenantID: tenantID, StoreID: storeID,
		CredType: appcreds.CredTypeAppleP8, Payload: p8, Actor: "user:abc",
	}); err != nil {
		t.Fatalf("Store: %v", err)
	}

	got, err := svc.Load(context.Background(), appcreds.LoadInput{
		TenantID: tenantID, StoreID: storeID,
		CredType: appcreds.CredTypeAppleP8, Actor: "system:build",
	})
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if string(got) != string(p8) {
		t.Errorf("Load payload = %q, want %q", got, p8)
	}
}

func TestService_Load_CrossTenant_ReturnsErrNotFound(t *testing.T) {
	fake := appcreds.NewFakeSM()
	svc := newTestService(fake)

	tenantA, tenantB, storeID := uuid.New(), uuid.New(), uuid.New()
	if err := svc.Store(context.Background(), appcreds.StoreInput{
		TenantID: tenantA, StoreID: storeID,
		CredType: appcreds.CredTypeAppleP8, Payload: []byte("A"), Actor: "test",
	}); err != nil {
		t.Fatalf("Store: %v", err)
	}

	_, err := svc.Load(context.Background(), appcreds.LoadInput{
		TenantID: tenantB, StoreID: storeID,
		CredType: appcreds.CredTypeAppleP8, Actor: "user:x",
	})
	if !errors.Is(err, appcreds.ErrNotFound) {
		t.Errorf("cross-tenant Load err = %v, want wraps ErrNotFound", err)
	}
}

func TestService_Load_Unknown_ReturnsErrNotFound(t *testing.T) {
	fake := appcreds.NewFakeSM()
	svc := newTestService(fake)

	_, err := svc.Load(context.Background(), appcreds.LoadInput{
		TenantID: uuid.New(), StoreID: uuid.New(),
		CredType: appcreds.CredTypeAppleP8, Actor: "test",
	})
	if !errors.Is(err, appcreds.ErrNotFound) {
		t.Errorf("Load(empty fake) = %v, want wraps ErrNotFound", err)
	}
}

func TestService_Delete_Idempotent(t *testing.T) {
	fake := appcreds.NewFakeSM()
	svc := newTestService(fake)

	tenantID, storeID := uuid.New(), uuid.New()
	if err := svc.Store(context.Background(), appcreds.StoreInput{
		TenantID: tenantID, StoreID: storeID,
		CredType: appcreds.CredTypeAppleP8, Payload: []byte("x"), Actor: "test",
	}); err != nil {
		t.Fatalf("Store: %v", err)
	}

	if err := svc.Delete(context.Background(), appcreds.DeleteInput{
		TenantID: tenantID, StoreID: storeID,
		CredType: appcreds.CredTypeAppleP8, Actor: "system:cron",
	}); err != nil {
		t.Fatalf("first Delete: %v", err)
	}

	// Second Delete on a now-missing secret must not error.
	if err := svc.Delete(context.Background(), appcreds.DeleteInput{
		TenantID: tenantID, StoreID: storeID,
		CredType: appcreds.CredTypeAppleP8, Actor: "system:cron",
	}); err != nil {
		t.Errorf("second Delete = %v, want nil (idempotent)", err)
	}
}

func TestService_PurgeAll_RemovesAllFour(t *testing.T) {
	fake := appcreds.NewFakeSM()
	svc := newTestService(fake)

	tenantID, storeID := uuid.New(), uuid.New()
	for _, ct := range appcreds.AllCredTypes() {
		if err := svc.Store(context.Background(), appcreds.StoreInput{
			TenantID: tenantID, StoreID: storeID,
			CredType: ct, Payload: []byte("x"), Actor: "test",
		}); err != nil {
			t.Fatalf("Store(%s): %v", ct, err)
		}
	}

	if err := svc.PurgeAll(context.Background(), tenantID, storeID, "system:cron:day_90"); err != nil {
		t.Fatalf("PurgeAll: %v", err)
	}

	for _, ct := range appcreds.AllCredTypes() {
		_, err := svc.Load(context.Background(), appcreds.LoadInput{
			TenantID: tenantID, StoreID: storeID, CredType: ct, Actor: "test",
		})
		if !errors.Is(err, appcreds.ErrNotFound) {
			t.Errorf("after PurgeAll, Load(%s) = %v; want ErrNotFound", ct, err)
		}
	}
}

func TestService_ValidationErrors(t *testing.T) {
	fake := appcreds.NewFakeSM()
	svc := newTestService(fake)

	cases := []struct {
		name string
		in   appcreds.StoreInput
	}{
		{"missing tenantID", appcreds.StoreInput{
			CredType: appcreds.CredTypeAppleP8, Payload: []byte("x"),
		}},
		{"missing credType", appcreds.StoreInput{
			TenantID: uuid.New(), Payload: []byte("x"),
		}},
		{"unknown credType", appcreds.StoreInput{
			TenantID: uuid.New(),
			CredType: appcreds.CredType("not-a-real-type"),
			Payload:  []byte("x"),
		}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if err := svc.Store(context.Background(), tc.in); err == nil {
				t.Errorf("Store(%s) = nil, want error", tc.name)
			}
		})
	}
}

func TestFakeSM_CopiesPayload(t *testing.T) {
	fake := appcreds.NewFakeSM()
	ctx := context.Background()
	name := "projects/p/secrets/merchant_t_x"
	original := []byte("original")

	if err := fake.CreateOrAddVersion(ctx, name, original); err != nil {
		t.Fatalf("CreateOrAddVersion: %v", err)
	}
	// Mutate the input slice — stored copy must not change.
	original[0] = '!'

	got, err := fake.AccessLatest(ctx, name)
	if err != nil {
		t.Fatalf("AccessLatest: %v", err)
	}
	if string(got) != "original" {
		t.Errorf("FakeSM did not copy payload; got %q after caller mutation", got)
	}
}

func TestFakeSM_Has(t *testing.T) {
	fake := appcreds.NewFakeSM()
	ctx := context.Background()
	name := "projects/p/secrets/merchant_t_x"

	if fake.Has(name) {
		t.Error("Has(empty) = true, want false")
	}
	_ = fake.CreateOrAddVersion(ctx, name, []byte("x"))
	if !fake.Has(name) {
		t.Error("Has(after create) = false, want true")
	}
	_ = fake.Delete(ctx, name)
	if fake.Has(name) {
		t.Error("Has(after delete) = true, want false")
	}
}
