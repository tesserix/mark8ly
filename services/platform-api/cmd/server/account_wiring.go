package main

import (
	"context"
	"log/slog"
	"time"

	"gorm.io/gorm"

	"github.com/mark8ly/platform-api/internal/account"
	"github.com/mark8ly/platform-api/internal/authz"
	"github.com/mark8ly/platform-api/internal/gipadmin"
)

// gipAccountDeleter mirrors internal/account's unexported gipDeleter
// interface. It exists so this package can hold a TRUE nil interface value
// when GIP is unconfigured — see newAccountService.
type gipAccountDeleter interface {
	DeleteAccount(ctx context.Context, uid string) error
}

// newAccountService builds the account-teardown service that backs BOTH the
// merchant DeleteAccount route and the operator tenant-purge route (#288).
//
// It exists as a function purely so the nil-client wiring below is testable
// (see account_wiring_test.go). This is NOT the general main.go refactor —
// that is #323.
//
// # Why gipAdmin is not passed straight through
//
// gipAdmin is a CONCRETE *gipadmin.AdminClient; account.NewService's
// parameter is an INTERFACE. Assigning a nil *AdminClient into an interface
// produces a NON-NIL interface value holding a nil pointer, so
// Service.cleanupAfterTeardown's `if s.gip != nil` guard PASSES and
// DeleteAccount runs on a nil receiver, panicking as it dereferences the
// client's config.
//
// That panic lands AFTER the teardown transaction has committed. gin's
// Recovery answers 500, marketplace-api's tenantlifecycle maps it to
// ErrUnavailable, and the operator is told `503 upstream_unavailable` — for
// a tenant that is already destroyed and whose purge was never audited.
// platform-api deployed without GIP_PROJECT_ID/GIP_TENANT_ID/
// GIP_WEB_API_KEY is a real configuration; the startup warning above the
// call site exists because of it.
//
// So: declare the interface-typed variable, assign it ONLY inside the
// non-nil guard, and pass that. Same shape marketplace-api already uses for
// its TenantTeardown client (cmd/marketplace-api/main.go).
//
// fga needs no such treatment: it is declared in main as `var fga
// authz.Client`, an interface all the way down, so it is a TRUE nil
// interface when OpenFGA is unconfigured and Service's `if s.fga != nil`
// guard works as written. It is threaded through here unchanged.
func newAccountService(
	conn *gorm.DB,
	repo account.TenantRepo,
	fga authz.Client,
	gipAdmin *gipadmin.AdminClient,
	// enqueue is deliberately an UNNAMED func type: a named type here would
	// not be assignable to internal/account's named outboxEnqueuer. The
	// delay parameter is outbox.EnqueueAfter's — see account.outboxEnqueuer
	// for why the two tenant.deleted callers want different values.
	enqueue func(tx *gorm.DB, kind string, payload any, delay time.Duration) error,
	log *slog.Logger,
) *account.Service {
	var gipCleanup gipAccountDeleter
	if gipAdmin != nil {
		gipCleanup = gipAdmin
	}
	return account.NewService(conn, repo, fga, gipCleanup, enqueue, log)
}
