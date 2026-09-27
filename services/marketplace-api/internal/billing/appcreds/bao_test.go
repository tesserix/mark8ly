package appcreds

import (
	"bytes"
	"encoding/base64"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/mark8ly/marketplace-api/internal/bao"
)

func TestBaoSMRoundTripAndTenantIsolation(t *testing.T) {
	values := map[string]string{}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.Method {
		case http.MethodPut:
			var body struct {
				Data map[string]string `json:"data"`
			}
			if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
				t.Error(err)
			}
			values[r.URL.Path] = body.Data["value"]
			_, _ = w.Write([]byte(`{"data":{"version":1}}`))
		case http.MethodGet:
			v, ok := values[r.URL.Path]
			if !ok {
				w.WriteHeader(404)
				return
			}
			_ = json.NewEncoder(w).Encode(map[string]any{"data": map[string]any{"data": map[string]string{"value": v}, "metadata": map[string]int{"version": 1}}})
		case http.MethodDelete:
			delete(values, "/v1/kv/data/"+strings.TrimPrefix(r.URL.Path, "/v1/kv/metadata/"))
			w.WriteHeader(204)
		}
	}))
	defer server.Close()
	client, err := bao.New(bao.Config{Address: server.URL, Token: "test-token"})
	if err != nil {
		t.Fatal(err)
	}
	sm := NewBaoSM(client, "test-project")
	tenant := "11111111-1111-4111-8111-111111111111"
	name := Path("test-project", tenant, CredTypeAppleP8)
	payload := []byte{0, 255, 1, 10}
	if err := sm.CreateOrAddVersion(t.Context(), name, payload); err != nil {
		t.Fatal(err)
	}
	path := "/v1/kv/data/mark8ly/marketplace-api/tenants/" + tenant + "/app-credentials/mark8ly-apple-asc-api-key"
	if values[path] != base64.StdEncoding.EncodeToString(payload) {
		t.Fatal("wrong destination or payload encoding")
	}
	got, err := sm.AccessLatest(t.Context(), name)
	if err != nil || !bytes.Equal(got, payload) {
		t.Fatalf("readback mismatch: %v", err)
	}
	_, err = sm.AccessLatest(t.Context(), Path("test-project", "22222222-2222-4222-8222-222222222222", CredTypeAppleP8))
	if !errors.Is(err, ErrSMNotFound) {
		t.Fatalf("other tenant should not read this value: %v", err)
	}
	for _, invalid := range []string{Path("wrong-project", tenant, CredTypeAppleP8), Path("test-project", "../other", CredTypeAppleP8), Path("test-project", tenant, CredType("../../other"))} {
		if err := sm.CreateOrAddVersion(t.Context(), invalid, payload); err == nil {
			t.Fatal("accepted invalid name")
		}
		if _, err := sm.AccessLatest(t.Context(), invalid); err == nil {
			t.Fatal("accepted invalid read")
		}
		if err := sm.Delete(t.Context(), invalid); err == nil {
			t.Fatal("accepted invalid delete")
		}
	}
	if err := sm.Delete(t.Context(), name); err != nil {
		t.Fatal(err)
	}
	if _, err := sm.AccessLatest(t.Context(), name); !errors.Is(err, ErrSMNotFound) {
		t.Fatalf("expected deleted secret: %v", err)
	}
	if err := sm.Delete(t.Context(), name); err != nil {
		t.Fatal(err)
	}
}

func TestBaoSMReadErrors(t *testing.T) {
	for _, tc := range []struct {
		name   string
		status int
		body   string
		want   error
	}{
		{"missing", 404, `{}`, ErrSMNotFound},
		{"forbidden", 403, `{}`, bao.ErrForbidden},
		{"no-value", 200, `{"data":{"data":{},"metadata":{"version":1}}}`, nil},
		{"invalid-encoding", 200, `{"data":{"data":{"value":"!"},"metadata":{"version":1}}}`, nil},
	} {
		t.Run(tc.name, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.WriteHeader(tc.status)
				_, _ = w.Write([]byte(tc.body))
			}))
			defer server.Close()
			client, err := bao.New(bao.Config{Address: server.URL, Token: "test-token"})
			if err != nil {
				t.Fatal(err)
			}
			sm := NewBaoSM(client, "test-project")
			_, err = sm.AccessLatest(t.Context(), Path("test-project", "11111111-1111-4111-8111-111111111111", CredTypeAppleP8))
			if err == nil {
				t.Fatal("expected error")
			}
			if tc.want != nil && !errors.Is(err, tc.want) {
				t.Fatalf("wrong error: %v", err)
			}
		})
	}
}
