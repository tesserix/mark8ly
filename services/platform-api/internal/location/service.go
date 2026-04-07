package location

import (
	"context"
	"strings"

	apperrors "github.com/mark8ly/platform-api/pkg/errors"
)

// Service is the business-logic layer for location reads.
//
// For reference data, the "logic" is mostly input normalization (uppercasing
// codes, validating shape) before delegating to the repository. Keeping this
// layer thin but explicit makes future caching and metrics natural to add.
type Service struct {
	repo Repository
}

// NewService constructs a Service backed by the given Repository.
func NewService(repo Repository) *Service {
	return &Service{repo: repo}
}

// ListCountries returns all known countries, sorted by name.
func (s *Service) ListCountries(ctx context.Context) ([]Country, error) {
	return s.repo.ListCountries(ctx)
}

// GetCountry returns the country with the given ISO 3166-1 alpha-2 code.
// The code is uppercased before lookup so callers can pass either case.
func (s *Service) GetCountry(ctx context.Context, code string) (*Country, error) {
	code = strings.ToUpper(strings.TrimSpace(code))
	if len(code) != 2 {
		return nil, apperrors.BadRequest("invalid_country_code", "country code must be a 2-letter ISO code")
	}
	return s.repo.GetCountry(ctx, code)
}

// ListStatesByCountry returns all states for a given country, sorted by name.
// Returns BadRequest if the country code is malformed; an empty list (not an
// error) if the country exists but has no states.
func (s *Service) ListStatesByCountry(ctx context.Context, countryCode string) ([]State, error) {
	countryCode = strings.ToUpper(strings.TrimSpace(countryCode))
	if len(countryCode) != 2 {
		return nil, apperrors.BadRequest("invalid_country_code", "country code must be a 2-letter ISO code")
	}
	return s.repo.ListStatesByCountry(ctx, countryCode)
}

// GetState returns the state with the given country-prefixed ID (e.g. "US-CA").
func (s *Service) GetState(ctx context.Context, id string) (*State, error) {
	id = strings.ToUpper(strings.TrimSpace(id))
	if !isValidStateID(id) {
		return nil, apperrors.BadRequest("invalid_state_id", "state id must be in CC-XX format (e.g. US-CA)")
	}
	return s.repo.GetState(ctx, id)
}

// ListCurrencies returns all known currencies, sorted by ISO code.
func (s *Service) ListCurrencies(ctx context.Context) ([]Currency, error) {
	return s.repo.ListCurrencies(ctx)
}

// GetCurrency returns the currency with the given ISO 4217 code.
func (s *Service) GetCurrency(ctx context.Context, code string) (*Currency, error) {
	code = strings.ToUpper(strings.TrimSpace(code))
	if len(code) != 3 {
		return nil, apperrors.BadRequest("invalid_currency_code", "currency code must be a 3-letter ISO 4217 code")
	}
	return s.repo.GetCurrency(ctx, code)
}

// ListTimezones returns all known IANA timezones, sorted by ID.
func (s *Service) ListTimezones(ctx context.Context) ([]Timezone, error) {
	return s.repo.ListTimezones(ctx)
}

// GetTimezone returns the timezone with the given IANA ID.
func (s *Service) GetTimezone(ctx context.Context, id string) (*Timezone, error) {
	id = strings.TrimSpace(id)
	if id == "" {
		return nil, apperrors.BadRequest("invalid_timezone_id", "timezone id is required")
	}
	return s.repo.GetTimezone(ctx, id)
}

// isValidStateID checks the CC-XX shape (2-letter country, dash, 1+ char state code).
func isValidStateID(id string) bool {
	if len(id) < 4 || id[2] != '-' {
		return false
	}
	for i := 0; i < 2; i++ {
		if id[i] < 'A' || id[i] > 'Z' {
			return false
		}
	}
	return true
}
