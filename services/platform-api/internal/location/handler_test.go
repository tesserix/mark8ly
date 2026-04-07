package location

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
)

// newTestHandler builds a Handler with a fake repo seeded for tests.
// No DB, no network — pure in-process. Tests run in microseconds.
func newTestHandler() *Handler {
	repo := newFakeRepo()
	repo.addCountry("US", "United States")
	repo.addCountry("IN", "India")
	repo.addState("US", "CA", "California")
	repo.addState("US", "NY", "New York")
	repo.addCurrency("USD", "US Dollar", "$")
	repo.addTimezone("America/New_York", "Eastern", "-05:00")
	return NewHandler(NewService(repo))
}

func newRouter(h *Handler) *gin.Engine {
	gin.SetMode(gin.TestMode)
	r := gin.New()
	v1 := r.Group("/api/v1")
	h.Register(v1)
	return r
}

func doGet(t *testing.T, r http.Handler, path string) (*httptest.ResponseRecorder, map[string]any) {
	t.Helper()
	req := httptest.NewRequest(http.MethodGet, path, nil).WithContext(context.Background())
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, req)

	var body map[string]any
	if rec.Body.Len() > 0 {
		if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
			t.Fatalf("response not JSON: %v\nbody: %s", err, rec.Body.String())
		}
	}
	return rec, body
}

func TestHandler_ListCountries_Returns200WithDataEnvelope(t *testing.T) {
	r := newRouter(newTestHandler())
	rec, body := doGet(t, r, "/api/v1/locations/countries")

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", rec.Code)
	}
	data, ok := body["data"].([]any)
	if !ok {
		t.Fatalf("response missing `data` array, got %v", body)
	}
	if len(data) != 2 {
		t.Errorf("data len = %d, want 2", len(data))
	}
}

func TestHandler_GetCountry_Returns200ForKnown(t *testing.T) {
	r := newRouter(newTestHandler())
	rec, body := doGet(t, r, "/api/v1/locations/countries/US")

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", rec.Code)
	}
	data := body["data"].(map[string]any)
	if data["code"] != "US" {
		t.Errorf("data.code = %v, want US", data["code"])
	}
}

func TestHandler_GetCountry_AcceptsLowercase(t *testing.T) {
	r := newRouter(newTestHandler())
	rec, _ := doGet(t, r, "/api/v1/locations/countries/in")

	if rec.Code != http.StatusOK {
		t.Errorf("status = %d, want 200 (lowercase should be normalized)", rec.Code)
	}
}

func TestHandler_GetCountry_Returns404ForUnknown(t *testing.T) {
	r := newRouter(newTestHandler())
	rec, body := doGet(t, r, "/api/v1/locations/countries/ZZ")

	if rec.Code != http.StatusNotFound {
		t.Fatalf("status = %d, want 404", rec.Code)
	}
	if body["error"] != "country_not_found" {
		t.Errorf("error = %v, want country_not_found", body["error"])
	}
}

func TestHandler_GetCountry_Returns400ForMalformed(t *testing.T) {
	r := newRouter(newTestHandler())
	rec, body := doGet(t, r, "/api/v1/locations/countries/USA")

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400", rec.Code)
	}
	if body["error"] != "invalid_country_code" {
		t.Errorf("error = %v, want invalid_country_code", body["error"])
	}
}

func TestHandler_ListStatesByCountry_Returns200(t *testing.T) {
	r := newRouter(newTestHandler())
	rec, body := doGet(t, r, "/api/v1/locations/countries/US/states")

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", rec.Code)
	}
	if len(body["data"].([]any)) != 2 {
		t.Errorf("expected 2 states, got %v", body["data"])
	}
}

func TestHandler_ListStatesByCountry_ReturnsEmptyForCountryWithNoStates(t *testing.T) {
	r := newRouter(newTestHandler())
	rec, body := doGet(t, r, "/api/v1/locations/countries/IN/states")

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", rec.Code)
	}
	if len(body["data"].([]any)) != 0 {
		t.Errorf("expected empty data, got %v", body["data"])
	}
}

func TestHandler_GetState_Returns200(t *testing.T) {
	r := newRouter(newTestHandler())
	rec, body := doGet(t, r, "/api/v1/locations/states/US-CA")

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", rec.Code)
	}
	if body["data"].(map[string]any)["name"] != "California" {
		t.Errorf("name = %v, want California", body["data"])
	}
}

func TestHandler_GetState_Returns400ForMalformedID(t *testing.T) {
	r := newRouter(newTestHandler())
	rec, body := doGet(t, r, "/api/v1/locations/states/garbage")

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400", rec.Code)
	}
	if body["error"] != "invalid_state_id" {
		t.Errorf("error = %v, want invalid_state_id", body["error"])
	}
}

func TestHandler_GetCurrency_Returns200(t *testing.T) {
	r := newRouter(newTestHandler())
	rec, body := doGet(t, r, "/api/v1/locations/currencies/USD")

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", rec.Code)
	}
	if body["data"].(map[string]any)["symbol"] != "$" {
		t.Errorf("symbol = %v, want $", body["data"])
	}
}

func TestHandler_GetTimezone_Returns200(t *testing.T) {
	r := newRouter(newTestHandler())
	// IANA IDs contain "/" — handler uses Gin's catch-all so we pass them raw.
	rec, body := doGet(t, r, "/api/v1/locations/timezones/America/New_York")

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200, body = %s", rec.Code, rec.Body.String())
	}
	if body["data"].(map[string]any)["utc_offset"] != "-05:00" {
		t.Errorf("utc_offset = %v, want -05:00", body["data"])
	}
}

func TestHandler_GetTimezone_Returns404ForUnknown(t *testing.T) {
	r := newRouter(newTestHandler())
	rec, body := doGet(t, r, "/api/v1/locations/timezones/Mars/Olympus_Mons")

	if rec.Code != http.StatusNotFound {
		t.Fatalf("status = %d, want 404", rec.Code)
	}
	if body["error"] != "timezone_not_found" {
		t.Errorf("error = %v, want timezone_not_found", body["error"])
	}
}
