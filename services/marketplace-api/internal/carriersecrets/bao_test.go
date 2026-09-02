package carriersecrets

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
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
// match the real OpenBao KV v2 HTTP surface for ANY mount:
// /v1/<mount>/data/<rest> and /v1/<mount>/metadata/<rest>. The mount is not
// hardcoded to "kv" — TestBaoClient_UsesClientsConfiguredMount depends on
// this server understanding a non-default mount ("secret") too. requestLog,
// if non-nil, records every request path/method observed so tests can
// assert on which URL the client actually hit. Storage keys are the full
// "<mount>/data-or-metadata-relative-path>" so distinct mounts never
// collide even if a rest path happens to repeat across them.
func newFakeBaoServer(t *testing.T, kv *fakeKV, requestLog *[]string) *httptest.Server {
	t.Helper()

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if requestLog != nil {
			*requestLog = append(*requestLog, r.Method+" "+r.URL.Path)
		}
		kv.mu.Lock()
		defer kv.mu.Unlock()

		path := strings.TrimPrefix(r.URL.Path, "/v1/")
		var key string
		var isData bool
		switch {
		case strings.Contains(path, "/data/"):
			mount, rest, _ := strings.Cut(path, "/data/")
			key = mount + "/" + rest
			isData = true
		case strings.Contains(path, "/metadata/"):
			mount, rest, _ := strings.Cut(path, "/metadata/")
			key = mount + "/" + rest
			isData = false
		default:
			w.WriteHeader(http.StatusNotFound)
			return
		}

		switch {
		case isData && r.Method == http.MethodPut:
			var body struct {
				Data map[string]string `json:"data"`
			}
			if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
				w.WriteHeader(http.StatusBadRequest)
				return
			}
			kv.version[key]++
			kv.data[key] = body.Data[baoValueField]
			w.Header().Set("Content-Type", "application/json")
			fmt.Fprintf(w, `{"data":{"version":%d}}`, kv.version[key])

		case isData && r.Method == http.MethodGet:
			v, ok := kv.data[key]
			if !ok {
				w.WriteHeader(http.StatusNotFound)
				return
			}
			w.Header().Set("Content-Type", "application/json")
			fmt.Fprintf(w, `{"data":{"data":{%q:%q},"metadata":{"version":%d}}}`, baoValueField, v, kv.version[key])

		case !isData && r.Method == http.MethodDelete:
			if _, ok := kv.data[key]; !ok {
				w.WriteHeader(http.StatusNotFound)
				return
			}
			delete(kv.data, key)
			delete(kv.version, key)
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
	return NewBaoClient(c)
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

// TestBaoClient_RelativePathRejectsWrongMount pins that a logical path
// lacking this client's mount prefix is rejected rather than silently
// stripped/misrouted. This does NOT cover the mount-disagreement case (a
// BaoClient's mount differing from the underlying bao.Client's own
// configured mount) — that class of bug is now structurally impossible
// because NewBaoClient derives its mount from c.Mount() instead of
// accepting a separate value; see TestBaoClient_UsesClientsConfiguredMount.
func TestBaoClient_RelativePathRejectsWrongMount(t *testing.T) {
	srv := newFakeBaoServer(t, newFakeKV(), nil)
	bc := newTestBaoClient(t, srv.URL)

	err := bc.CreateOrAddVersion(t.Context(), "secret/mark8ly/foo", []byte("x"))
	if err == nil {
		t.Fatal("expected an error for a path outside this client's mount")
	}
}

// TestBaoClient_UsesClientsConfiguredMount proves BaoClient derives its
// mount from the underlying bao.Client (via Mount()) rather than assuming
// the default "kv". A bao.Client configured with a non-default mount
// ("secret") must route a write to "secret/data/...", not "kv/data/...".
//
// This is the regression test for the mount double-configuration risk: a
// prior version of NewBaoClient took its own mount parameter independent of
// the bao.Client's Config.Mount, so a caller could construct a BaoClient
// whose mount ("kv") silently disagreed with the bao.Client it wrapped
// (e.g. "secret") — every write would land in the wrong mount with no
// error. Deriving the mount from c.Mount() makes that disagreement
// impossible to express.
func TestBaoClient_UsesClientsConfiguredMount(t *testing.T) {
	kv := newFakeKV()
	var requests []string
	srv := newFakeBaoServer(t, kv, &requests)

	c, err := bao.New(bao.Config{
		Address: srv.URL,
		Mount:   "secret",
		Token:   "test-token",
	})
	if err != nil {
		t.Fatalf("bao.New: %v", err)
	}
	bc := NewBaoClient(c)

	// BaoPath always emits "kv/..." paths; a client mounted at "secret"
	// naturally only ever receives "secret/..." paths from real callers.
	// This input is shaped to match the client's own mount, not BaoPath's
	// output — the point here is proving BaoClient followed c.Mount(),
	// not that BaoPath and a non-default mount coexist.
	const path = "secret/mark8ly/marketplace-api/tenants/tenant-1/payment/razorpay/api_key"

	if err := bc.CreateOrAddVersion(t.Context(), path, []byte("x")); err != nil {
		t.Fatalf("CreateOrAddVersion: %v", err)
	}

	wantPath := "PUT /v1/secret/data/mark8ly/marketplace-api/tenants/tenant-1/payment/razorpay/api_key"
	found := false
	for _, req := range requests {
		if req == wantPath {
			found = true
		}
		if req == "PUT /v1/kv/data/mark8ly/marketplace-api/tenants/tenant-1/payment/razorpay/api_key" {
			t.Fatalf("write landed on the default \"kv\" mount instead of the client's configured \"secret\" mount: %v", requests)
		}
	}
	if !found {
		t.Fatalf("write never hit %q; requests observed: %v", wantPath, requests)
	}
}
