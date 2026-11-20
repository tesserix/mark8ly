//go:build integration

package delivery_test

import (
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/shopspring/decimal"

	"github.com/mark8ly/marketplace-api/internal/delivery"
	"github.com/mark8ly/marketplace-api/internal/stores"
	"github.com/mark8ly/marketplace-api/pkg/apperrors"
	"github.com/mark8ly/marketplace-api/pkg/testdb"
)

func seed(t *testing.T) (*delivery.Service, string, string) {
	db := testdb.NewDB(t, "store_delivery_options", "store_couriers", "stores")
	tid, sid := uuid.NewString(), uuid.NewString()
	if err := db.Create(&stores.Store{ID: sid, TenantID: tid, Slug: "d-" + sid[:8], Name: "S", CountryCode: "US",
		CurrencyCode: "USD", Timezone: "UTC", Status: stores.StatusActive, SyncedAt: time.Now()}).Error; err != nil {
		t.Fatal(err)
	}
	return delivery.NewService(db), tid, sid
}

func opt(name, price string) delivery.OptionInput {
	return delivery.OptionInput{Name: name, Price: decimal.RequireFromString(price), Enabled: true}
}

func TestReplaceOptionsValidation(t *testing.T) {
	svc, tid, sid := seed(t)
	ctx := t.Context()
	six := []delivery.OptionInput{opt("a", "1"), opt("b", "1"), opt("c", "1"), opt("d", "1"), opt("e", "1"), opt("f", "1")}
	for name, in := range map[string][]delivery.OptionInput{
		"six": six, "long name": {opt(strings.Repeat("x", 41), "1")}, "negative": {opt("a", "-1")},
	} {
		if _, err := svc.ReplaceOptions(ctx, tid, sid, in); !isCode(err, apperrors.CodeValidationFailed) {
			t.Errorf("%s: want validation_failed, got %v", name, err)
		}
	}
}

func TestReplaceOptionsOrderAndDelete(t *testing.T) {
	svc, tid, sid := seed(t)
	ctx := t.Context()
	got, err := svc.ReplaceOptions(ctx, tid, sid, []delivery.OptionInput{opt("Std", "5"), opt("Fast", "9.50")})
	if err != nil {
		t.Fatal(err)
	}
	// Reorder, rename one in place, drop the other, add a new one.
	got, err = svc.ReplaceOptions(ctx, tid, sid, []delivery.OptionInput{opt("New", "1"),
		{ID: got[1].ID, Name: "Fast2", Price: decimal.NewFromInt(10), Enabled: true}})
	if err != nil {
		t.Fatal(err)
	}
	s, _ := svc.Get(ctx, sid)
	if len(s.Options) != 2 || s.Options[0].Name != "New" || s.Options[1].Name != "Fast2" || s.Options[1].ID != got[1].ID {
		t.Fatalf("%+v", s.Options)
	}
	if s.Mode != "carrier" {
		t.Fatalf("mode %q", s.Mode)
	}
}

func TestEnabledOption(t *testing.T) {
	svc, tid, sid := seed(t)
	_, otherTid, otherSid := tid, "", ""
	_ = otherTid
	ctx := t.Context()
	off := opt("Off", "1")
	off.Enabled = false
	got, err := svc.ReplaceOptions(ctx, tid, sid, []delivery.OptionInput{opt("On", "1"), off})
	if err != nil {
		t.Fatal(err)
	}
	if o, err := svc.EnabledOption(ctx, sid, got[0].ID); err != nil || o.Name != "On" {
		t.Fatalf("%v %v", o, err)
	}
	otherSid = uuid.NewString()
	for name, args := range map[string][2]string{"disabled": {sid, got[1].ID}, "other store": {otherSid, got[0].ID}, "malformed": {sid, "nope"}} {
		if o, err := svc.EnabledOption(ctx, args[0], args[1]); o != nil || !isCode(err, apperrors.CodeNotFound) {
			t.Errorf("%s: %v %v", name, o, err)
		}
	}
}

func TestSetModeAndCouriers(t *testing.T) {
	svc, tid, sid := seed(t)
	ctx := t.Context()
	if err := svc.SetMode(ctx, sid, "x"); !isCode(err, apperrors.CodeValidationFailed) {
		t.Fatalf("%v", err)
	}
	if err := svc.SetMode(ctx, sid, "own"); err != nil {
		t.Fatal(err)
	}
	bad := "http://c.in/{number}"
	if _, err := svc.CreateCourier(ctx, tid, sid, delivery.CourierInput{Name: "C", TrackingURLTemplate: &bad}); !isCode(err, apperrors.CodeValidationFailed) {
		t.Fatalf("%v", err)
	}
	c, err := svc.CreateCourier(ctx, tid, sid, delivery.CourierInput{Name: "C"})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := svc.GetCourier(ctx, uuid.NewString(), c.ID); !isCode(err, apperrors.CodeNotFound) {
		t.Fatalf("other store: %v", err)
	}
	if _, err := svc.GetCourier(ctx, sid, "bad"); !isCode(err, apperrors.CodeNotFound) {
		t.Fatalf("malformed: %v", err)
	}
	if err := svc.DeleteCourier(ctx, sid, c.ID); err != nil {
		t.Fatal(err)
	}
	if _, err := svc.GetCourier(ctx, sid, c.ID); !isCode(err, apperrors.CodeNotFound) {
		t.Fatalf("deleted: %v", err)
	}
}
