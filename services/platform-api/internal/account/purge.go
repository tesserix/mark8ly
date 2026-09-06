package account

import (
	"context"
	"fmt"
	"sort"
	"time"

	"gorm.io/gorm"

	"github.com/mark8ly/platform-api/internal/tenant"
)

// MismatchError is returned by PurgeTenant when the operator's supplied
// store-slug set does not equal the tenant's actual set.
//
// It carries the actual set so the caller can answer 409 with what the
// console should have sent, sparing it a second round trip. Disclosing it
// is safe: the caller is already authenticated on the internal boundary
// and already holds the tenant's detail row.
type MismatchError struct {
	Expected []string
}

func (e *MismatchError) Error() string {
	return fmt.Sprintf("account: store slug confirmation mismatch; expected %v", e.Expected)
}

// PurgeResult is what a successful operator teardown reports back.
// StoreIDs is what marketplace-api needs to scope its own purge; the rest
// is echoed to the operator.
type PurgeResult struct {
	TenantID   string
	TenantName string
	StoreIDs   []string
	StoreSlugs []string
}

// PurgeBackstopDelay defers the first drain attempt of the tenant.deleted
// event enqueued by PurgeTenant.
//
// The operator purge does the marketplace-side purge INLINE, and its whole
// point is to report what it destroyed. The outbox event exists so the purge
// still happens if this process dies between the teardown commit and that
// inline call — a backstop.
//
// Undelayed, it is not a backstop but a competitor, and it wins. Measured in
// production on 2026-08-25: the teardown committed at 14:12:55.405, the
// drainer completed the whole marketplace purge at 14:12:55.580, and the
// inline purge did not finish until 14:12:57.211 — so it deleted 0 rows and
// reported `total_rows: 0` for a purge that destroyed 5. The operator was
// told nothing was destroyed, and the permanent audit record said the same.
// That is a direct violation of #288's "audited with ... what was destroyed",
// and it is non-deterministic: whichever transaction gets the locks first
// wins.
//
// 30s is chosen to be far longer than any plausible inline purge (the plan
// issues 53 DELETEs in one transaction; the run above took ~1.8s) while
// still bounding how long a crashed request leaves rows stranded. It costs
// nothing in the happy path: the inline purge finishes first, so the drained
// event is a no-op re-run, which Purge is designed for.
const PurgeBackstopDelay = 30 * time.Second

// PurgeTenant is the operator-initiated tenant teardown behind
// POST /admin/tenants/{id}/purge (#288). It is IRREVERSIBLE.
//
// It is deliberately NOT a branch of DeleteAccount: that function opens by
// resolving the ACTOR's FGA role and requires RoleOwner, and a platform
// operator holds no FGA role at all. Sharing the entry point would mean
// threading a skip-authorization flag through the one function whose job
// is authorization.
//
// The confirmation check runs INSIDE the teardown transaction, against a
// snapshot taken under SELECT ... FOR UPDATE. Postgres runs at READ
// COMMITTED by default, so every statement inside the transaction takes
// its own fresh snapshot — running the comparison inside the transaction
// NARROWS the window in which a store could be created and missed by the
// check, it does not close it. A store committed by another session
// between the snapshot statement and the DELETE FROM tenants is cascaded
// away without ever appearing in the confirmed set, under either
// arrangement. That narrower window is the actual reason for doing the
// comparison in-transaction; concurrent store creation during a purge is
// not defended against.
//
// Exactly one purge of a given tenant succeeding is enforced by
// DeleteInTx's RowsAffected == 0 check, riding Postgres's implicit row
// lock on DELETE — that's the mechanism
// TestPurgeTenant_Integration_ConcurrentPurgesHaveExactlyOneWinner proves
// (one winner, by whatever mechanism; it isn't evidence for FOR UPDATE
// specifically). SELECT ... FOR UPDATE is kept as defence in depth and to
// narrow the snapshot window above, not because a test shows it
// load-bearing: removing it, that same test still passed 8/8.
//
// Post-commit cleanup mirrors deleteOwnerAccount and is best-effort for
// the same reason: the tenant.deleted outbox event enqueued inside the
// transaction is the real retry channel, so an FGA or GIP hiccup is logged
// rather than surfaced. fga and gip may be nil here — unlike the merchant
// path, this route is mounted unconditionally, because a route that is
// absent answers 404 and the caller cannot tell that apart from "no such
// tenant".
//
// "May be nil" means a TRUE nil interface, and that is a property of the
// WIRING, not of this file: s.fga is authz.Client, an interface all the
// way up through main, and s.gip is only ever assigned from a NON-nil
// *gipadmin.AdminClient — see newAccountService in
// cmd/server/account_wiring.go and its test. Handing NewService a nil
// concrete pointer directly would produce a non-nil interface holding a
// nil pointer; cleanupAfterTeardown's `!= nil` guards would pass, and the
// nil receiver would panic AFTER this transaction has already committed.
//
// #361 FIXED: cleanupAfterTeardown now enumerates every member via
// authz.Client.ListTenantMembers (not just snap.OwnerUserID), so
// staff/admin/viewer tuples no longer survive pointing at a tenant object
// that no longer exists. See cleanupAfterTeardown and
// Service.cleanupTenantMembers in service.go.
func (s *Service) PurgeTenant(ctx context.Context, tenantID string, suppliedSlugs []string) (*PurgeResult, error) {
	var snap *tenant.TeardownSnapshot

	run := func(tx *gorm.DB) error {
		var err error
		snap, err = s.repo.SnapshotForTeardown(ctx, tx, tenantID)
		if err != nil {
			return err
		}

		actual := slugsOf(snap.Stores)
		if !sameSet(suppliedSlugs, actual) {
			return &MismatchError{Expected: actual}
		}

		if err := s.repo.DeleteInTx(ctx, tx, tenantID); err != nil {
			return err
		}
		// PurgeBackstopDelay, not 0: this request purges marketplace-api
		// INLINE and reports what it destroyed. An immediately-drainable
		// event makes the drainer race that inline purge — see the constant.
		return s.outbox(tx, TenantDeletedOutboxKind, tenantDeletedPayload{
			TenantID: tenantID,
			StoreIDs: idsOf(snap.Stores),
		}, PurgeBackstopDelay)
	}

	if s.db == nil {
		if err := run(nil); err != nil {
			return nil, err
		}
	} else if err := s.db.WithContext(ctx).Transaction(run); err != nil {
		return nil, err
	}

	s.cleanupAfterTeardown(ctx, snap)

	return &PurgeResult{
		TenantID:   tenantID,
		TenantName: snap.Name,
		StoreIDs:   idsOf(snap.Stores),
		StoreSlugs: slugsOf(snap.Stores),
	}, nil
}

// cleanupAfterTeardown removes every member's FGA role tuple(s), the store
// parent tuples, and (conditionally) member identities — via the same
// cleanupTenantMembers helper deleteOwnerAccount uses, so the merchant and
// operator teardown paths can't drift on which users get cleaned up (#361:
// before this, only snap.OwnerUserID was unwound, leaving staff/admin/
// viewer tuples pointing at a tenant object that no longer exists). Every
// failure is logged and swallowed: the transaction has already committed
// and the outbox event is the durable retry channel. Nil-tolerant on both
// clients — cleanupTenantMembers no-ops when s.fga is nil, and
// deleteIdentityIfNoTenantsRemain (called from within it) no-ops when
// s.gip is nil.
func (s *Service) cleanupAfterTeardown(ctx context.Context, snap *tenant.TeardownSnapshot) {
	if s.fga != nil {
		s.cleanupTenantMembers(ctx, snap.TenantID)
		for _, st := range snap.Stores {
			if err := s.fga.DeleteStoreParent(ctx, st.ID, snap.TenantID); err != nil {
				s.warn("account: post-purge store parent delete failed",
					"tenant_id", snap.TenantID, "store_id", st.ID, "err", err)
			}
		}
	} else {
		s.warn("account: post-purge FGA cleanup skipped, no client configured",
			"tenant_id", snap.TenantID)
	}

	if s.gip == nil {
		s.warn("account: post-purge GIP cleanup skipped, no client configured",
			"tenant_id", snap.TenantID)
	}
}

func idsOf(refs []tenant.StoreRef) []string {
	out := make([]string, 0, len(refs))
	for _, r := range refs {
		out = append(out, r.ID)
	}
	return out
}

func slugsOf(refs []tenant.StoreRef) []string {
	out := make([]string, 0, len(refs))
	for _, r := range refs {
		out = append(out, r.Slug)
	}
	sort.Strings(out)
	return out
}

// sameSet reports whether a and b contain exactly the same slugs,
// ignoring order. A supplied subset is NOT a match: a comparison
// implemented as "every supplied slug exists" would silently accept an
// operator who confirmed one store of two.
func sameSet(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	x := append([]string(nil), a...)
	y := append([]string(nil), b...)
	sort.Strings(x)
	sort.Strings(y)
	for i := range x {
		if x[i] != y[i] {
			return false
		}
	}
	return true
}
