package payment

import (
	"fmt"
	"strings"
)

// NewGateway is a factory function that returns the correct Gateway
// implementation for the given provider name.
func NewGateway(provider, apiKey, secretKey, mode string) (Gateway, error) {
	switch strings.ToLower(provider) {
	case "stripe":
		return NewStripeGateway(apiKey, secretKey, mode), nil
	case "razorpay":
		return NewRazorpayGateway(apiKey, secretKey, mode), nil
	case "paypal":
		return NewPayPalGateway(apiKey, secretKey, mode), nil
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
