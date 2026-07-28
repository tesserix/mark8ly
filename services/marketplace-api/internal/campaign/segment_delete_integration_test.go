//go:build integration

package campaign_test

import (
	"context"
	"errors"
	"testing"

	"github.com/google/uuid"
	"gorm.io/gorm"

	"github.com/mark8ly/marketplace-api/internal/campaign"
	"github.com/mark8ly/marketplace-api/pkg/apperrors"
	"github.com/mark8ly/marketplace-api/pkg/testdb"
)

func seedStoreForSegment(t *testing.T, tx *gorm.DB) (storeID, tenantID uuid.UUID) {
	t.Helper()
	storeID = uuid.New()
	tenantID = uuid.New()
	// storefront_customer_portal_secret has no default and is NOT NULL.
	err := tx.Exec(`
		INSERT INTO stores (id, tenant_id, slug, name, country_code, currency_code,
		                    timezone, status, synced_at, storefront_customer_portal_secret)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, now(), encode(gen_random_bytes(32), 'hex'))`,
		storeID, tenantID, "seg-del-"+storeID.String()[:8], "Segment Delete Store",
		"US", "USD", "UTC", "active").Error
	if err != nil {
		t.Fatalf("insert store: %v", err)
	}
	return
}

func seedSegment(t *testing.T, tx *gorm.DB, storeID, tenantID uuid.UUID) uuid.UUID {
	t.Helper()
	seg := &campaign.CustomerSegment{
		TenantID: tenantID,
		StoreID:  storeID,
		Name:     "Repeat buyers",
		Rules:    []byte(`[]`),
	}
	if err := tx.Create(seg).Error; err != nil {
		t.Fatalf("insert segment: %v", err)
	}
	return seg.ID
}

func segmentExists(t *testing.T, tx *gorm.DB, id uuid.UUID) bool {
	t.Helper()
	var n int64
	if err := tx.Raw(`SELECT count(*) FROM customer_segments WHERE id = ?`, id).Scan(&n).Error; err != nil {
		t.Fatalf("count segment: %v", err)
	}
	return n > 0
}

func newCampaignService(tx *gorm.DB) *campaign.Service {
	return campaign.NewService(campaign.ServiceConfig{
		DB:   tx,
		Repo: campaign.NewRepository(tx),
	})
}

// TestIntegration_DeleteSegment_ReferencedByCampaign is the red→green guard
// against the raw-FK-violation-as-500 bug. Against a real Postgres, deleting a
// segment a campaign points at must fail with the typed segment_in_use error
// (409) and leave the segment row in place.
func TestIntegration_DeleteSegment_ReferencedByCampaign(t *testing.T) {
	tx := testdb.NewTx(t)
	svc := newCampaignService(tx)
	storeID, tenantID := seedStoreForSegment(t, tx)
	segID := seedSegment(t, tx, storeID, tenantID)

	c := &campaign.Campaign{
		TenantID:  tenantID,
		StoreID:   storeID,
		Name:      "Winback",
		SegmentID: &segID,
	}
	if err := svc.CreateCampaign(context.Background(), c); err != nil {
		t.Fatalf("create campaign: %v", err)
	}

	err := svc.DeleteSegment(context.Background(), segID)
	if err == nil {
		t.Fatal("expected delete of a referenced segment to fail, got nil")
	}
	if !errors.Is(err, apperrors.ErrSegmentInUse) {
		t.Fatalf("expected ErrSegmentInUse, got %v", err)
	}

	var ae *apperrors.Error
	if !errors.As(err, &ae) {
		t.Fatalf("expected *apperrors.Error, got %T", err)
	}
	if got := ae.Details["campaign_count"]; got != int64(1) {
		t.Fatalf("campaign_count = %v (%T), want int64(1)", got, got)
	}

	if !segmentExists(t, tx, segID) {
		t.Fatal("segment was deleted despite the refusal")
	}
}

// TestIntegration_DeleteSegment_TOCTOU covers the window the pre-check cannot
// close: a campaign inserted after the count still has to produce the typed
// 409, translated from Postgres' 23503, rather than a 500.
func TestIntegration_DeleteSegment_TOCTOU(t *testing.T) {
	tx := testdb.NewTx(t)
	repo := campaign.NewRepository(tx)
	storeID, tenantID := seedStoreForSegment(t, tx)
	segID := seedSegment(t, tx, storeID, tenantID)

	// Bypass the service pre-check entirely — this is exactly the state the
	// racing request lands in: count said 0, then a campaign appeared.
	c := &campaign.Campaign{
		TenantID:  tenantID,
		StoreID:   storeID,
		Name:      "Raced in",
		SegmentID: &segID,
	}
	if err := repo.CreateCampaign(tx, c); err != nil {
		t.Fatalf("create campaign: %v", err)
	}

	err := repo.DeleteSegment(tx, segID)
	if !errors.Is(err, apperrors.ErrSegmentInUse) {
		t.Fatalf("expected ErrSegmentInUse from the FK violation, got %v", err)
	}
}

// TestIntegration_DeleteSegment_Unreferenced keeps the happy path honest.
func TestIntegration_DeleteSegment_Unreferenced(t *testing.T) {
	tx := testdb.NewTx(t)
	svc := newCampaignService(tx)
	storeID, tenantID := seedStoreForSegment(t, tx)
	segID := seedSegment(t, tx, storeID, tenantID)

	if err := svc.DeleteSegment(context.Background(), segID); err != nil {
		t.Fatalf("delete unreferenced segment: %v", err)
	}
	if segmentExists(t, tx, segID) {
		t.Fatal("segment still present after a successful delete")
	}
}
