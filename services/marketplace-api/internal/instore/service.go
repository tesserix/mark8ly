package instore

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/google/uuid"
	"github.com/shopspring/decimal"
	"gorm.io/gorm"

	"github.com/mark8ly/marketplace-api/internal/checkoutcore"
	"github.com/mark8ly/marketplace-api/internal/country"
	"github.com/mark8ly/marketplace-api/internal/order"
	"github.com/mark8ly/marketplace-api/internal/stockhold"
	"github.com/mark8ly/marketplace-api/internal/tax"
	"github.com/mark8ly/marketplace-api/pkg/apperrors"
)

const (
	maxQuantity    = 999
	maxCustomTitle = 200
	maxMethodLabel = 40
)

// Service records in-store sales.
type Service struct {
	db        *gorm.DB
	orders    *order.Service
	holds     *stockhold.Repository
	pricer    checkoutcore.CatalogPricer
	details   DetailsLookup
	taxes     TaxQuoter
	countries func(ctx context.Context, code string) (*country.SupportedCountry, error)
}

// NewService wires the service to the database. taxes is the storefront
// checkout handler (see TaxQuoter).
func NewService(db *gorm.DB, orders *order.Service, holds *stockhold.Repository, taxes TaxQuoter) *Service {
	return &Service{
		db:      db,
		orders:  orders,
		holds:   holds,
		pricer:  checkoutcore.NewDBCatalogPricer(db),
		details: dbDetails{db: db},
		taxes:   taxes,
		countries: func(ctx context.Context, code string) (*country.SupportedCountry, error) {
			var sc country.SupportedCountry
			if err := db.WithContext(ctx).
				Where("country_code = ? AND is_active = true", code).
				First(&sc).Error; err != nil {
				return nil, apperrors.ValidationFailed("country", "store country not supported")
			}
			return &sc, nil
		},
	}
}

// RecordSale validates, prices and taxes a sale, then creates it as a paid
// order whose stock is taken in the same transaction. A retry with the same
// IdempotencyKey returns the first order (CreateResult.Reused). A shortage
// returns checkoutcore.OutOfStockError and records nothing.
func (s *Service) RecordSale(ctx context.Context, in SaleInput) (*order.CreateResult, error) {
	if err := validateShape(in); err != nil {
		return nil, err
	}

	items, stockLines, taxLines, subtotal, err := s.priceLines(ctx, in)
	if err != nil {
		return nil, err
	}
	if in.Discount.IsNegative() || in.Discount.GreaterThan(subtotal) {
		return nil, apperrors.ValidationFailed("discount", "discount must be between 0 and the subtotal")
	}

	sc, err := s.countries(ctx, in.Store.CountryCode)
	if err != nil {
		return nil, err
	}
	breakdown, err := s.taxes.QuoteCounterTax(ctx, in.Store, sc, checkoutcore.TaxableItems(sc, taxLines))
	if err != nil {
		return nil, fmt.Errorf("instore: tax: %w", err)
	}
	// Same arithmetic as online checkout: tax on the undiscounted lines,
	// the discount taken off the total.
	grand := subtotal.Add(breakdown.TaxTotal).Sub(in.Discount)
	if in.PaymentMethod == PaymentCash && (in.CashTendered == nil || in.CashTendered.LessThan(grand)) {
		return nil, apperrors.ValidationFailed("cash_tendered", "cash tendered must cover the total")
	}

	paid := order.PaymentStatusPaid
	create := order.CreateInput{
		TenantID:       in.TenantID,
		StoreID:        in.StoreID,
		StorePrefix:    order.StorePrefix(in.Store.Slug),
		IdempotencyKey: in.IdempotencyKey,
		CustomerEmail:  strings.TrimSpace(in.CustomerEmail),
		Items:          items,
		Subtotal:       subtotal,
		TaxTotal:       breakdown.TaxTotal,
		DiscountTotal:  in.Discount,
		GrandTotal:     grand,
		CurrencyCode:   in.Store.CurrencyCode,
		Notes:          in.Note,
		InStore: &order.InStoreDetails{
			PaymentMethod:      in.PaymentMethod,
			PaymentMethodLabel: in.PaymentMethodLabel,
			SoldByUserID:       in.SoldByUserID,
			CashTendered:       in.CashTendered,
		},
		WithinTx: func(tx *gorm.DB, o *order.Order) error {
			if len(stockLines) > 0 {
				// Holds are keyed by a uuid cart token; derive a stable one
				// from the sale so it never collides with a shopper's cart.
				token := uuid.NewSHA1(uuid.NameSpaceOID, []byte("instore:"+in.StoreID.String()+":"+in.IdempotencyKey)).String()
				if err := checkoutcore.CommitStock(ctx, tx, s.holds, token,
					o.ID.String(), in.StoreID.String(), stockLines); err != nil {
					return err
				}
			}
			if err := s.orders.Confirm(ctx, tx, o.ID, &paid, "in-store sale"); err != nil {
				return err
			}
			if in.Fulfilled {
				return s.orders.MarkFulfilled(ctx, tx, o.ID)
			}
			return nil
		},
	}
	res, err := s.orders.Create(ctx, create)
	if err != nil {
		return nil, err
	}
	if !res.Reused && len(breakdown.Lines) > 0 {
		// Best effort, as at checkout: tax_total is already on the order.
		_ = tax.NewRepository().SaveTaxLines(ctx, s.db, res.Order.ID, breakdown.Lines)
	}
	return res, nil
}

func validateShape(in SaleInput) error {
	if in.Store == nil {
		return apperrors.ValidationFailed("store", "store is required")
	}
	if strings.TrimSpace(in.IdempotencyKey) == "" {
		return apperrors.ValidationFailed("idempotency_key", "idempotency_key is required")
	}
	if len(in.Lines) == 0 {
		return apperrors.ValidationFailed("lines", "add at least one item")
	}
	for i, l := range in.Lines {
		if l.Quantity < 1 || l.Quantity > maxQuantity {
			return apperrors.ValidationFailed("lines", fmt.Sprintf("line %d: quantity must be 1 to %d", i+1, maxQuantity))
		}
		custom := l.CustomTitle != nil || l.CustomPrice != nil
		switch {
		case custom && l.VariantID != nil:
			return apperrors.ValidationFailed("lines", fmt.Sprintf("line %d: a line is either a product or a custom item", i+1))
		case custom:
			if !in.CanDiscount {
				return apperrors.ValidationFailed("lines", "you are not allowed to add custom items")
			}
			title := ""
			if l.CustomTitle != nil {
				title = strings.TrimSpace(*l.CustomTitle)
			}
			if title == "" || len(title) > maxCustomTitle {
				return apperrors.ValidationFailed("lines", fmt.Sprintf("line %d: custom item needs a name up to %d characters", i+1, maxCustomTitle))
			}
			if l.CustomPrice == nil || !l.CustomPrice.IsPositive() {
				return apperrors.ValidationFailed("lines", fmt.Sprintf("line %d: custom item needs a price above 0", i+1))
			}
		case l.VariantID == nil || *l.VariantID == "":
			return apperrors.ValidationFailed("lines", fmt.Sprintf("line %d: no product chosen", i+1))
		}
	}
	if !in.Discount.IsZero() && !in.CanDiscount {
		return apperrors.ValidationFailed("discount", "you are not allowed to give discounts")
	}
	switch in.PaymentMethod {
	case PaymentCard, PaymentCash, PaymentTransfer:
	case PaymentOther:
		if in.PaymentMethodLabel == nil || strings.TrimSpace(*in.PaymentMethodLabel) == "" ||
			len(*in.PaymentMethodLabel) > maxMethodLabel {
			return apperrors.ValidationFailed("payment_method_label", "name the payment method, up to 40 characters")
		}
	default:
		return apperrors.ValidationFailed("payment_method", "unknown payment method")
	}
	if in.SoldByUserID == "" {
		return apperrors.ValidationFailed("sold_by", "seller is required")
	}
	return nil
}

// priceLines prices catalogue lines from the catalogue and snapshots them.
func (s *Service) priceLines(ctx context.Context, in SaleInput) (
	items []order.OrderItem, stockLines []checkoutcore.Line, taxLines []checkoutcore.TaxLine,
	subtotal decimal.Decimal, err error,
) {
	storeID := in.StoreID.String()
	var ids []string
	for _, l := range in.Lines {
		if l.VariantID != nil {
			ids = append(ids, strings.ToLower(*l.VariantID))
		}
	}
	details := map[string]VariantDetails{}
	if len(ids) > 0 {
		if details, err = s.details.Details(ctx, storeID, ids); err != nil {
			return nil, nil, nil, decimal.Zero, err
		}
	}

	for i, l := range in.Lines {
		qty := decimal.NewFromInt(int64(l.Quantity))
		if l.VariantID == nil {
			line := l.CustomPrice.Round(2).Mul(qty)
			items = append(items, order.OrderItem{
				TitleSnapshot: strings.TrimSpace(*l.CustomTitle),
				SKUSnapshot:   "",
				UnitPrice:     l.CustomPrice.Round(2),
				Quantity:      l.Quantity,
				LineTotal:     line,
				CurrencyCode:  in.Store.CurrencyCode,
			})
			taxLines = append(taxLines, checkoutcore.TaxLine{UnitPrice: l.CustomPrice.Round(2), Quantity: l.Quantity})
			subtotal = subtotal.Add(line)
			continue
		}

		vid := strings.ToLower(*l.VariantID)
		d, ok := details[vid]
		if !ok {
			return nil, nil, nil, decimal.Zero, apperrors.ValidationFailed("lines",
				fmt.Sprintf("line %d: this product is not for sale in this store", i+1))
		}
		price, perr := s.pricer.PriceFor(ctx, storeID, vid)
		if errors.Is(perr, checkoutcore.ErrCatalogItemNotFound) {
			return nil, nil, nil, decimal.Zero, apperrors.ValidationFailed("lines",
				fmt.Sprintf("line %d: this product is not for sale in this store", i+1))
		}
		if perr != nil {
			return nil, nil, nil, decimal.Zero, perr
		}

		line := price.Amount.Mul(qty)
		item := order.OrderItem{
			TitleSnapshot: d.Title,
			SKUSnapshot:   d.SKU,
			OptionSummary: d.OptionSummary,
			UnitPrice:     price.Amount,
			Quantity:      l.Quantity,
			LineTotal:     line,
			CurrencyCode:  price.CurrencyCode,
			ImageURL:      d.ImageURL,
		}
		if pid, perr := uuid.Parse(d.ProductID); perr == nil {
			item.ProductID = &pid
		}
		if v, perr := uuid.Parse(vid); perr == nil {
			item.VariantID = &v
		}
		items = append(items, item)
		stockLines = append(stockLines, checkoutcore.Line{VariantID: vid, Quantity: l.Quantity})
		productID := d.ProductID
		taxLines = append(taxLines, checkoutcore.TaxLine{
			SKU: d.SKU, ProductID: &productID, UnitPrice: price.Amount, Quantity: l.Quantity,
			TaxCode: price.TaxCode, TaxCategory: price.TaxCategory, TaxRateOverride: price.TaxRateOverride,
		})
		subtotal = subtotal.Add(line)
	}
	return items, stockLines, taxLines, subtotal, nil
}

// dbDetails reads active, published products only: a draft or archived
// product cannot be rung up.
type dbDetails struct{ db *gorm.DB }

func (d dbDetails) Details(ctx context.Context, storeID string, variantIDs []string) (map[string]VariantDetails, error) {
	var rows []struct {
		ID            string
		ProductID     string
		Title         string
		SKU           string
		OptionSummary *string
		ImageURL      *string
	}
	err := d.db.WithContext(ctx).Raw(`
		SELECT v.id::text AS id, v.product_id::text AS product_id, p.title, v.sku,
		       (SELECT string_agg(pov.value, ' / ' ORDER BY po.position, pov.position)
		          FROM variant_option_values vov
		          JOIN product_option_values pov ON pov.id = vov.option_value_id
		          JOIN product_options po ON po.id = pov.option_id
		         WHERE vov.variant_id = v.id) AS option_summary,
		       (SELECT pm.url FROM product_media pm
		         WHERE pm.product_id = p.id ORDER BY pm.position, pm.id LIMIT 1) AS image_url
		  FROM product_variants v
		  JOIN products p ON p.id = v.product_id AND p.deleted_at IS NULL
		 WHERE v.store_id = ? AND v.id::text IN ? AND v.deleted_at IS NULL
		   AND p.status = 'active' AND p.published_at IS NOT NULL AND p.published_at <= now()`,
		storeID, variantIDs).Scan(&rows).Error
	if err != nil {
		return nil, fmt.Errorf("instore: load variants: %w", err)
	}
	out := make(map[string]VariantDetails, len(rows))
	for _, r := range rows {
		out[r.ID] = VariantDetails{ProductID: r.ProductID, Title: r.Title, SKU: r.SKU,
			OptionSummary: r.OptionSummary, ImageURL: r.ImageURL}
	}
	return out, nil
}
