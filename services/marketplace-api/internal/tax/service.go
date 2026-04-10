package tax

import (
	"context"
	"fmt"

	"github.com/shopspring/decimal"
)

// Service provides a thin orchestration layer around any Calculator. It
// validates inputs, delegates to the calculator, and sanity-checks the result.
type Service struct{}

// NewService returns a stateless tax service.
func NewService() *Service { return &Service{} }

// CalculateOrderTax builds a TaxRequest from the supplied parameters, delegates
// to the provided Calculator, and validates the result before returning.
func (s *Service) CalculateOrderTax(
	ctx context.Context,
	storeCountryCode string,
	sellerAddress Address,
	buyerAddress Address,
	items []TaxableItem,
	shippingAmount decimal.Decimal,
	currency string,
	calculator Calculator,
) (*TaxBreakdown, error) {
	if calculator == nil {
		return nil, fmt.Errorf("tax: calculator must not be nil")
	}
	if storeCountryCode == "" {
		return nil, fmt.Errorf("tax: store country code is required")
	}
	if currency == "" {
		return nil, fmt.Errorf("tax: currency code is required")
	}

	req := TaxRequest{
		StoreCountryCode: storeCountryCode,
		SellerAddress:    sellerAddress,
		BuyerAddress:     buyerAddress,
		Items:            items,
		ShippingAmount:   shippingAmount,
		CurrencyCode:     currency,
	}

	breakdown, err := calculator.Calculate(ctx, req)
	if err != nil {
		return nil, fmt.Errorf("tax: %s: %w", calculator.ProviderName(), err)
	}

	if err := s.validateBreakdown(breakdown); err != nil {
		return nil, fmt.Errorf("tax: invalid breakdown from %s: %w", calculator.ProviderName(), err)
	}

	return breakdown, nil
}

// validateBreakdown sanity-checks the calculator output.
func (s *Service) validateBreakdown(bd *TaxBreakdown) error {
	if bd == nil {
		return fmt.Errorf("breakdown is nil")
	}
	if bd.TaxTotal.LessThan(decimal.Zero) {
		return fmt.Errorf("tax total is negative: %s", bd.TaxTotal)
	}

	lineSum := decimal.Zero
	for _, line := range bd.Lines {
		if line.Amount.LessThan(decimal.Zero) {
			return fmt.Errorf("tax line %q has negative amount: %s", line.Description, line.Amount)
		}
		lineSum = lineSum.Add(line.Amount)
	}

	// Allow rounding tolerance that scales with the number of tax lines.
	// Each line can contribute up to 0.005 of rounding, so we allow
	// 0.01 per line with a minimum of 0.01.
	diff := bd.TaxTotal.Sub(lineSum).Abs()
	lineCount := int64(len(bd.Lines))
	if lineCount < 1 {
		lineCount = 1
	}
	tolerance := decimal.NewFromFloat(0.01).Mul(decimal.NewFromInt(lineCount))
	if diff.GreaterThan(tolerance) {
		return fmt.Errorf("tax total %s does not match sum of lines %s (diff %s)", bd.TaxTotal, lineSum, diff)
	}

	return nil
}
