package payment

import (
	"fmt"
	"strings"
)

// NewGateway is a factory function that returns the correct Gateway
// implementation for the given provider name. Mode is normalized and
// validated fail-closed: an unrecognized value (typo like "prod") is
// rejected rather than silently defaulting to the live endpoint. "sandbox"
// is accepted as an alias for "test" (PayPal's terminology); empty stays
// "live" to preserve existing gateway-config behaviour.
func NewGateway(provider, apiKey, secretKey, mode string) (Gateway, error) {
	m := strings.ToLower(strings.TrimSpace(mode))
	switch m {
	case "", "live":
		m = "live"
	case "test", "sandbox":
		m = "test"
	default:
		return nil, fmt.Errorf("payment: invalid gateway mode %q (want live|test)", mode)
	}
	switch strings.ToLower(provider) {
	case "stripe":
		return NewStripeGateway(apiKey, secretKey, m), nil
	case "razorpay":
		return NewRazorpayGateway(apiKey, secretKey, m), nil
	case "paypal":
		return NewPayPalGateway(apiKey, secretKey, m), nil
	default:
		return nil, fmt.Errorf("unsupported payment provider: %s", provider)
	}
}

// ResolveForCountry filters the provided gateways to those that support
// the given ISO 3166-1 alpha-2 country code. The order of the input
// slice is preserved so callers can express preference via ordering.
func ResolveForCountry(countryCode string, gateways []Gateway) []Gateway {
	code := strings.ToUpper(countryCode)
	var matched []Gateway
	for _, gw := range gateways {
		for _, c := range gw.SupportedCountries() {
			if c == code {
				matched = append(matched, gw)
				break
			}
		}
	}
	return matched
}
