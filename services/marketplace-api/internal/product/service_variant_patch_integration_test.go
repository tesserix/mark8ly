//go:build integration

package product_test

import (
	"context"
	"errors"
	"testing"

	"github.com/google/uuid"
	"github.com/shopspring/decimal"

	"github.com/mark8ly/marketplace-api/internal/media"
	"github.com/mark8ly/marketplace-api/internal/product"
	"github.com/mark8ly/marketplace-api/pkg/apperrors"
	"github.com/mark8ly/marketplace-api/pkg/testdb"
)

func seedProductWithTwoVariants(t *testing.T, svc *product.Service, storeID, tenantID string) *product.Aggregate {
	t.Helper()
	agg, err := svc.Create(context.Background(), product.CreateRequest{
		StoreID:  storeID,
		TenantID: tenantID,
		Title:    "Tee " + uuid.NewString()[:6],
		Options: []product.OptionSpec{{
			Name:   "Size",
			Values: []product.OptionValueSpec{{Value: "S"}, {Value: "M"}},
		}},
		Variants: []product.VariantInput{
			{
				SKU: "TEE-S-" + uuid.NewString()[:6], Price: decimal.NewFromInt(10), CurrencyCode: "USD",
				OptionValues: []product.OptionValueRef{{OptionName: "Size", Value: "S"}},
			},
			{
				SKU: "TEE-M-" + uuid.NewString()[:6], Price: decimal.NewFromInt(10), CurrencyCode: "USD",
				OptionValues: []product.OptionValueRef{{OptionName: "Size", Value: "M"}},
			},
		},
	})
	if err != nil {
		t.Fatalf("seed two variants: %v", err)
	}
	return agg
}

func TestIntegration_Service_UpdateVariantBasics_Price(t *testing.T) {
	tx := testdb.NewTx(t)
	storeID, tenantID := seedStore(t, tx)
	svc := buildService(tx, media.NewFakeUploader())
	agg := seedProductWithTwoVariants(t, svc, storeID, tenantID)
	v := agg.Variants[0]

	price := decimal.NewFromFloat(42.50)
	got, err := svc.UpdateVariantBasics(context.Background(), product.UpdateVariantBasicsRequest{
		ProductID: agg.Product.ID, VariantID: v.ID, StoreID: storeID, TenantID: tenantID,
		Price: &price,
	})
	if err != nil {
		t.Fatalf("update: %v", err)
	}
	if !got.Price.Equal(price) {
		t.Fatalf("price = %s, want %s", got.Price, price)
	}
}

func TestIntegration_Service_UpdateVariantBasics_Inventory_TriggerSyncs(t *testing.T) {
	tx := testdb.NewTx(t)
	storeID, tenantID := seedStore(t, tx)
	svc := buildService(tx, media.NewFakeUploader())
	agg := seedProductWithTwoVariants(t, svc, storeID, tenantID)
	v := agg.Variants[0]

	qty := 17
	_, err := svc.UpdateVariantBasics(context.Background(), product.UpdateVariantBasicsRequest{
		ProductID: agg.Product.ID, VariantID: v.ID, StoreID: storeID, TenantID: tenantID,
		InventoryQuantity: &qty,
	})
	if err != nil {
		t.Fatalf("update: %v", err)
	}
	fresh, err := svc.Get(context.Background(), agg.Product.ID, storeID, tenantID)
	if err != nil {
		t.Fatal(err)
	}
	for _, fv := range fresh.Variants {
		if fv.ID == v.ID {
			if fv.InventoryQuantity != 17 {
				t.Fatalf("inventory_quantity = %d, want 17", fv.InventoryQuantity)
			}
			return
		}
	}
	t.Fatal("variant not found")
}

func TestIntegration_Service_UpdateVariantBasics_SKUCollision(t *testing.T) {
	tx := testdb.NewTx(t)
	storeID, tenantID := seedStore(t, tx)
	svc := buildService(tx, media.NewFakeUploader())
	agg := seedProductWithTwoVariants(t, svc, storeID, tenantID)
	firstSKU := agg.Variants[0].SKU
	second := agg.Variants[1]

	_, err := svc.UpdateVariantBasics(context.Background(), product.UpdateVariantBasicsRequest{
		ProductID: agg.Product.ID, VariantID: second.ID, StoreID: storeID, TenantID: tenantID,
		SKU: &firstSKU,
	})
	var ae *apperrors.Error
	if !errors.As(err, &ae) || ae.Code != apperrors.CodeSKUTaken {
		t.Fatalf("expected SKUTaken; got %v", err)
	}
}

func TestIntegration_Service_UpdateVariantBasics_CurrencyChange_Forbidden(t *testing.T) {
	tx := testdb.NewTx(t)
	storeID, tenantID := seedStore(t, tx)
	svc := buildService(tx, media.NewFakeUploader())
	agg := seedProductWithTwoVariants(t, svc, storeID, tenantID)
	v := agg.Variants[0]

	cc := "EUR"
	_, err := svc.UpdateVariantBasics(context.Background(), product.UpdateVariantBasicsRequest{
		ProductID: agg.Product.ID, VariantID: v.ID, StoreID: storeID, TenantID: tenantID,
		CurrencyCode: &cc,
	})
	var ae *apperrors.Error
	if !errors.As(err, &ae) || ae.Code != apperrors.CodeCurrencyChangeForbidden {
		t.Fatalf("expected CurrencyChangeForbidden; got %v", err)
	}
}

func TestIntegration_Service_UpdateVariantBasics_NotFound(t *testing.T) {
	tx := testdb.NewTx(t)
	storeID, tenantID := seedStore(t, tx)
	svc := buildService(tx, media.NewFakeUploader())
	agg := seedProductWithTwoVariants(t, svc, storeID, tenantID)

	price := decimal.NewFromInt(1)
	_, err := svc.UpdateVariantBasics(context.Background(), product.UpdateVariantBasicsRequest{
		ProductID: agg.Product.ID, VariantID: uuid.NewString(), StoreID: storeID, TenantID: tenantID,
		Price: &price,
	})
	var ae *apperrors.Error
	if !errors.As(err, &ae) || ae.Code != apperrors.CodeNotFound {
		t.Fatalf("expected NotFound; got %v", err)
	}
}
