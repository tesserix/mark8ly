package account

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/mark8ly/platform-api/internal/authz"
	"github.com/mark8ly/platform-api/internal/tenant"
)

type fakeCounter struct {
	counts map[string]int64
	err    error
}

func (f fakeCounter) CountTenant(context.Context, string, []string) (map[string]int64, error) {
	return f.counts, f.err
}

func ownerDeleteFixture(t *testing.T) (*fakeTenantRepo, *fakeOutbox, *Service) {
	t.Helper()
	fga := authz.NewFake()
	if err := fga.WriteOwnership(context.Background(), "owner-1", "t1"); err != nil {
		t.Fatal(err)
	}
	signedUp := time.Date(2026, 11, 2, 0, 25, 0, 0, time.UTC)
	repo := &fakeTenantRepo{
		stores: map[string][]string{"t1": {"s1"}},
		snap: &tenant.TeardownSnapshot{TenantID: "t1", Name: "CreatedBy.E", OwnerEmail: "seller@example.com",
			CreatedAt: signedUp, Stores: []tenant.StoreRef{{ID: "s1", Slug: "createdbye"}}},
	}
	ob := &fakeOutbox{}
	svc := NewService(nil, repo, fga, &fakeGIP{}, ob.Enqueue, testLogger())
	svc.now = func() time.Time { return signedUp.Add(14*24*time.Hour + time.Hour) }
	return repo, ob, svc
}

func noticeFrom(t *testing.T, ob *fakeOutbox) *deletedNotice {
	t.Helper()
	for i, k := range ob.kinds {
		if k == TenantDeletedNoticeOutboxKind {
			return ob.payloads[i].(*deletedNotice)
		}
	}
	return nil
}

func TestDeleteAccount_DeletedNoticeCarriesSnapshotAndCounts(t *testing.T) {
	repo, ob, svc := ownerDeleteFixture(t)
	svc.EnableDeletedNotice(fakeCounter{counts: map[string]int64{"products": 2, "payment_gateway_configs": 1}})

	if err := svc.DeleteAccount(context.Background(), "t1", "owner-1"); err != nil {
		t.Fatal(err)
	}
	if !repo.deleted["t1"] || !ob.has(TenantDeletedOutboxKind) {
		t.Fatal("tenant must still be torn down and purged")
	}
	n := noticeFrom(t, ob)
	if n == nil {
		t.Fatal("no deleted notice enqueued")
	}
	if n.OwnerEmail != "seller@example.com" || n.Name != "CreatedBy.E" || len(n.StoreSlugs) != 1 || n.StoreSlugs[0] != "createdbye" || n.Counts["products"] != 2 {
		t.Fatalf("notice = %+v", n)
	}
	msg := n.message()
	for _, want := range []string{"**mark8ly account deleted**", "**Days active:** 14", "**Products:** 2", "**Payments connected:** yes", "**Custom domain:** no"} {
		if !strings.Contains(msg, want) {
			t.Errorf("message missing %q:\n%s", want, msg)
		}
	}
}

func TestDeleteAccount_CountFailureStillDeletesAndSaysSo(t *testing.T) {
	repo, ob, svc := ownerDeleteFixture(t)
	svc.EnableDeletedNotice(fakeCounter{err: errors.New("marketplace down")})

	if err := svc.DeleteAccount(context.Background(), "t1", "owner-1"); err != nil {
		t.Fatalf("a failed activity count must not block the delete: %v", err)
	}
	n := noticeFrom(t, ob)
	if !repo.deleted["t1"] || n == nil || n.Counts != nil {
		t.Fatalf("deleted=%v notice=%+v", repo.deleted["t1"], n)
	}
	if !strings.Contains(n.message(), "Activity counts unavailable") {
		t.Fatalf("message should flag missing counts:\n%s", n.message())
	}
}

func TestDeleteAccount_NoNoticeUnlessEnabled(t *testing.T) {
	_, ob, svc := ownerDeleteFixture(t)
	if err := svc.DeleteAccount(context.Background(), "t1", "owner-1"); err != nil {
		t.Fatal(err)
	}
	if ob.has(TenantDeletedNoticeOutboxKind) {
		t.Fatal("notice enqueued with Rocket.Chat disabled")
	}
}

func TestTenantDeletedNoticeHandler_PostsUnderStableIDAndEscapesInput(t *testing.T) {
	var gotID, gotText string
	h := NewTenantDeletedNoticeHandler(func(_ context.Context, id, text string) error {
		gotID, gotText = id, text
		return nil
	})
	payload, _ := json.Marshal(deletedNotice{TenantID: "t1", Name: "*bold* @here", DeletedAt: time.Now()})
	if err := h(context.Background(), payload); err != nil {
		t.Fatal(err)
	}
	if gotID != "mark8ly-deleted-t1" || strings.Contains(gotText, "*bold*") || strings.Contains(gotText, " @here") {
		t.Fatalf("id %q text %q", gotID, gotText)
	}
	if err := h(context.Background(), json.RawMessage(`{}`)); err == nil {
		t.Fatal("event without tenant_id must be rejected")
	}
}
