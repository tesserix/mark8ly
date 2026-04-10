package tax

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"

	"github.com/shopspring/decimal"
)

const (
	taxjarLiveURL    = "https://api.taxjar.com"
	taxjarSandboxURL = "https://api.sandbox.taxjar.com"
	taxjarTimeout    = 10 * time.Second
)

// TaxJarCalculator calls the TaxJar REST API to compute US sales tax. It uses
// net/http directly (no external SDK). Set mode to "sandbox" for testing; any
// other value hits the production endpoint.
type TaxJarCalculator struct {
	apiKey  string
	mode    string
	baseURL string
	client  *http.Client
}

// NewTaxJarCalculator creates a TaxJar calculator. mode "sandbox" routes
// requests to api.sandbox.taxjar.com; anything else uses the production URL.
func NewTaxJarCalculator(apiKey, mode string) *TaxJarCalculator {
	base := taxjarLiveURL
	if mode == "sandbox" {
		base = taxjarSandboxURL
	}
	return &TaxJarCalculator{
		apiKey:  apiKey,
		mode:    mode,
		baseURL: base,
		client:  &http.Client{Timeout: taxjarTimeout},
	}
}

func (c *TaxJarCalculator) ProviderName() string { return "taxjar" }

// ── TaxJar request / response shapes ────────────────────────────────────────

type taxjarRequest struct {
	FromCountry string            `json:"from_country"`
	FromZip     string            `json:"from_zip"`
	FromState   string            `json:"from_state"`
	FromCity    string            `json:"from_city"`
	ToCountry   string            `json:"to_country"`
	ToZip       string            `json:"to_zip"`
	ToState     string            `json:"to_state"`
	ToCity      string            `json:"to_city"`
	Shipping    float64           `json:"shipping"`
	LineItems   []taxjarLineItem  `json:"line_items"`
}

type taxjarLineItem struct {
	ID             string  `json:"id"`
	Quantity       int     `json:"quantity"`
	UnitPrice      float64 `json:"unit_price"`
	ProductTaxCode string  `json:"product_tax_code,omitempty"`
}

type taxjarResponse struct {
	Tax taxjarTax `json:"tax"`
}

type taxjarTax struct {
	AmountToCollect float64              `json:"amount_to_collect"`
	Shipping        taxjarShippingTax    `json:"shipping"`
	Breakdown       taxjarBreakdown      `json:"breakdown"`
}

type taxjarShippingTax struct {
	TaxableAmount   float64 `json:"taxable_amount"`
	TaxCollectable  float64 `json:"tax_collectable"`
	CombinedTaxRate float64 `json:"combined_tax_rate"`
}

type taxjarBreakdown struct {
	TaxableAmount       float64                    `json:"taxable_amount"`
	TaxCollectable      float64                    `json:"tax_collectable"`
	CombinedTaxRate     float64                    `json:"combined_tax_rate"`
	StateTaxableAmount  float64                    `json:"state_taxable_amount"`
	StateTaxRate        float64                    `json:"state_tax_rate"`
	StateTaxCollectable float64                    `json:"state_amount"`
	CountyTaxableAmount float64                    `json:"county_taxable_amount"`
	CountyTaxRate       float64                    `json:"county_tax_rate"`
	CountyTaxCollectable float64                   `json:"county_amount"`
	CityTaxableAmount   float64                    `json:"city_taxable_amount"`
	CityTaxRate         float64                    `json:"city_tax_rate"`
	CityTaxCollectable  float64                    `json:"city_amount"`
	SpecialTaxRate      float64                    `json:"special_district_taxable_amount"`
	SpecialTaxCollectable float64                  `json:"special_tax_rate"`
	Shipping            *taxjarShippingBreakdown   `json:"shipping,omitempty"`
}

type taxjarShippingBreakdown struct {
	TaxableAmount       float64 `json:"taxable_amount"`
	TaxCollectable      float64 `json:"tax_collectable"`
	StateTaxRate        float64 `json:"state_sales_tax_rate"`
	StateAmount         float64 `json:"state_amount"`
	CountyTaxRate       float64 `json:"county_tax_rate"`
	CountyAmount        float64 `json:"county_amount"`
	CityTaxRate         float64 `json:"city_tax_rate"`
	CityAmount          float64 `json:"city_amount"`
	SpecialTaxRate      float64 `json:"special_district_tax_rate"`
	SpecialAmount       float64 `json:"special_tax_amount"`
}

type taxjarErrorResponse struct {
	Error  string `json:"error"`
	Detail string `json:"detail"`
	Status int    `json:"status"`
}

// ── Calculate ────────────────────────────────────────────────────────────────

func (c *TaxJarCalculator) Calculate(ctx context.Context, in TaxRequest) (*TaxBreakdown, error) {
	reqBody := c.buildRequest(in)

	bodyBytes, err := json.Marshal(reqBody)
	if err != nil {
		return nil, fmt.Errorf("taxjar: marshal request: %w", err)
	}

	url := c.baseURL + "/v2/taxes"
	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(bodyBytes))
	if err != nil {
		return nil, fmt.Errorf("taxjar: create request: %w", err)
	}
	httpReq.Header.Set("Authorization", "Bearer "+c.apiKey)
	httpReq.Header.Set("Content-Type", "application/json")

	resp, err := c.client.Do(httpReq)
	if err != nil {
		return nil, fmt.Errorf("taxjar: calculate: %w", err)
	}
	defer resp.Body.Close()

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("taxjar: read response: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		var apiErr taxjarErrorResponse
		_ = json.Unmarshal(respBody, &apiErr)
		return nil, fmt.Errorf("taxjar: calculate: status %d: %s — %s", resp.StatusCode, apiErr.Error, apiErr.Detail)
	}

	var taxResp taxjarResponse
	if err := json.Unmarshal(respBody, &taxResp); err != nil {
		return nil, fmt.Errorf("taxjar: unmarshal response: %w", err)
	}

	return c.parseBreakdown(taxResp, in), nil
}

func (c *TaxJarCalculator) buildRequest(in TaxRequest) taxjarRequest {
	lineItems := make([]taxjarLineItem, 0, len(in.Items))
	for _, item := range in.Items {
		unitPrice, _ := item.Amount.Float64()
		lineItems = append(lineItems, taxjarLineItem{
			ID:             item.ProductID,
			Quantity:       item.Quantity,
			UnitPrice:      unitPrice,
			ProductTaxCode: item.TaxCode,
		})
	}

	shipping, _ := in.ShippingAmount.Float64()

	return taxjarRequest{
		FromCountry: in.SellerAddress.CountryCode,
		FromZip:     in.SellerAddress.PostalCode,
		FromState:   in.SellerAddress.Region,
		FromCity:    in.SellerAddress.City,
		ToCountry:   in.BuyerAddress.CountryCode,
		ToZip:       in.BuyerAddress.PostalCode,
		ToState:     in.BuyerAddress.Region,
		ToCity:      in.BuyerAddress.City,
		Shipping:    shipping,
		LineItems:   lineItems,
	}
}

func (c *TaxJarCalculator) parseBreakdown(resp taxjarResponse, in TaxRequest) *TaxBreakdown {
	bd := resp.Tax.Breakdown
	jurisdiction := fmt.Sprintf("%s-%s", in.BuyerAddress.Region, in.BuyerAddress.City)

	var lines []TaxLine

	if bd.StateTaxCollectable > 0 {
		lines = append(lines, TaxLine{
			Description:  fmt.Sprintf("%s State Tax", in.BuyerAddress.Region),
			Rate:         decimal.NewFromFloat(bd.StateTaxRate),
			Amount:       decimal.NewFromFloat(bd.StateTaxCollectable),
			Jurisdiction: jurisdiction,
		})
	}
	if bd.CountyTaxCollectable > 0 {
		lines = append(lines, TaxLine{
			Description:  "County Tax",
			Rate:         decimal.NewFromFloat(bd.CountyTaxRate),
			Amount:       decimal.NewFromFloat(bd.CountyTaxCollectable),
			Jurisdiction: jurisdiction,
		})
	}
	if bd.CityTaxCollectable > 0 {
		lines = append(lines, TaxLine{
			Description:  fmt.Sprintf("%s City Tax", in.BuyerAddress.City),
			Rate:         decimal.NewFromFloat(bd.CityTaxRate),
			Amount:       decimal.NewFromFloat(bd.CityTaxCollectable),
			Jurisdiction: jurisdiction,
		})
	}

	// Shipping tax — include if TaxJar reports it.
	if bd.Shipping != nil && bd.Shipping.TaxCollectable > 0 {
		lines = append(lines, TaxLine{
			Description:  "Shipping Tax",
			Rate:         decimal.NewFromFloat(bd.Shipping.StateTaxRate + bd.Shipping.CountyTaxRate + bd.Shipping.CityTaxRate + bd.Shipping.SpecialTaxRate),
			Amount:       decimal.NewFromFloat(bd.Shipping.TaxCollectable),
			Jurisdiction: jurisdiction,
		})
	}

	total := decimal.NewFromFloat(resp.Tax.AmountToCollect)

	return &TaxBreakdown{
		TaxTotal: total,
		Lines:    lines,
	}
}
