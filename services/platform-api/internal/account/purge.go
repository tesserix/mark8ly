package account

import (
	"context"
	"fmt"
	"sort"

	"gorm.io/gorm"

	"github.com/mark8ly/platform-api/internal/authz"
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
// What the transaction plus SELECT ... FOR UPDATE on the tenant row does
// genuinely guarantee is serialising two concurrent PURGES of the same
// tenant: the second blocks on the lock until the first commits (or rolls
// back), then re-reads and finds no row, so exactly one purge can
// succeed. See TestPurgeTenant_Integration_ConcurrentPurgesHaveExactlyOneWinner.
//
// Post-commit cleanup mirrors deleteOwnerAccount and is best-effort for
// the same reason: the tenant.deleted outbox event enqueued inside the
// transaction is the real retry channel, so an FGA or GIP hiccup is logged
// rather than surfaced. fga and gip may be nil here — unlike the merchant
// path, this route is mounted unconditionally, because a route that is
// absent answers 404 and the caller cannot tell that apart from "no such
// tenant".
//
// KNOWN GAP, inherited from deleteOwnerAccount rather than introduced
// here: authz.Client has no method enumerating a tenant's members
// (DeleteTuple requires a userID), so staff/admin/viewer tuples and their
// GIP identities survive, pointing at a tenant object that no longer
// exists. Filed separately.
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
		return s.outbox(tx, TenantDeletedOutboxKind, tenantDeletedPayload{
			TenantID: tenantID,
			StoreIDs: idsOf(snap.Stores),
		})
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

// cleanupAfterTeardown removes the owner's FGA role tuple, the store
// parent tuples and the owner's GIP identity. Every failure is logged and
// swallowed: the transaction has already committed and the outbox event is
// the durable retry channel. Nil-tolerant on both clients.
func (s *Service) cleanupAfterTeardown(ctx context.Context, snap *tenant.TeardownSnapshot) {
	if s.fga != nil {
		if err := s.fga.DeleteTuple(ctx, snap.OwnerUserID, string(authz.RoleOwner), snap.TenantID); err != nil {
			s.warn("account: post-purge owner tuple delete failed",
				"tenant_id", snap.TenantID, "owner_uid", snap.OwnerUserID, "err", err)
		}
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

	if s.gip != nil {
		if err := s.gip.DeleteAccount(ctx, snap.OwnerUserID); err != nil {
			s.warn("account: post-purge gip delete failed",
				"tenant_id", snap.TenantID, "owner_uid", snap.OwnerUserID, "err", err)
		}
	} else {
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
