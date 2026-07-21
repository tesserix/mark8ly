package invitation

import (
	"context"
	"errors"
	"testing"
	"time"
)

// fakeRepo implements just enough of Repository for Accept.
type fakeRepo struct {
	Repository
	inv           *Invitation
	acceptedID    string
	acceptedUID   string
	markAcceptErr error
}

func (f *fakeRepo) GetByTokenHash(_ context.Context, _ string) (*Invitation, error) {
	return f.inv, nil
}

func (f *fakeRepo) MarkAccepted(_ context.Context, id, uid string) error {
	if f.markAcceptErr != nil {
		return f.markAcceptErr
	}
	f.acceptedID = id
	f.acceptedUID = uid
	return nil
}

type fakeClaimSetter struct {
	uid      string
	tenantID string
	err      error
	calls    int
}

func (f *fakeClaimSetter) EnsureTenantClaim(_ context.Context, uid, tenantID string) error {
	f.calls++
	f.uid = uid
	f.tenantID = tenantID
	return f.err
}

func pendingInvitation() *Invitation {
	return &Invitation{
		ID:        "inv-1",
		TenantID:  "tid-1",
		Email:     "staff@example.com",
		Role:      "staff",
		Status:    StatusPending,
		ExpiresAt: time.Now().Add(time.Hour),
	}
}

func acceptInput() AcceptInput {
	return AcceptInput{Token: "tok", UID: "uid-staff", VerifiedEmail: "staff@example.com"}
}

func TestAccept_StampsTenantClaim(t *testing.T) {
	claims := &fakeClaimSetter{}
	svc := NewService(Config{
		Repo:   &fakeRepo{inv: pendingInvitation()},
		Claims: claims,
	})

	res, err := svc.Accept(context.Background(), acceptInput())
	if err != nil {
		t.Fatalf("Accept: %v", err)
	}
	if res.TenantID != "tid-1" {
		t.Errorf("TenantID = %q, want tid-1", res.TenantID)
	}
	if claims.calls != 1 || claims.uid != "uid-staff" || claims.tenantID != "tid-1" {
		t.Errorf("claim setter called %d times with (%q, %q), want 1 call with (uid-staff, tid-1)",
			claims.calls, claims.uid, claims.tenantID)
	}
}

func TestAccept_ClaimFailureDoesNotFailAccept(t *testing.T) {
	repo := &fakeRepo{inv: pendingInvitation()}
	claims := &fakeClaimSetter{err: errors.New("identity toolkit 503")}
	svc := NewService(Config{Repo: repo, Claims: claims})

	if _, err := svc.Accept(context.Background(), acceptInput()); err != nil {
		t.Fatalf("Accept must succeed despite claim failure, got: %v", err)
	}
	if repo.acceptedID != "inv-1" {
		t.Error("invitation was not marked accepted")
	}
}

func TestAccept_NilClaimSetterSkipsClaimWrite(t *testing.T) {
	svc := NewService(Config{Repo: &fakeRepo{inv: pendingInvitation()}})
	if _, err := svc.Accept(context.Background(), acceptInput()); err != nil {
		t.Fatalf("Accept with nil claim setter: %v", err)
	}
}
