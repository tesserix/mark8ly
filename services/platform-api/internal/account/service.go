// Package account implements the account-teardown service: what happens
// when a merchant asks to delete their Mark8ly account.
//
// The MVP branches on the actor's FGA role for the tenant:
//
//   - owner: full tenant teardown. The tenant row (and everything that
//     cascades from it — stores, invitations) is deleted inside one DB
//     transaction alongside a "tenant.deleted" outbox event, so the
//     Phase 5 marketplace purge has a durable, retryable trigger even if
//     the process crashes right after commit. Once that transaction
//     commits, FGA tuples and the GIP identity are cleaned up on a
//     best-effort basis: every one of those primitives is idempotent and
//     the outbox event is the real retry channel, so a GIP or FGA hiccup
//     here is logged, not surfaced to the caller.
//
//   - admin/staff/viewer ("staff" for short): platform-api has no
//     membership table — team membership is expressed entirely as FGA
//     role tuples plus accepted `invitation` rows keyed by email (with no
//     by-user-id delete method). Cleaning up the accepted-invitation row
//     for a departing non-owner is explicitly deferred to Phase 5; see
//     task-4-brief.md CONTROLLER RESOLUTION #1. For the MVP this branch
//     only removes the actor's FGA role tuple and their GIP identity. The
//     tenant itself is untouched.
//
//   - no role at all: apperrors.Forbidden — the actor isn't a member of
//     the tenant they're asking to leave.
package account

import (
	"context"
	"fmt"
	"log/slog"
	"time"

	"gorm.io/gorm"

	"github.com/mark8ly/platform-api/internal/authz"
	"github.com/mark8ly/platform-api/internal/tenant"
	apperrors "github.com/mark8ly/platform-api/pkg/errors"
)

// gipDeleter is the subset of the identity-provider client the service
// needs. Defined locally so *zitadeladmin.Client satisfies it without this
// package importing it, and so tests can supply a trivial fake.
type gipDeleter interface {
	DeleteAccount(ctx context.Context, uid string) error
}

// outboxEnqueuer matches the real outbox.EnqueueAfter function's signature
// (see internal/outbox/outbox.go). It's a func type rather than a
// method-interface so outbox.EnqueueAfter itself can be passed directly in
// production with no adapter, while tests supply a recording fake's
// method value.
//
// The delay is part of the TYPE, not a detail hidden inside the outbox
// package, because the two paths that enqueue tenant.deleted want opposite
// things from it. The merchant self-serve delete has no inline purge — the
// outbox IS the purge, so it passes 0. The operator purge (#288) does the
// purge inline and needs the event to be a genuine backstop rather than a
// competitor, so it passes PurgeBackstopDelay. A caller that has to choose
// a value is a caller that has to think about which it is.
type outboxEnqueuer func(tx *gorm.DB, kind string, payload any, delay time.Duration) error

// TenantRepo is the subset of tenant.Repository the service uses.
// ListStoreIDs is needed before the DB cascade removes the stores out
// from under the FGA store-parent tuples; DeleteInTx performs the actual
// tenant row (+ cascades) delete inside the caller's transaction.
type TenantRepo interface {
	ListStoreIDs(ctx context.Context, tx *gorm.DB, tenantID string) ([]string, error)
	DeleteInTx(ctx context.Context, tx *gorm.DB, tenantID string) error
	// SnapshotForTeardown is used by the operator purge path to read the
	// tenant's identifying state under lock, inside the same transaction
	// that deletes it. See PurgeTenant.
	SnapshotForTeardown(ctx context.Context, tx *gorm.DB, tenantID string) (*tenant.TeardownSnapshot, error)
}

// tenantDeletedPayload is the outbox payload for "tenant.deleted". The
// Phase 5 marketplace purge drainer reads store_ids to know which
// product/order/etc. rows to sweep without having to re-derive them from
// a tenant row that no longer exists.
type tenantDeletedPayload struct {
	TenantID string   `json:"tenant_id"`
	StoreIDs []string `json:"store_ids"`
}

// Service is the account-teardown business logic.
type Service struct {
	db     *gorm.DB
	repo   TenantRepo
	fga    authz.Client
	gip    gipDeleter
	outbox outboxEnqueuer
	log    *slog.Logger
}

// NewService constructs a Service. db may be nil in unit tests: the
// atomic tenant-delete + outbox-enqueue step short-circuits to running
// the same two calls without a real transaction wrapper when db is nil,
// which is safe because the fakes standing in for repo/outbox in that
// case don't need a real DB transaction to prove atomicity — only
// production wiring (a non-nil db) needs the real gorm.DB.Transaction
// guarantee.
func NewService(db *gorm.DB, repo TenantRepo, fga authz.Client, gip gipDeleter, outbox outboxEnqueuer, log *slog.Logger) *Service {
	return &Service{db: db, repo: repo, fga: fga, gip: gip, outbox: outbox, log: log}
}

// DeleteAccount deletes the calling actor's account in the context of the
// given tenant. Owners get a full tenant teardown; everyone else just
// removes their own membership. An actor with no role on the tenant is
// rejected with apperrors.Forbidden.
func (s *Service) DeleteAccount(ctx context.Context, tenantID, actorUID string) error {
	role, err := s.fga.GetRole(ctx, actorUID, tenantID)
	if err != nil {
		return fmt.Errorf("account: get role: %w", err)
	}
	if role == "" {
		return apperrors.Forbidden("not_a_member", "actor has no role on this tenant")
	}

	if role == authz.RoleOwner {
		return s.deleteOwnerAccount(ctx, tenantID, actorUID)
	}
	return s.deleteStaffAccount(ctx, tenantID, actorUID, role)
}

// deleteOwnerAccount runs the atomic DB teardown, then best-effort cleans
// up FGA tuples and GIP identities for every member of the tenant — not
// just the owner (actorUID). A tenant can have staff/admin/viewer
// invitees with their own role tuples, and those tuples must not survive
// pointing at a tenant object that no longer exists (#361). See the
// package doc for why this second half is best-effort.
func (s *Service) deleteOwnerAccount(ctx context.Context, tenantID, actorUID string) error {
	storeIDs, err := s.repo.ListStoreIDs(ctx, nil, tenantID)
	if err != nil {
		return fmt.Errorf("account: list store ids: %w", err)
	}

	if err := s.teardownTenantTx(ctx, tenantID, storeIDs); err != nil {
		return fmt.Errorf("account: teardown tenant: %w", err)
	}

	s.cleanupTenantMembers(ctx, tenantID)
	for _, storeID := range storeIDs {
		if err := s.fga.DeleteStoreParent(ctx, storeID, tenantID); err != nil {
			s.warn("account: post-commit store parent delete failed", "tenant_id", tenantID, "store_id", storeID, "err", err)
		}
	}
	return nil
}

// cleanupTenantMembers enumerates every user holding a direct FGA role
// tuple on tenantID — the owner included — and unwinds their role
// tuple(s) and (conditionally) their identity. Called only after the
// tenant's DB row is already gone in the same transaction that enqueued
// the tenant.deleted outbox event, so every step here is best-effort:
// a failure is logged via s.warn and cleanup moves on, never aborting or
// returning an error.
//
// Nil-tolerant on s.fga: PurgeTenant's route is mounted unconditionally
// and may be wired with fga == nil (see purge.go's cleanupAfterTeardown
// doc). deleteOwnerAccount's caller always wires a real client, so this
// nil check is a no-op there in practice — it exists so the same helper
// serves both call sites without each having to remember to guard.
func (s *Service) cleanupTenantMembers(ctx context.Context, tenantID string) {
	if s.fga == nil {
		return
	}
	members, err := s.fga.ListTenantMembers(ctx, tenantID)
	if err != nil {
		s.warn("account: post-commit list tenant members failed", "tenant_id", tenantID, "err", err)
		return
	}

	// A user can hold more than one direct role tuple on the same
	// tenant (e.g. a staff→admin promotion intentionally leaves the old
	// tuple in place for audit trail — see authz.rolePriority's doc
	// comment). Remove every tuple for a user before deciding whether
	// to delete their identity, so that decision is made once per user
	// against a tenant that's already fully unwound for them.
	touchedUsers := map[string]bool{}
	for _, m := range members {
		if err := s.fga.DeleteTuple(ctx, m.UserID, m.Relation, tenantID); err != nil {
			s.warn("account: post-commit member tuple delete failed",
				"tenant_id", tenantID, "user_id", m.UserID, "relation", m.Relation, "err", err)
		}
		touchedUsers[m.UserID] = true
	}
	for userID := range touchedUsers {
		s.deleteIdentityIfNoTenantsRemain(ctx, userID, tenantID)
	}
}

// deleteIdentityIfNoTenantsRemain deletes userID's GIP/Zitadel identity
// ONLY if authz.ListMemberTenants shows no remaining tenant membership
// anywhere. A user can hold roles on multiple tenants — that's exactly
// why ListMemberTenants exists — so deleting their identity because ONE
// of their tenants was torn down would destroy their access to every
// other tenant they belong to.
//
// If the ListMemberTenants lookup itself fails, the answer is
// inconclusive and this deliberately does NOT delete: an inconclusive
// answer must never be treated as "safe to delete". The failure is
// logged and the identity deletion is skipped.
func (s *Service) deleteIdentityIfNoTenantsRemain(ctx context.Context, userID, justRemovedTenantID string) {
	if s.gip == nil {
		return
	}
	remaining, err := s.fga.ListMemberTenants(ctx, userID)
	if err != nil {
		s.warn("account: post-commit list member tenants failed, skipping identity delete",
			"user_id", userID, "tenant_id", justRemovedTenantID, "err", err)
		return
	}
	if len(remaining) > 0 {
		return
	}
	if err := s.gip.DeleteAccount(ctx, userID); err != nil {
		s.warn("account: post-commit identity delete failed",
			"user_id", userID, "tenant_id", justRemovedTenantID, "err", err)
	}
}

// teardownTenantTx deletes the tenant row and enqueues the tenant.deleted
// outbox event in one transaction, so a crash between the two never
// happens: either both land, or neither does.
func (s *Service) teardownTenantTx(ctx context.Context, tenantID string, storeIDs []string) error {
	run := func(tx *gorm.DB) error {
		if err := s.repo.DeleteInTx(ctx, tx, tenantID); err != nil {
			return err
		}
		payload := tenantDeletedPayload{TenantID: tenantID, StoreIDs: storeIDs}
		// 0: the merchant path runs NO inline purge, so this event is the
		// only thing that will ever purge marketplace-api. Drain it at once.
		return s.outbox(tx, TenantDeletedOutboxKind, payload, 0)
	}
	if s.db == nil {
		// Unit tests construct the service with a nil db and fakes that
		// don't require a real transaction to prove atomicity.
		return run(nil)
	}
	return s.db.WithContext(ctx).Transaction(run)
}

// deleteStaffAccount removes the actor's FGA role tuple and GIP identity.
// Unlike the owner path there is no outbox event backing this up, so
// failures here propagate to the caller rather than being swallowed —
// the caller can safely retry the whole DeleteAccount call since both
// primitives are idempotent. The tenant and any accepted invitation rows
// are intentionally left untouched; see the package doc.
func (s *Service) deleteStaffAccount(ctx context.Context, tenantID, actorUID string, role authz.Role) error {
	if err := s.fga.DeleteTuple(ctx, actorUID, string(role), tenantID); err != nil {
		return fmt.Errorf("account: delete role tuple: %w", err)
	}
	// Same rule as the teardown paths (#361), and the case most likely to
	// occur: a staff member who belongs to two tenants leaves ONE of them.
	// Deleting their identity here would take their access to the other
	// tenant with it. Unlike cleanupTenantMembers this runs inside the
	// caller's request rather than post-commit, so failures are returned
	// rather than logged — including a failed membership lookup, because
	// an inconclusive answer must not fall through to a delete.
	remaining, err := s.fga.ListMemberTenants(ctx, actorUID)
	if err != nil {
		return fmt.Errorf("account: list member tenants: %w", err)
	}
	if len(remaining) > 0 {
		return nil
	}
	if err := s.gip.DeleteAccount(ctx, actorUID); err != nil {
		return fmt.Errorf("account: delete gip account: %w", err)
	}
	return nil
}

// warn logs a best-effort cleanup failure. Nil-safe so tests that don't
// care about logging can pass log=nil without panicking.
func (s *Service) warn(msg string, args ...any) {
	if s.log == nil {
		return
	}
	s.log.Warn(msg, args...)
}
