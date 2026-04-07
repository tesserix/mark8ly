package location

import (
	"context"
	"errors"
	"net/http"
	"testing"

	apperrors "github.com/mark8ly/platform-api/pkg/errors"
)

// fakeRepo is a tiny in-memory implementation of Repository for unit tests.
// Lookups by primary key go through the maps; "list" methods return slices
// in the order they were inserted (test fixtures decide the order).
type fakeRepo struct {
	countries  map[string]Country
	states     map[string]State
	statesByCC map[string][]State
	currencies map[string]Currency
	timezones  map[string]Timezone
	listErr    error // when non-nil, list methods return this error (forces failure paths)
}

func newFakeRepo() *fakeRepo {
	return &fakeRepo{
		countries:  make(map[string]Country),
		states:     make(map[string]State),
		statesByCC: make(map[string][]State),
		currencies: make(map[string]Currency),
		timezones:  make(map[string]Timezone),
	}
}

func (f *fakeRepo) addCountry(code, name string) {
	f.countries[code] = Country{Code: code, Name: name}
}
func (f *fakeRepo) addState(country, code, name string) {
	id := country + "-" + code
	s := State{ID: id, CountryCode: country, Code: code, Name: name, Type: "state"}
	f.states[id] = s
	f.statesByCC[country] = append(f.statesByCC[country], s)
}
func (f *fakeRepo) addCurrency(code, name, symbol string) {
	f.currencies[code] = Currency{Code: code, Name: name, Symbol: symbol, DecimalPlaces: 2}
}
func (f *fakeRepo) addTimezone(id, name, offset string) {
	f.timezones[id] = Timezone{ID: id, Name: name, UTCOffset: offset}
}

func (f *fakeRepo) ListCountries(ctx context.Context) ([]Country, error) {
	if f.listErr != nil {
		return nil, f.listErr
	}
	out := make([]Country, 0, len(f.countries))
	for _, c := range f.countries {
		out = append(out, c)
	}
	return out, nil
}

func (f *fakeRepo) GetCountry(ctx context.Context, code string) (*Country, error) {
	c, ok := f.countries[code]
	if !ok {
		return nil, apperrors.NotFound("country_not_found", "no such country")
	}
	return &c, nil
}

func (f *fakeRepo) ListStatesByCountry(ctx context.Context, countryCode string) ([]State, error) {
	return f.statesByCC[countryCode], nil
}

func (f *fakeRepo) GetState(ctx context.Context, id string) (*State, error) {
	s, ok := f.states[id]
	if !ok {
		return nil, apperrors.NotFound("state_not_found", "no such state")
	}
	return &s, nil
}

func (f *fakeRepo) ListCurrencies(ctx context.Context) ([]Currency, error) {
	out := make([]Currency, 0, len(f.currencies))
	for _, c := range f.currencies {
		out = append(out, c)
	}
	return out, nil
}

func (f *fakeRepo) GetCurrency(ctx context.Context, code string) (*Currency, error) {
	c, ok := f.currencies[code]
	if !ok {
		return nil, apperrors.NotFound("currency_not_found", "no such currency")
	}
	return &c, nil
}

func (f *fakeRepo) ListTimezones(ctx context.Context) ([]Timezone, error) {
	out := make([]Timezone, 0, len(f.timezones))
	for _, tz := range f.timezones {
		out = append(out, tz)
	}
	return out, nil
}

func (f *fakeRepo) GetTimezone(ctx context.Context, id string) (*Timezone, error) {
	tz, ok := f.timezones[id]
	if !ok {
		return nil, apperrors.NotFound("timezone_not_found", "no such timezone")
	}
	return &tz, nil
}

// ─────────────────────────────────────────────────────────────────────────
// Country tests
// ─────────────────────────────────────────────────────────────────────────

func TestService_GetCountry_UppercasesInput(t *testing.T) {
	repo := newFakeRepo()
	repo.addCountry("US", "United States")
	svc := NewService(repo)

	got, err := svc.GetCountry(context.Background(), "us")
	if err != nil {
		t.Fatalf("GetCountry(\"us\") error = %v, want nil (lowercase should be normalized)", err)
	}
	if got.Code != "US" {
		t.Errorf("Code = %q, want %q", got.Code, "US")
	}
}

func TestService_GetCountry_TrimsWhitespace(t *testing.T) {
	repo := newFakeRepo()
	repo.addCountry("IN", "India")
	svc := NewService(repo)

	if _, err := svc.GetCountry(context.Background(), "  IN  "); err != nil {
		t.Errorf("GetCountry with whitespace should succeed, got %v", err)
	}
}

func TestService_GetCountry_RejectsInvalidLength(t *testing.T) {
	svc := NewService(newFakeRepo())
	cases := []string{"", "U", "USA", "ABCD"}
	for _, code := range cases {
		t.Run("len_"+code, func(t *testing.T) {
			_, err := svc.GetCountry(context.Background(), code)
			ae, ok := apperrors.As(err)
			if !ok {
				t.Fatalf("expected AppError, got %v", err)
			}
			if ae.Status != http.StatusBadRequest {
				t.Errorf("status = %d, want 400", ae.Status)
			}
			if ae.Code != "invalid_country_code" {
				t.Errorf("code = %q, want invalid_country_code", ae.Code)
			}
		})
	}
}

func TestService_GetCountry_PropagatesNotFound(t *testing.T) {
	svc := NewService(newFakeRepo())

	_, err := svc.GetCountry(context.Background(), "ZZ")
	ae, ok := apperrors.As(err)
	if !ok {
		t.Fatalf("expected AppError, got %v", err)
	}
	if ae.Status != http.StatusNotFound {
		t.Errorf("status = %d, want 404", ae.Status)
	}
}

func TestService_ListCountries_PropagatesRepoError(t *testing.T) {
	repo := newFakeRepo()
	repo.listErr = errors.New("db down")
	svc := NewService(repo)

	if _, err := svc.ListCountries(context.Background()); err == nil {
		t.Error("ListCountries should propagate repo error")
	}
}

// ─────────────────────────────────────────────────────────────────────────
// State tests
// ─────────────────────────────────────────────────────────────────────────

func TestService_ListStatesByCountry_ReturnsEmptySliceForNoStates(t *testing.T) {
	svc := NewService(newFakeRepo())

	states, err := svc.ListStatesByCountry(context.Background(), "US")
	if err != nil {
		t.Fatalf("error = %v, want nil", err)
	}
	if len(states) != 0 {
		t.Errorf("len = %d, want 0 (no states is not an error)", len(states))
	}
}

func TestService_ListStatesByCountry_ReturnsAllStates(t *testing.T) {
	repo := newFakeRepo()
	repo.addState("US", "CA", "California")
	repo.addState("US", "NY", "New York")
	svc := NewService(repo)

	states, err := svc.ListStatesByCountry(context.Background(), "us")
	if err != nil {
		t.Fatalf("error = %v", err)
	}
	if len(states) != 2 {
		t.Errorf("len = %d, want 2", len(states))
	}
}

func TestService_GetState_RejectsMalformedID(t *testing.T) {
	svc := NewService(newFakeRepo())
	cases := []string{"", "US", "US_CA", "US-", "U-CA", "1-CA", "us-ca", "USCA"}
	for _, id := range cases {
		t.Run("id_"+id, func(t *testing.T) {
			// "us-ca" succeeds because we uppercase before validation. Skip that case.
			if id == "us-ca" {
				return
			}
			_, err := svc.GetState(context.Background(), id)
			ae, ok := apperrors.As(err)
			if !ok {
				t.Fatalf("expected AppError, got %v", err)
			}
			if ae.Code != "invalid_state_id" {
				t.Errorf("code = %q, want invalid_state_id", ae.Code)
			}
		})
	}
}

func TestService_GetState_AcceptsValidID(t *testing.T) {
	repo := newFakeRepo()
	repo.addState("US", "CA", "California")
	svc := NewService(repo)

	got, err := svc.GetState(context.Background(), "us-ca")
	if err != nil {
		t.Fatalf("error = %v", err)
	}
	if got.Name != "California" {
		t.Errorf("Name = %q, want California", got.Name)
	}
}

// ─────────────────────────────────────────────────────────────────────────
// Currency tests
// ─────────────────────────────────────────────────────────────────────────

func TestService_GetCurrency_RejectsInvalidLength(t *testing.T) {
	svc := NewService(newFakeRepo())
	cases := []string{"", "US", "USDD"}
	for _, code := range cases {
		_, err := svc.GetCurrency(context.Background(), code)
		ae, ok := apperrors.As(err)
		if !ok || ae.Code != "invalid_currency_code" {
			t.Errorf("GetCurrency(%q) should return invalid_currency_code, got %v", code, err)
		}
	}
}

func TestService_GetCurrency_UppercasesInput(t *testing.T) {
	repo := newFakeRepo()
	repo.addCurrency("USD", "US Dollar", "$")
	svc := NewService(repo)

	got, err := svc.GetCurrency(context.Background(), "usd")
	if err != nil {
		t.Fatalf("error = %v", err)
	}
	if got.Symbol != "$" {
		t.Errorf("Symbol = %q, want $", got.Symbol)
	}
}

// ─────────────────────────────────────────────────────────────────────────
// Timezone tests
// ─────────────────────────────────────────────────────────────────────────

func TestService_GetTimezone_RejectsEmpty(t *testing.T) {
	svc := NewService(newFakeRepo())

	_, err := svc.GetTimezone(context.Background(), "  ")
	ae, ok := apperrors.As(err)
	if !ok || ae.Code != "invalid_timezone_id" {
		t.Errorf("expected invalid_timezone_id, got %v", err)
	}
}

func TestService_GetTimezone_PreservesCase(t *testing.T) {
	repo := newFakeRepo()
	repo.addTimezone("America/New_York", "Eastern", "-05:00")
	svc := NewService(repo)

	// IANA IDs are case-sensitive: "america/new_york" must NOT match.
	if _, err := svc.GetTimezone(context.Background(), "america/new_york"); err == nil {
		t.Error("GetTimezone should be case-sensitive")
	}
	if _, err := svc.GetTimezone(context.Background(), "America/New_York"); err != nil {
		t.Errorf("GetTimezone with correct casing failed: %v", err)
	}
}
