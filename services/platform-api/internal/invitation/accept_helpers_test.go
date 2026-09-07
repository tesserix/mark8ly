package invitation

import (
	"context"
	"time"
)

// Shared doubles and fixtures for the Accept tests in this package.

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
