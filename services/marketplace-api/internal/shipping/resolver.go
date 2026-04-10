package shipping

import (
	"fmt"
	"strings"
)

// NewCarrier constructs a Carrier implementation for the named provider.
// Supported providers: "shipengine", "delhivery", "ninjavan".
// mode must be "test" or "live". secretKey is only required by providers
// that use OAuth (NinjaVan); pass "" for others.
func NewCarrier(provider, apiKey, secretKey, mode string) (Carrier, error) {
	if apiKey == "" {
		return nil, fmt.Errorf("shipping: NewCarrier: apiKey is required")
	}
	if mode != "test" && mode != "live" {
		return nil, fmt.Errorf("shipping: NewCarrier: mode must be \"test\" or \"live\", got %q", mode)
	}

	switch strings.ToLower(provider) {
	case "shipengine":
		return NewShipEngineCarrier(apiKey, mode), nil
	case "delhivery":
		return NewDelhiveryCarrier(apiKey, mode), nil
	case "ninjavan":
		if secretKey == "" {
			return nil, fmt.Errorf("shipping: NewCarrier: secretKey is required for ninjavan")
		}
		return NewNinjaVanCarrier(apiKey, secretKey, mode), nil
	default:
		return nil, fmt.Errorf("shipping: NewCarrier: unknown provider %q", provider)
	}
}

// ResolveForCountry filters the supplied carriers to those that support the
// given ISO 3166-1 alpha-2 country code. Returns an empty slice (never nil)
// when no carrier matches.
func ResolveForCountry(countryCode string, carriers []Carrier) []Carrier {
	cc := strings.ToUpper(countryCode)
	matched := make([]Carrier, 0, len(carriers))
	for _, c := range carriers {
		for _, supported := range c.SupportedCountries() {
			if supported == cc {
				matched = append(matched, c)
				break
			}
		}
	}
	return matched
}
