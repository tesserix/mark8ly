package public_test

import (
	"context"
	"errors"
	"testing"

	"github.com/google/uuid"

	"github.com/mark8ly/marketplace-api/internal/handlers/public"
	"github.com/mark8ly/marketplace-api/internal/stores"
)

type stubSlugLookup struct {
	store *stores.Store
	err   error
	slugs []string
}

func (s *stubSlugLookup) Get(_ context.Context, slug string) (*stores.Store, error) {
	s.slugs = append(s.slugs, slug)
	return s.store, s.err
}

func TestByTenantSlug_ResolvesTheTenantThatOwnsTheStore(t *testing.T) {
	tenantID := uuid.New()
	lookup := &stubSlugLookup{store: &stores.Store{TenantID: tenantID.String(), Slug: "bondi-surf"}}
	r := public.NewStoreSlugTenantResolver(lookup)

	got, err := r.ByTenantSlug(context.Background(), "bondi-surf")
	if err != nil {
		t.Fatalf("ByTenantSlug: %v", err)
	}
	if got != tenantID {
		t.Errorf("tenant = %s, want %s", got, tenantID)
	}
	if len(lookup.slugs) != 1 || lookup.slugs[0] != "bondi-surf" {
		t.Errorf("looked up %v", lookup.slugs)
	}
}

// The handler renders ErrTenantNotFound as a 404 with the same body it uses
// for "this tenant has no SSO config". Distinguishing the two would let an
// anonymous caller enumerate which stores exist.
func TestByTenantSlug_UnknownSlugIsNotFound(t *testing.T) {
	r := public.NewStoreSlugTenantResolver(&stubSlugLookup{err: stores.ErrNotFound})

	_, err := r.ByTenantSlug(context.Background(), "no-such-store")
	if !errors.Is(err, public.ErrTenantNotFound) {
		t.Fatalf("err = %v, want ErrTenantNotFound", err)
	}
}

func TestByTenantSlug_EmptySlugIsNotFoundWithoutALookup(t *testing.T) {
	lookup := &stubSlugLookup{}
	r := public.NewStoreSlugTenantResolver(lookup)

	if _, err := r.ByTenantSlug(context.Background(), ""); !errors.Is(err, public.ErrTenantNotFound) {
		t.Fatalf("err = %v, want ErrTenantNotFound", err)
	}
	if len(lookup.slugs) != 0 {
		t.Error("an empty slug still hit the store lookup")
	}
}

// A lookup that fails for any other reason must NOT collapse into "no such
// tenant": the handler answers 404 for that, and a merchant whose SSO is down
// because the store service is unreachable would be told their tenant does not
// exist.
func TestByTenantSlug_ALookupFailureIsNotAMissingTenant(t *testing.T) {
	r := public.NewStoreSlugTenantResolver(&stubSlugLookup{err: errors.New("platform-api down")})

	_, err := r.ByTenantSlug(context.Background(), "bondi-surf")
	if err == nil {
		t.Fatal("a lookup failure was reported as success")
	}
	if errors.Is(err, public.ErrTenantNotFound) {
		t.Fatal("a lookup failure was reported as a missing tenant — that renders as 404")
	}
}

// Same reasoning for a corrupt projection row: a 404 would send an operator
// looking for a store that is sitting right there.
func TestByTenantSlug_AnUnparseableTenantIDIsAnError(t *testing.T) {
	r := public.NewStoreSlugTenantResolver(&stubSlugLookup{
		store: &stores.Store{TenantID: "not-a-uuid", Slug: "bondi-surf"},
	})

	_, err := r.ByTenantSlug(context.Background(), "bondi-surf")
	if err == nil || errors.Is(err, public.ErrTenantNotFound) {
		t.Fatalf("err = %v, want a real error", err)
	}
}
