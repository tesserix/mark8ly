package bao

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
)

// TestClient_ReadSecret_DecodesNumericVersion pins the version-decoding bug:
// the pinned OpenBao SDK decodes every JSON response with json.Decoder's
// UseNumber() (see openbao/api/v2's secret.go/response.go), so every numeric
// field in resp.Data — including "version" — arrives as json.Number, not
// float64 or string. intFrom must handle that case, or ReadSecret and
// WriteSecret silently report version 0 and CAS ("ifVersion > 0") is
// permanently disabled for a version reported through the real SDK path.
func TestClient_ReadSecret_DecodesNumericVersion(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/v1/auth/kubernetes/login":
			w.Header().Set("Content-Type", "application/json")
			fmt.Fprint(w, loginResponse(3600))
		case "/v1/kv/data/razorpay/store-1":
			w.Header().Set("Content-Type", "application/json")
			// A real KV v2 read response: "version" is a bare JSON number,
			// which the pinned SDK decodes as json.Number, not float64.
			fmt.Fprint(w, `{
				"data": {
					"data": {"key_id": "rzp_test_123"},
					"metadata": {"version": 7}
				}
			}`)
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	defer srv.Close()

	c, err := New(Config{
		Address:             srv.URL,
		KubernetesRole:      "marketplace-api",
		ServiceAccountToken: writeServiceAccountToken(t),
	})
	if err != nil {
		t.Fatalf("New: %v", err)
	}

	_, version, err := c.ReadSecret(t.Context(), "razorpay/store-1")
	if err != nil {
		t.Fatalf("ReadSecret: %v", err)
	}
	if version != 7 {
		t.Fatalf("expected version 7 (decoded from json.Number), got %d", version)
	}
}
