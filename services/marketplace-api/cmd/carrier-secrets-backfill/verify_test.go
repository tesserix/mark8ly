package main

import (
	"context"
	"errors"
	"testing"

	"github.com/mark8ly/marketplace-api/internal/carriersecrets"
)

// verifyStore records every Get and fails the ones whose reference is in
// failFor. Put/Destroy panic: Verify must never write, and a panic makes a
// regression impossible to miss.
type verifyStore struct {
	got     []string
	failFor map[string]bool
}

func (s *verifyStore) Get(_ context.Context, ref string) (string, error) {
	s.got = append(s.got, ref)
	if s.failFor[ref] {
		return "", errors.New("backend unavailable")
	}
	return "resolved-value", nil
}

func (s *verifyStore) Put(context.Context, carriersecrets.Scope, string) (string, error) {
	panic("Verify must never write: Put called")
}

func (s *verifyStore) Destroy(context.Context, string) error {
	panic("Verify must never write: Destroy called")
}

func TestVerify_ResolvesEveryReferenceAndCountsBySchema(t *testing.T) {
	scope := carriersecrets.Scope{TenantID: "t1", Domain: "payment", Provider: "razorpay", Field: "api_key"}
	baoRef := carriersecrets.FormatBaoReference(scope)
	gsmRef := carriersecrets.GSMRefPrefix + "projects/p/secrets/s"

	rows := []Row{
		{Table: "payment_gateway_configs", Column: "api_key_encrypted", ID: "1", Ref: baoRef, Scope: scope},
		{Table: "payment_gateway_configs", Column: "secret_key_encrypted", ID: "1", Ref: baoRef, Scope: scope},
		{Table: "shipping_carrier_configs", Column: "api_key_encrypted", ID: "2", Ref: gsmRef, Scope: scope},
		{Table: "custom_domains", Column: "cf_api_token_encrypted", ID: "3", Ref: "", Scope: scope},
	}

	store := &verifyStore{}
	b := &Backfiller{Rows: &recordingRowStore{rows: rows}, Store: store}

	res, err := b.Verify(context.Background())
	if err != nil {
		t.Fatalf("Verify() error = %v", err)
	}

	if res.Examined != 4 {
		t.Errorf("Examined = %d, want 4", res.Examined)
	}
	// The empty reference is not a credential and must not be dialled.
	if res.Resolved != 3 {
		t.Errorf("Resolved = %d, want 3", res.Resolved)
	}
	if res.Failed != 0 {
		t.Errorf("Failed = %d, want 0", res.Failed)
	}
	if len(store.got) != 3 {
		t.Errorf("Store.Get called %d times, want 3 (empty ref must not be read)", len(store.got))
	}

	// The scheme census is the actual decision input for #621: it says
	// whether any credential is still served by GCP Secret Manager.
	if res.ByScheme["bao"] != 2 {
		t.Errorf("ByScheme[bao] = %d, want 2", res.ByScheme["bao"])
	}
	if res.ByScheme["gsm"] != 1 {
		t.Errorf("ByScheme[gsm] = %d, want 1", res.ByScheme["gsm"])
	}
	if res.ByScheme["empty"] != 1 {
		t.Errorf("ByScheme[empty] = %d, want 1", res.ByScheme["empty"])
	}
}

func TestVerify_CountsUnresolvableReferences(t *testing.T) {
	scope := carriersecrets.Scope{TenantID: "t1", Domain: "payment", Provider: "razorpay", Field: "api_key"}
	badRef := carriersecrets.FormatBaoReference(scope)

	store := &verifyStore{failFor: map[string]bool{badRef: true}}
	b := &Backfiller{
		Rows:  &recordingRowStore{rows: []Row{{Table: "payment_gateway_configs", Column: "api_key_encrypted", ID: "1", Ref: badRef, Scope: scope}}},
		Store: store,
	}

	res, err := b.Verify(context.Background())
	if err != nil {
		t.Fatalf("Verify() error = %v", err)
	}
	if res.Failed != 1 {
		t.Errorf("Failed = %d, want 1", res.Failed)
	}
	if res.Resolved != 0 {
		t.Errorf("Resolved = %d, want 0", res.Resolved)
	}
}
