package lifecycle

import (
	"context"
	"errors"
	"testing"

	"github.com/google/uuid"
)

// These are pure unit tests: they never touch a database. The validation
// they cover runs before any SQL is issued, which is precisely the
// property being asserted — a rejected event must leave no row behind.

func TestValidateEvent_RejectsEventWithNoAppIdentifiers(t *testing.T) {
	err := validateEvent(ProAppCancelledEvent{
		TenantID: uuid.New(),
		StoreID:  uuid.New(),
	})
	if !errors.Is(err, ErrNoAppIdentifiers) {
		t.Fatalf("validateEvent(no identifiers) = %v; want ErrNoAppIdentifiers", err)
	}
}

func TestValidateEvent_AcceptsAnySingleIdentifier(t *testing.T) {
	base := ProAppCancelledEvent{TenantID: uuid.New(), StoreID: uuid.New()}

	cases := map[string]ProAppCancelledEvent{
		"apple only": func() ProAppCancelledEvent {
			ev := base
			ev.AppleAppID = "1234567890"
			return ev
		}(),
		"google only": func() ProAppCancelledEvent {
			ev := base
			ev.GooglePackage = "com.example.store"
			return ev
		}(),
		"firebase only": func() ProAppCancelledEvent {
			ev := base
			ev.FirebaseProjectID = "fb-store-1"
			return ev
		}(),
	}

	for name, ev := range cases {
		t.Run(name, func(t *testing.T) {
			if err := validateEvent(ev); err != nil {
				t.Fatalf("validateEvent(%s) = %v; want nil — a store may hold one platform without the others", name, err)
			}
		})
	}
}

func TestValidateEvent_StillRequiresTenantAndStore(t *testing.T) {
	cases := map[string]ProAppCancelledEvent{
		"missing tenant": {TenantID: uuid.Nil, StoreID: uuid.New(), AppleAppID: "a1"},
		"missing store":  {TenantID: uuid.New(), StoreID: uuid.Nil, AppleAppID: "a1"},
	}

	for name, ev := range cases {
		t.Run(name, func(t *testing.T) {
			err := validateEvent(ev)
			if err == nil {
				t.Fatalf("validateEvent(%s) = nil; want error", name)
			}
			if errors.Is(err, ErrNoAppIdentifiers) {
				t.Fatalf("validateEvent(%s) returned ErrNoAppIdentifiers; identifier presence is a separate failure", name)
			}
		})
	}
}

// Handle is given a nil *gorm.DB on purpose: if validation did not return
// before the insert, the call would panic on the nil handle. A clean
// sentinel return is proof that no write was attempted.
func TestHandle_RejectsBeforeTouchingTheDatabase(t *testing.T) {
	c := NewProAppCancelledConsumer(nil, nil)

	err := c.Handle(context.Background(), ProAppCancelledEvent{
		TenantID: uuid.New(),
		StoreID:  uuid.New(),
	})
	if !errors.Is(err, ErrNoAppIdentifiers) {
		t.Fatalf("Handle(no identifiers) = %v; want ErrNoAppIdentifiers", err)
	}
}

func TestHandle_RejectsMissingIDsBeforeTouchingTheDatabase(t *testing.T) {
	c := NewProAppCancelledConsumer(nil, nil)

	if err := c.Handle(context.Background(), ProAppCancelledEvent{StoreID: uuid.New(), AppleAppID: "a1"}); err == nil {
		t.Fatal("Handle(missing tenant) = nil; want error")
	}
	if err := c.Handle(context.Background(), ProAppCancelledEvent{TenantID: uuid.New(), AppleAppID: "a1"}); err == nil {
		t.Fatal("Handle(missing store) = nil; want error")
	}
}
