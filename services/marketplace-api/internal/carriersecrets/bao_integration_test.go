//go:build integration

// Round-trips a real secret through a real OpenBao instance: write, read,
// write again (a new KV v2 version), read the new value back, destroy the
// secret, and confirm a subsequent read maps to ErrSecretNotFound.
//
// Tasks 1-7 proved this whole path against an httptest fake (see
// bao_test.go). Fakes prove the logic; they do not prove marketplace-api
// speaks KV v2 correctly to a real server. Earlier in this phase a bug got
// through precisely because the fake was more forgiving than reality: a
// `case json.Number` in intFrom (internal/bao/kv.go) was dropped as
// "unused" during a refactor, when the real OpenBao SDK decodes every
// numeric JSON field as json.Number — silently disabling check-and-set.
// This test exists to catch that class of gap again.
//
// Gated on TEST_OPENBAO_ADDR / TEST_OPENBAO_TOKEN so it never runs by
// accident in the default `go test ./...`. Missing either skips loudly
// (t.Skip naming the variable) rather than passing silently — a test that
// appears to run but does nothing is worse than no test at all.
package carriersecrets_test

import (
	"bytes"
	"context"
	"errors"
	"os"
	"testing"

	"github.com/google/uuid"

	"github.com/mark8ly/marketplace-api/internal/bao"
	"github.com/mark8ly/marketplace-api/internal/carriersecrets"
)

// TestBaoClient_RealRoundTrip exercises BaoClient against a real OpenBao
// server reachable at TEST_OPENBAO_ADDR, authenticated with
// TEST_OPENBAO_TOKEN (a client token already exchanged via Kubernetes
// login — this test never performs that exchange itself).
//
// The scratch path is built through carriersecrets.BaoPath so the test
// exercises the real path builder rather than a hand-written string, and it
// lives under a synthetic tenant ID ("_test-<uuid>") so it stays inside the
// marketplace-api policy's granted prefix:
//
//	kv/data/mark8ly/marketplace-api/tenants/*
//	kv/metadata/mark8ly/marketplace-api/tenants/*
//
// A path outside that prefix is refused with 403 regardless of whether the
// client code is correct.
func TestBaoClient_RealRoundTrip(t *testing.T) {
	addr := os.Getenv("TEST_OPENBAO_ADDR")
	if addr == "" {
		t.Skip("skipping: TEST_OPENBAO_ADDR is not set (need a real OpenBao instance to round-trip against)")
	}
	token := os.Getenv("TEST_OPENBAO_TOKEN")
	if token == "" {
		t.Skip("skipping: TEST_OPENBAO_TOKEN is not set (need a client token scoped to the marketplace-api KV policy)")
	}

	client, err := bao.New(bao.Config{
		Address: addr,
		Mount:   "kv",
		Token:   token,
	})
	if err != nil {
		t.Fatalf("bao.New: %v", err)
	}
	bc := carriersecrets.NewBaoClient(client)

	scope := carriersecrets.Scope{
		// A unique per-run tenant ID keeps concurrent runs (and reruns) from
		// colliding on the same path; the "_test-" prefix marks the row as
		// throwaway scratch, not a real tenant.
		TenantID: "_test-" + uuid.NewString(),
		Domain:   "payment",
		Provider: "_probe",
		Field:    "api_key",
	}
	path := carriersecrets.BaoPath(scope)

	ctx := t.Context()
	t.Cleanup(func() {
		// t.Context() is already canceled by the time Cleanup funcs run, so
		// this final best-effort delete uses a fresh background context
		// rather than the (canceled) test context.
		if err := bc.DeleteSecret(context.Background(), path); err != nil {
			t.Logf("cleanup: DeleteSecret(%s): %v", path, err)
		}
	})

	first := []byte("real-openbao-round-trip-v1")
	if err := bc.CreateOrAddVersion(ctx, path, first); err != nil {
		t.Fatalf("CreateOrAddVersion (first write): %v", err)
	}

	got, err := bc.AccessLatest(ctx, path)
	if err != nil {
		t.Fatalf("AccessLatest (after first write): %v", err)
	}
	if !bytes.Equal(got, first) {
		t.Fatalf("AccessLatest = %q, want %q", got, first)
	}

	second := []byte("real-openbao-round-trip-v2")
	if err := bc.CreateOrAddVersion(ctx, path, second); err != nil {
		t.Fatalf("CreateOrAddVersion (second write): %v", err)
	}

	got, err = bc.AccessLatest(ctx, path)
	if err != nil {
		t.Fatalf("AccessLatest (after second write): %v", err)
	}
	if !bytes.Equal(got, second) {
		t.Fatalf("AccessLatest after second write = %q, want the new version %q (stale read suggests KV v2 versioning is broken)", got, second)
	}

	if err := bc.DeleteSecret(ctx, path); err != nil {
		t.Fatalf("DeleteSecret: %v", err)
	}

	if _, err := bc.AccessLatest(ctx, path); !errors.Is(err, carriersecrets.ErrSecretNotFound) {
		t.Fatalf("AccessLatest after DeleteSecret = %v, want ErrSecretNotFound", err)
	}
}
