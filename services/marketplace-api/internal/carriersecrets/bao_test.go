package carriersecrets

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"

	"github.com/mark8ly/marketplace-api/internal/bao"
)

// fakeKV is a minimal in-memory KV v2 backend used to drive an httptest
// server. It tracks the encoded payload and a monotonically increasing
// version per data path, and honors metadata-path deletes as removing the
// path entirely (matching OpenBao's "destroy" semantics).
type fakeKV struct {
	mu      sync.Mutex
	data    map[string]string // data path -> "value" field
	version map[string]int
}

func newFakeKV() *fakeKV {
	return &fakeKV{data: map[string]string{}, version: map[string]int{}}
}

// newFakeBaoServer wires a fakeKV behind an httptest server whose routes
// match the real OpenBao KV v2 HTTP surface under mount "kv":
// /v1/kv/data/<rest> and /v1/kv/metadata/<rest>. requestLog, if non-nil,
// records every request path/method observed so tests can assert on which
// URL the client actually hit.
func newFakeBaoServer(t *testing.T, kv *fakeKV, requestLog *[]string) *httptest.Server {
	t.Helper()
	const dataPrefix = "/v1/kv/data/"
	const metaPrefix = "/v1/kv/metadata/"

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if requestLog != nil {
			*requestLog = append(*requestLog, r.Method+" "+r.URL.Path)
		}
		kv.mu.Lock()
		defer kv.mu.Unlock()

		switch {
		case r.Method == http.MethodPut && len(r.URL.Path) > len(dataPrefix) && r.URL.Path[:len(dataPrefix)] == dataPrefix:
			rest := r.URL.Path[len(dataPrefix):]
			var body struct {
				Data map[string]string `json:"data"`
			}
			if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
				w.WriteHeader(http.StatusBadRequest)
				return
			}
			kv.version[rest]++
			kv.data[rest] = body.Data[baoValueField]
			w.Header().Set("Content-Type", "application/json")
			fmt.Fprintf(w, `{"data":{"version":%d}}`, kv.version[rest])

		case r.Method == http.MethodGet && len(r.URL.Path) > len(dataPrefix) && r.URL.Path[:len(dataPrefix)] == dataPrefix:
			rest := r.URL.Path[len(dataPrefix):]
			v, ok := kv.data[rest]
			if !ok {
				w.WriteHeader(http.StatusNotFound)
				return
			}
			w.Header().Set("Content-Type", "application/json")
			fmt.Fprintf(w, `{"data":{"data":{%q:%q},"metadata":{"version":%d}}}`, baoValueField, v, kv.version[rest])

		case r.Method == http.MethodDelete && len(r.URL.Path) > len(metaPrefix) && r.URL.Path[:len(metaPrefix)] == metaPrefix:
			rest := r.URL.Path[len(metaPrefix):]
			if _, ok := kv.data[rest]; !ok {
				w.WriteHeader(http.StatusNotFound)
				return
			}
			delete(kv.data, rest)
			delete(kv.version, rest)
			w.WriteHeader(http.StatusNoContent)

		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	t.Cleanup(srv.Close)
	return srv
}

func newTestBaoClient(t *testing.T, addr string) *BaoClient {
	t.Helper()
	c, err := bao.New(bao.Config{
		Address: addr,
		Mount:   "kv",
		Token:   "test-token",
	})
	if err != nil {
		t.Fatalf("bao.New: %v", err)
	}
	return NewBaoClient(c, "kv")
}

const testBaoPath = "kv/mark8ly/marketplace-api/tenants/tenant-1/payment/razorpay/api_key"

func TestBaoClient_CreateAddAccess(t *testing.T) {
	srv := newFakeBaoServer(t, newFakeKV(), nil)
	bc := newTestBaoClient(t, srv.URL)

	want := []byte("rzp_live_secret_123")
	if err := bc.CreateOrAddVersion(t.Context(), testBaoPath, want); err != nil {
		t.Fatalf("CreateOrAddVersion: %v", err)
	}

	got, err := bc.AccessLatest(t.Context(), testBaoPath)
	if err != nil {
		t.Fatalf("AccessLatest: %v", err)
	}
	if !bytes.Equal(got, want) {
		t.Fatalf("AccessLatest = %q, want %q", got, want)
	}
}

func TestBaoClient_SecondWriteIsNewVersion(t *testing.T) {
	srv := newFakeBaoServer(t, newFakeKV(), nil)
	bc := newTestBaoClient(t, srv.URL)

	first := []byte("first-value")
	second := []byte("second-value")

	if err := bc.CreateOrAddVersion(t.Context(), testBaoPath, first); err != nil {
		t.Fatalf("first CreateOrAddVersion: %v", err)
	}
	if err := bc.CreateOrAddVersion(t.Context(), testBaoPath, second); err != nil {
		t.Fatalf("second CreateOrAddVersion: %v", err)
	}

	got, err := bc.AccessLatest(t.Context(), testBaoPath)
	if err != nil {
		t.Fatalf("AccessLatest: %v", err)
	}
	if !bytes.Equal(got, second) {
		t.Fatalf("AccessLatest = %q, want second write %q", got, second)
	}
}

func TestBaoClient_MissingMapsToErrSecretNotFound(t *testing.T) {
	srv := newFakeBaoServer(t, newFakeKV(), nil)
	bc := newTestBaoClient(t, srv.URL)

	_, err := bc.AccessLatest(t.Context(), testBaoPath)
	if !errors.Is(err, ErrSecretNotFound) {
		t.Fatalf("AccessLatest error = %v, want ErrSecretNotFound", err)
	}
	if errors.Is(err, bao.ErrNotFound) {
		t.Fatalf("AccessLatest leaked bao.ErrNotFound instead of the carriersecrets sentinel: %v", err)
	}
}

func TestBaoClient_DeleteUsesMetadataPath(t *testing.T) {
	kv := newFakeKV()
	var requests []string
	srv := newFakeBaoServer(t, kv, &requests)
	bc := newTestBaoClient(t, srv.URL)

	if err := bc.CreateOrAddVersion(t.Context(), testBaoPath, []byte("secret")); err != nil {
		t.Fatalf("CreateOrAddVersion: %v", err)
	}
	requests = nil // ignore setup traffic; only inspect the delete call itself

	if err := bc.DeleteSecret(t.Context(), testBaoPath); err != nil {
		t.Fatalf("DeleteSecret: %v", err)
	}

	wantPath := "DELETE /v1/kv/metadata/mark8ly/marketplace-api/tenants/tenant-1/payment/razorpay/api_key"
	found := false
	for _, req := range requests {
		if req == wantPath {
			found = true
		}
		if req == "DELETE /v1/kv/data/mark8ly/marketplace-api/tenants/tenant-1/payment/razorpay/api_key" {
			t.Fatalf("DeleteSecret hit the soft-delete DATA path, want the irreversible METADATA path: %v", requests)
		}
	}
	if !found {
		t.Fatalf("DeleteSecret never hit %q; requests observed: %v", wantPath, requests)
	}

	// Confirm the delete was real: a subsequent read is not-found, not a
	// resurrected soft-deleted version.
	if _, err := bc.AccessLatest(t.Context(), testBaoPath); !errors.Is(err, ErrSecretNotFound) {
		t.Fatalf("AccessLatest after delete = %v, want ErrSecretNotFound", err)
	}
}

func TestBaoClient_DeleteNotFoundIsSuccess(t *testing.T) {
	srv := newFakeBaoServer(t, newFakeKV(), nil)
	bc := newTestBaoClient(t, srv.URL)

	if err := bc.DeleteSecret(t.Context(), testBaoPath); err != nil {
		t.Fatalf("DeleteSecret on a never-written path should be idempotent success, got: %v", err)
	}
}

// TestBaoClient_RelativePathRejectsWrongMount pins that a logical path not
// under this client's configured mount is rejected rather than silently
// routed to the wrong OpenBao mount.
func TestBaoClient_RelativePathRejectsWrongMount(t *testing.T) {
	srv := newFakeBaoServer(t, newFakeKV(), nil)
	bc := newTestBaoClient(t, srv.URL)

	err := bc.CreateOrAddVersion(t.Context(), "secret/mark8ly/foo", []byte("x"))
	if err == nil {
		t.Fatal("expected an error for a path outside this client's mount")
	}
}
