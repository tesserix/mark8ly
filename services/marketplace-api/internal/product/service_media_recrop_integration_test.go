//go:build integration

package product_test

import (
	"context"
	"testing"
	"time"

	"github.com/mark8ly/marketplace-api/internal/media"
	"github.com/mark8ly/marketplace-api/internal/product"
	"github.com/mark8ly/marketplace-api/pkg/testdb"
)

// capturingUploader is a FakeUploader that also implements the signer
// interfaces so RecropMedia runs its full path and records the
// content-type handed to the PUT signer.
type capturingUploader struct {
	*media.FakeUploader
	gotPutContentType string
}

func (c *capturingUploader) SignedUploadURL(_ context.Context, key, contentType string, _ time.Duration) (string, time.Time, error) {
	c.gotPutContentType = contentType
	return "https://put.example/" + key, time.Now().Add(time.Minute), nil
}

func (c *capturingUploader) SignedReadURL(_ context.Context, key string, _ time.Duration) (string, time.Time, error) {
	return "https://get.example/" + key, time.Now().Add(time.Minute), nil
}

func seedMediaRow(t *testing.T, svc *product.Service, agg *product.Aggregate, storeID, tenantID, key string) *product.Media {
	t.Helper()
	row, err := svc.AddMedia(context.Background(), product.AddMediaRequest{
		ProductID:  agg.Product.ID,
		StoreID:    storeID,
		TenantID:   tenantID,
		StorageKey: key,
		URL:        "https://cdn/" + key,
	})
	if err != nil {
		t.Fatalf("seed media: %v", err)
	}
	return row
}

func TestIntegration_RecropMedia_SignsWithGivenContentType(t *testing.T) {
	tx := testdb.NewTx(t)
	storeID, tenantID := seedStore(t, tx)
	up := &capturingUploader{FakeUploader: media.NewFakeUploader()}
	up.Register(media.Attrs{StorageKey: "orig-webp", Size: 10, ContentType: "image/webp"})
	svc := buildService(tx, up)
	agg := seedProductForMedia(t, svc, storeID, tenantID)
	row := seedMediaRow(t, svc, agg, storeID, tenantID, "orig-webp")

	_, err := svc.RecropMedia(context.Background(), product.RecropMediaRequest{
		ProductID:   agg.Product.ID,
		MediaID:     row.ID,
		StoreID:     storeID,
		TenantID:    tenantID,
		ContentType: "image/webp",
	}, up, up, time.Minute)
	if err != nil {
		t.Fatalf("recrop: %v", err)
	}
	if up.gotPutContentType != "image/webp" {
		t.Fatalf("put content-type = %q, want image/webp", up.gotPutContentType)
	}
}

func TestIntegration_RecropMedia_DefaultsContentTypeToJPEG(t *testing.T) {
	tx := testdb.NewTx(t)
	storeID, tenantID := seedStore(t, tx)
	up := &capturingUploader{FakeUploader: media.NewFakeUploader()}
	up.Register(media.Attrs{StorageKey: "orig-jpg", Size: 10, ContentType: "image/jpeg"})
	svc := buildService(tx, up)
	agg := seedProductForMedia(t, svc, storeID, tenantID)
	row := seedMediaRow(t, svc, agg, storeID, tenantID, "orig-jpg")

	_, err := svc.RecropMedia(context.Background(), product.RecropMediaRequest{
		ProductID: agg.Product.ID,
		MediaID:   row.ID,
		StoreID:   storeID,
		TenantID:  tenantID,
		// ContentType intentionally empty.
	}, up, up, time.Minute)
	if err != nil {
		t.Fatalf("recrop: %v", err)
	}
	if up.gotPutContentType != "image/jpeg" {
		t.Fatalf("put content-type = %q, want image/jpeg default", up.gotPutContentType)
	}
}
