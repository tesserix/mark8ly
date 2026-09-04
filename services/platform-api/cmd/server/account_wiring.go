package main

import (
	"context"
	"log/slog"
	"time"

	"gorm.io/gorm"

	"github.com/mark8ly/platform-api/internal/account"
	"github.com/mark8ly/platform-api/internal/authz"
)

// gipAccountDeleter mirrors internal/account's unexported gipDeleter
// interface. It exists so this package can hold a TRUE nil interface value
// when no delete-account provider is configured, and so it is satisfied
// equally by *gipadmin.AdminClient and *zitadeladmin.Client (#524 phase 5).
// See newAccountService and selectAccountProviders.
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
// # gip must already be a genuinely nil-or-real interface value
//
// account.NewService's gip parameter is an INTERFACE
// (gipAccountDeleter/the package-local mirror of internal/account's
// unexported gipDeleter). Since #524 phase 5 task 3, the caller —
// selectAccountProviders — is the place that performs the typed-nil guard:
// it only ever assigns a concrete, non-nil client (gipAdmin OR a
// *zitadeladmin.Client) into the interface it returns, or leaves it a true
// nil interface when no provider is configured. newAccountService trusts
// that and passes gip straight through.
//
// Do NOT reintroduce the trap here by accepting a concrete
// *gipadmin.AdminClient again and assigning it unconditionally — see
// selectAccountProviders' doc (provider_wiring.go) and
// TestSelectAccountProviders_FlagUnset_NilGIPStaysGenuinelyNil for why: a
// nil *AdminClient assigned straight into an interface is a NON-NIL
// interface holding a nil pointer, so Service.cleanupAfterTeardown's
// `if s.gip != nil` guard PASSES and DeleteAccount runs on a nil receiver,
// panicking as it dereferences the client's config — AFTER the teardown
// transaction has committed. gin's Recovery then answers 500,
// marketplace-api's tenantlifecycle maps it to ErrUnavailable, and the
// operator is told `503 upstream_unavailable` for a tenant that is already
// destroyed and whose purge was never audited.
//
// fga needs no such treatment: it is declared in main as `var fga
// authz.Client`, an interface all the way down, so it is a TRUE nil
// interface when OpenFGA is unconfigured and Service's `if s.fga != nil`
// guard works as written. It is threaded through here unchanged.
func newAccountService(
	conn *gorm.DB,
	repo account.TenantRepo,
	fga authz.Client,
	gip gipAccountDeleter,
	// enqueue is deliberately an UNNAMED func type: a named type here would
	// not be assignable to internal/account's named outboxEnqueuer. The
	// delay parameter is outbox.EnqueueAfter's — see account.outboxEnqueuer
	// for why the two tenant.deleted callers want different values.
	enqueue func(tx *gorm.DB, kind string, payload any, delay time.Duration) error,
	log *slog.Logger,
) *account.Service {
	return account.NewService(conn, repo, fga, gip, enqueue, log)
}
