package customer

import (
	"context"
	"errors"
	"io"
	"log/slog"
	"testing"

	"github.com/google/uuid"
)

// fakeRepo embeds Repository so only the methods a test actually reaches
// need implementing; anything else panics loudly rather than silently
// returning a zero value.
type fakeRepo struct {
	Repository
	existing  *CustomerProfile
	lookupErr error
	upserted  *CustomerProfile
}

func (f *fakeRepo) GetProfileByEmail(_ context.Context, _ uuid.UUID, _ string) (*CustomerProfile, error) {
	if f.lookupErr != nil {
		return nil, f.lookupErr
	}
	if f.existing == nil {
		return nil, ErrNotFound
	}
	return f.existing, nil
}

func (f *fakeRepo) UpsertProfile(_ context.Context, p *CustomerProfile) (*CustomerProfile, error) {
	f.upserted = p
	return p, nil
}

func testLogger() *slog.Logger {
	return slog.New(slog.NewTextHandler(io.Discard, nil))
}

func TestJoinStoreCreatesMembership(t *testing.T) {
	repo := &fakeRepo{}
	svc := NewService(nil, repo, testLogger())

	got, err := svc.JoinStore(context.Background(), JoinStoreInput{
		Email: "  Shopper@Example.COM ",
	}, nil)
	if err != nil {
		t.Fatalf("JoinStore: %v", err)
	}
	if repo.upserted == nil {
		t.Fatal("expected an upsert — JoinStore is the path that creates a membership")
	}
	if got.Email != "shopper@example.com" {
		t.Fatalf("email not normalised: %q", got.Email)
	}
	if got.Status != StatusActive {
		t.Fatalf("status: %q", got.Status)
	}
}

func TestJoinStoreRefusesBlockedMembershipWithoutWriting(t *testing.T) {
	reason := "chargeback fraud"
	repo := &fakeRepo{existing: &CustomerProfile{
		Email:       "blocked@example.com",
		Status:      StatusBlocked,
		BlockReason: &reason,
	}}
	svc := NewService(nil, repo, testLogger())

	_, err := svc.JoinStore(context.Background(), JoinStoreInput{
		Email: "blocked@example.com",
	}, nil)
	if !errors.Is(err, ErrBlocked) {
		t.Fatalf("want ErrBlocked, got %v", err)
	}
	// The point of the check: a blocked customer must not be able to
	// reset their own state by re-joining.
	if repo.upserted != nil {
		t.Fatal("blocked membership was written to — joining must not resurrect a blocked customer")
	}
}

func TestLookupProfileNeverWrites(t *testing.T) {
	repo := &fakeRepo{existing: &CustomerProfile{Email: "member@example.com"}}
	svc := NewService(nil, repo, testLogger())

	if _, err := svc.LookupProfile(context.Background(), uuid.Nil, "member@example.com"); err != nil {
		t.Fatalf("LookupProfile: %v", err)
	}
	if repo.upserted != nil {
		t.Fatal("LookupProfile wrote a row — the session path must be read-only")
	}
}

func TestLookupProfileEmptyEmailIsNotFound(t *testing.T) {
	svc := NewService(nil, &fakeRepo{}, testLogger())
	if _, err := svc.LookupProfile(context.Background(), uuid.Nil, "   "); !errors.Is(err, ErrNotFound) {
		t.Fatalf("want ErrNotFound, got %v", err)
	}
}
