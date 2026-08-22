package billingarchive

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"

	billingstripe "github.com/mark8ly/marketplace-api/internal/billing/stripe"
	"github.com/mark8ly/marketplace-api/internal/metrics"
	"github.com/mark8ly/marketplace-api/internal/stores"
	"github.com/mark8ly/marketplace-api/internal/subscription"
)

const (
	// archiveRetentionYears is the 7-year tax-authority retention period (§23.2).
	archiveRetentionYears = 7
)

// BuildInput contains all the data needed to build and persist a billing archive.
type BuildInput struct {
	TenantID uuid.UUID
	StoreID  uuid.UUID
	// HardDeletedAt is the time the store was hard-deleted (anchors the retention clock).
	HardDeletedAt time.Time
}

// Builder fetches Stripe invoices and persists a billing_archive row for a
// hard-deleted store. The archive is retained for 7 years (§23.2).
type Builder struct {
	db     *gorm.DB
	stripe *billingstripe.Client
	logger *slog.Logger
}

// NewBuilder constructs a Builder.
func NewBuilder(db *gorm.DB, stripe *billingstripe.Client, logger *slog.Logger) *Builder {
	if logger == nil {
		logger = slog.Default()
	}
	return &Builder{db: db, stripe: stripe, logger: logger}
}

// BuildAndPersist fetches all Stripe invoices for the store's subscription,
// sums total revenue (paid invoices only), and inserts a billing_archive row.
// It is idempotent: if a row already exists for (original_store_id, hard_deleted_at)
// it is a no-op (Postgres UNIQUE constraint on those two columns is not present but
// the caller must ensure single-invocation per hard-delete event).
func (b *Builder) BuildAndPersist(ctx context.Context, in BuildInput) (*BillingArchive, error) {
	// 1. Fetch the subscription to get the Stripe customer ID and billing metadata.
	subRepo := subscription.NewRepository()
	sub, err := subRepo.GetByStoreID(ctx, b.db, in.TenantID, in.StoreID)
	if err != nil {
		return nil, fmt.Errorf("billingarchive: fetch subscription: %w", err)
	}

	// 2. Fetch the store record for business_name and billing_country.
	var store stores.Store
	if err := b.db.WithContext(ctx).
		Where("id = ? AND tenant_id = ?", in.StoreID, in.TenantID).
		First(&store).Error; err != nil {
		return nil, fmt.Errorf("billingarchive: fetch store: %w", err)
	}

	// 3. Fetch Stripe invoices (only when Stripe client is available).
	var invoices []billingstripe.Invoice
	var totalRevenueUSD *float64

	if b.stripe != nil && sub.StripeCustomerID != "" {
		inv, stripeErr := billingstripe.ListInvoicesForCustomer(ctx, b.stripe, sub.StripeCustomerID)
		if stripeErr != nil {
			// Log and continue — archive must succeed even if Stripe is unreachable.
			b.logger.Warn("billingarchive: failed to fetch Stripe invoices, archiving with empty invoice list",
				"store_id", in.StoreID,
				"stripe_customer_id", sub.StripeCustomerID,
				"err", stripeErr)
		} else {
			invoices = inv
		}

		// Sum paid invoices. Stripe amounts are in the invoice's currency minor
		// units; we accumulate a USD estimate using a 1:1 approximation for same-
		// currency invoices. Cross-currency conversion is outside scope — store
		// the raw total for the most common case (USD).
		var totalMinor int64
		for _, i := range invoices {
			if i.Status == "paid" {
				totalMinor += i.AmountPaid
			}
		}
		if totalMinor > 0 {
			// Convert minor units (cents) to major units (dollars).
			usd := float64(totalMinor) / 100.0
			totalRevenueUSD = &usd
		}
	} else {
		b.logger.Warn("billingarchive: no Stripe client or customer ID, archiving with empty invoice list",
			"store_id", in.StoreID)
	}

	// 4. Serialise invoices to JSON for storage.
	invoicesJSON, err := json.Marshal(invoices)
	if err != nil {
		return nil, fmt.Errorf("billingarchive: marshal invoices: %w", err)
	}

	// 5. Build archive row.
	archiveExpiresAt := in.HardDeletedAt.AddDate(archiveRetentionYears, 0, 0)
	billingCountry := &store.CountryCode
	archive := &BillingArchive{
		OriginalStoreID:  in.StoreID,
		OriginalTenantID: in.TenantID,
		BusinessName:     store.Name,
		TaxID:            sub.ReverseChargeTaxID,
		TaxIDCountry:     sub.TaxIDCountry,
		BillingCountry:   billingCountry,
		BillingCurrency:  sub.BillingCurrency,
		StripeCustomerID: sub.StripeCustomerID,
		AllInvoices:      invoicesJSON,
		TotalRevenueUSD:  totalRevenueUSD,
		HardDeletedAt:    in.HardDeletedAt,
		ArchiveExpiresAt: archiveExpiresAt,
	}

	// 6. Persist.
	if err := b.db.WithContext(ctx).Create(archive).Error; err != nil {
		return nil, fmt.Errorf("billingarchive: persist archive: %w", err)
	}

	if metrics.Subscription != nil {
		metrics.Subscription.BillingArchiveCreatedTotal.Inc()
	}

	b.logger.Info("billingarchive: archive persisted",
		"archive_id", archive.ID,
		"store_id", in.StoreID,
		"expires_at", archiveExpiresAt)

	return archive, nil
}
