//go:build integration

package product_test

import (
	"context"
	"errors"
	"testing"

	"github.com/google/uuid"
	"github.com/shopspring/decimal"

	"github.com/mark8ly/marketplace-api/internal/media"
	"github.com/mark8ly/marketplace-api/internal/outbox"
	"github.com/mark8ly/marketplace-api/internal/product"
	"github.com/mark8ly/marketplace-api/internal/stores"
	"github.com/mark8ly/marketplace-api/internal/vendor"
	"github.com/mark8ly/marketplace-api/pkg/apperrors"
	"github.com/mark8ly/marketplace-api/pkg/testdb"
)

func seedProductForMedia(t *testing.T, svc *product.Service, storeID, tenantID string) *product.Aggregate {
	t.Helper()
	agg, err := svc.Create(context.Background(), product.CreateRequest{
		StoreID:  storeID,
		TenantID: tenantID,
		Title:    "Media Host " + uuid.NewString()[:6],
		Variants: []product.VariantInput{{
			SKU:          "MH-" + uuid.NewString()[:6],
			Price:        decimal.NewFromInt(10),
			CurrencyCode: "USD",
		}},
	})
	if err != nil {
		t.Fatalf("seed product: %v", err)
	}
	return agg
}

func TestIntegration_Service_AddMedia_HappyPath(t *testing.T) {
	tx := testdb.NewTx(t)
	storeID, tenantID, _ := seedStore(t, tx)
	up := media.NewFakeUploader()
	up.Register(media.Attrs{StorageKey: "k-happy", Size: 123, ContentType: "image/jpeg"})
	svc := buildService(tx, up)
	agg := seedProductForMedia(t, svc, storeID, tenantID)

	got, err := svc.AddMedia(context.Background(), product.AddMediaRequest{
		ProductID:  agg.Product.ID,
		StoreID:    storeID,
		TenantID:   tenantID,
		StorageKey: "k-happy",
		URL:        "https://cdn/a.jpg",
		Position:   0,
	})
	if err != nil {
		t.Fatalf("add media: %v", err)
	}
	if got == nil || got.ID == "" {
		t.Fatal("expected returned media row")
	}
	if got.GcsPathOriginal != "k-happy" {
		t.Fatalf("gcs_path_original = %q, want %q", got.GcsPathOriginal, "k-happy")
	}
	fresh, err := svc.Get(context.Background(), agg.Product.ID, storeID, tenantID)
	if err != nil {
		t.Fatalf("get: %v", err)
	}
	found := false
	for _, m := range fresh.Media {
		if m.StorageKey == "k-happy" {
			found = true
		}
	}
	if !found {
		t.Fatalf("expected media in fresh get: %+v", fresh.Media)
	}
	if n := countProductOutbox(t, tx, agg.Product.ID, "product.updated"); n != 1 {
		t.Fatalf("expected 1 product.updated, got %d", n)
	}
}

func TestIntegration_Service_AddMedia_UploadNotFound(t *testing.T) {
	tx := testdb.NewTx(t)
	storeID, tenantID, _ := seedStore(t, tx)
	svc := buildService(tx, media.NewFakeUploader())
	agg := seedProductForMedia(t, svc, storeID, tenantID)

	_, err := svc.AddMedia(context.Background(), product.AddMediaRequest{
		ProductID:  agg.Product.ID,
		StoreID:    storeID,
		TenantID:   tenantID,
		StorageKey: "missing",
		URL:        "https://cdn/x.jpg",
	})
	var ae *apperrors.Error
	if !errors.As(err, &ae) || ae.Code != apperrors.CodeUploadNotFound {
		t.Fatalf("expected UploadNotFound; got %v", err)
	}
}

func TestIntegration_Service_UpdateMedia_UpdatesAlt(t *testing.T) {
	tx := testdb.NewTx(t)
	storeID, tenantID, _ := seedStore(t, tx)
	up := media.NewFakeUploader()
	up.Register(media.Attrs{StorageKey: "k1", Size: 1, ContentType: "image/jpeg"})
	svc := buildService(tx, up)
	agg := seedProductForMedia(t, svc, storeID, tenantID)

	m, err := svc.AddMedia(context.Background(), product.AddMediaRequest{
		ProductID: agg.Product.ID, StoreID: storeID, TenantID: tenantID,
		StorageKey: "k1", URL: "https://cdn/1.jpg",
	})
	if err != nil {
		t.Fatal(err)
	}
	alt := "new alt"
	if err := svc.UpdateMedia(context.Background(), product.UpdateMediaRequest{
		ProductID: agg.Product.ID, MediaID: m.ID, StoreID: storeID, TenantID: tenantID,
		Alt: &alt,
	}); err != nil {
		t.Fatalf("update media: %v", err)
	}
	fresh, _ := svc.Get(context.Background(), agg.Product.ID, storeID, tenantID)
	for _, md := range fresh.Media {
		if md.ID == m.ID {
			if md.Alt == nil || *md.Alt != "new alt" {
				t.Fatalf("alt = %v, want %q", md.Alt, "new alt")
			}
			return
		}
	}
	t.Fatal("media not found")
}

func TestIntegration_Service_UpdateMedia_NotFound(t *testing.T) {
	tx := testdb.NewTx(t)
	storeID, tenantID, _ := seedStore(t, tx)
	svc := buildService(tx, media.NewFakeUploader())
	agg := seedProductForMedia(t, svc, storeID, tenantID)

	alt := "x"
	err := svc.UpdateMedia(context.Background(), product.UpdateMediaRequest{
		ProductID: agg.Product.ID, MediaID: uuid.NewString(), StoreID: storeID, TenantID: tenantID,
		Alt: &alt,
	})
	var ae *apperrors.Error
	if !errors.As(err, &ae) || ae.Code != apperrors.CodeNotFound {
		t.Fatalf("expected NotFound; got %v", err)
	}
}

func TestIntegration_Service_DeleteMedia_Succeeds(t *testing.T) {
	tx := testdb.NewTx(t)
	storeID, tenantID, _ := seedStore(t, tx)
	up := media.NewFakeUploader()
	up.Register(media.Attrs{StorageKey: "kd", Size: 1, ContentType: "image/jpeg"})
	svc := buildService(tx, up)
	agg := seedProductForMedia(t, svc, storeID, tenantID)
	m, err := svc.AddMedia(context.Background(), product.AddMediaRequest{
		ProductID: agg.Product.ID, StoreID: storeID, TenantID: tenantID,
		StorageKey: "kd", URL: "https://cdn/d.jpg",
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := svc.DeleteMedia(context.Background(), agg.Product.ID, m.ID, storeID, tenantID); err != nil {
		t.Fatalf("delete: %v", err)
	}
	fresh, _ := svc.Get(context.Background(), agg.Product.ID, storeID, tenantID)
	for _, md := range fresh.Media {
		if md.ID == m.ID {
			t.Fatal("expected media absent")
		}
	}
}

func TestIntegration_Service_DeleteMedia_CrossTenant_NotFound(t *testing.T) {
	tx := testdb.NewTx(t)
	storeID, tenantID, _ := seedStore(t, tx)
	up := media.NewFakeUploader()
	up.Register(media.Attrs{StorageKey: "kx", Size: 1, ContentType: "image/jpeg"})
	svc := buildService(tx, up)
	agg := seedProductForMedia(t, svc, storeID, tenantID)
	m, err := svc.AddMedia(context.Background(), product.AddMediaRequest{
		ProductID: agg.Product.ID, StoreID: storeID, TenantID: tenantID,
		StorageKey: "kx", URL: "https://cdn/x.jpg",
	})
	if err != nil {
		t.Fatal(err)
	}
	err = svc.DeleteMedia(context.Background(), agg.Product.ID, m.ID, storeID, uuid.NewString())
	var ae *apperrors.Error
	if !errors.As(err, &ae) || ae.Code != apperrors.CodeNotFound {
		t.Fatalf("expected NotFound; got %v", err)
	}
}

// A recrop commits by PATCHing storage_key; url must follow it, or the
// shop keeps showing the uncropped image. A later full-product save
// sends the stale url back and must not undo it either.
func TestIntegration_Service_RecropStorageKey_MovesURL(t *testing.T) {
	tx := testdb.NewTx(t)
	storeID, tenantID, _ := seedStore(t, tx)
	up := media.NewFakeUploader()
	up.Register(media.Attrs{StorageKey: "orig.png", Size: 1, ContentType: "image/png"})
	up.Register(media.Attrs{StorageKey: "cropped.png", Size: 1, ContentType: "image/png"})
	svc := product.NewService(product.Config{
		DB:                 tx,
		Repo:               product.NewRepository(tx),
		StoresRepo:         stores.NewRepository(tx),
		OutboxRepo:         outbox.NewRepository(tx),
		Uploader:           up,
		VendorLookup:       vendor.NewService(vendor.NewRepository(tx)),
		MediaPublicBaseURL: "https://cdn.test",
	})
	ctx := context.Background()
	agg := seedProductForMedia(t, svc, storeID, tenantID)
	m, err := svc.AddMedia(ctx, product.AddMediaRequest{
		ProductID: agg.Product.ID, StoreID: storeID, TenantID: tenantID, StorageKey: "orig.png",
	})
	if err != nil {
		t.Fatal(err)
	}
	urlOf := func() string {
		fresh, _ := svc.Get(ctx, agg.Product.ID, storeID, tenantID)
		for _, md := range fresh.Media {
			if md.ID == m.ID {
				return md.URL
			}
		}
		t.Fatal("media not found")
		return ""
	}

	missing := "never-uploaded.png"
	err = svc.UpdateMedia(ctx, product.UpdateMediaRequest{
		ProductID: agg.Product.ID, MediaID: m.ID, StoreID: storeID, TenantID: tenantID, StorageKey: &missing,
	})
	if !errors.Is(err, apperrors.ErrUploadNotFound) {
		t.Fatalf("missing upload: err = %v, want upload not found", err)
	}

	key := "cropped.png"
	if err := svc.UpdateMedia(ctx, product.UpdateMediaRequest{
		ProductID: agg.Product.ID, MediaID: m.ID, StoreID: storeID, TenantID: tenantID, StorageKey: &key,
	}); err != nil {
		t.Fatalf("update media: %v", err)
	}
	if got := urlOf(); got != "https://cdn.test/cropped.png" {
		t.Fatalf("after recrop url = %q", got)
	}

	if err := svc.ReplaceMedia(ctx, agg.Product.ID, storeID, tenantID, []product.MediaInput{{
		StorageKey: "cropped.png", URL: "https://cdn.test/orig.png",
	}}); err != nil {
		t.Fatalf("replace media: %v", err)
	}
	if got := urlOf(); got != "https://cdn.test/cropped.png" {
		t.Fatalf("after product save url = %q", got)
	}
}
