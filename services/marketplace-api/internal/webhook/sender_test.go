package webhook

import (
	"context"
	"encoding/json"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/mark8ly/marketplace-api/internal/webhook/ssrfguard"
)

// allowAll resolves every host to a public address so httptest servers on
// 127.0.0.1 can be exercised. Production passes nil, which uses real DNS.
func allowAll() *ssrfguard.Guard {
	return ssrfguard.New(func(string) ([]net.IP, error) {
		return []net.IP{net.ParseIP("93.184.216.34")}, nil
	})
}

func TestSend_PostsSignedNotifyAndFetchBody(t *testing.T) {
	var gotBody []byte
	var gotSig string
	srv := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotBody, _ = io.ReadAll(r.Body)
		gotSig = r.Header.Get(SignatureHeader)
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	s := NewSender(allowAll(), srv.Client())
	sub := Subscription{ID: uuid.New(), URL: srv.URL, Secret: "shh"}
	d := Delivery{EventType: "order.placed", AggregateID: uuid.New(), CreatedAt: time.Now()}

	code, err := s.Send(context.Background(), sub, d)
	if err != nil || code != 200 {
		t.Fatalf("code=%d err=%v", code, err)
	}
	if gotSig == "" {
		t.Fatal("delivery was not signed")
	}

	var payload map[string]any
	if err := json.Unmarshal(gotBody, &payload); err != nil {
		t.Fatal(err)
	}
	for _, k := range []string{"event", "id", "occurred_at"} {
		if _, ok := payload[k]; !ok {
			t.Fatalf("payload missing %q: %s", k, gotBody)
		}
	}
	// Notify-and-fetch: the body must NOT carry the entity.
	if len(payload) != 3 {
		t.Fatalf("payload should carry exactly event/id/occurred_at, got %v", payload)
	}
}

// The guard runs immediately before dialling, not only at registration.
// This is the DNS-rebinding case: the row was saved when the host was
// public, and now resolves private.
func TestSend_RefusesWhenTheHostNowResolvesPrivate(t *testing.T) {
	rebind := ssrfguard.New(func(string) ([]net.IP, error) {
		return []net.IP{net.ParseIP("169.254.169.254")}, nil
	})
	s := NewSender(rebind, http.DefaultClient)
	sub := Subscription{ID: uuid.New(), URL: "https://hooks.example.com/x", Secret: "shh"}

	if _, err := s.Send(context.Background(), sub, Delivery{EventType: "order.placed"}); err == nil {
		t.Fatal("expected delivery to a rebound private address to be refused")
	}
}

func TestSend_TreatsNon2xxAsFailureButReturnsTheCode(t *testing.T) {
	srv := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer srv.Close()

	s := NewSender(allowAll(), srv.Client())
	code, err := s.Send(context.Background(), Subscription{URL: srv.URL, Secret: "x"}, Delivery{EventType: "order.placed"})
	if err == nil {
		t.Fatal("non-2xx must be an error")
	}
	if code != 500 {
		t.Fatalf("want the status code surfaced for the merchant log, got %d", code)
	}
}

// TestSend_CapturesTheFailingEndpointsResponseBodyForTheMerchantLog proves
// last_error ends up with more than a bare status code: the endpoint's own
// response body, which is what actually makes a broken endpoint debuggable
// for the merchant (maxErrorLen's doc comment, and migration 000126's
// last_error column comment, both promise this).
func TestSend_CapturesTheFailingEndpointsResponseBodyForTheMerchantLog(t *testing.T) {
	const want = "validation failed: missing signature header"
	srv := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusBadRequest)
		_, _ = w.Write([]byte(want))
	}))
	defer srv.Close()

	s := NewSender(allowAll(), srv.Client())
	_, err := s.Send(context.Background(), Subscription{URL: srv.URL, Secret: "x"}, Delivery{EventType: "order.placed"})
	if err == nil {
		t.Fatal("non-2xx must be an error")
	}
	if !strings.Contains(err.Error(), want) {
		t.Fatalf("error should carry the endpoint's response body so the merchant log is debuggable, got: %v", err)
	}
}

// TestSend_TruncatesAnOverlongResponseBody proves the capture is bounded by
// maxErrorLen, not just "whatever the endpoint sent" — an unbounded body
// from a merchant-controlled endpoint must not be stored as-is.
func TestSend_TruncatesAnOverlongResponseBody(t *testing.T) {
	huge := strings.Repeat("x", maxErrorLen*4)
	srv := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
		_, _ = w.Write([]byte(huge))
	}))
	defer srv.Close()

	s := NewSender(allowAll(), srv.Client())
	_, err := s.Send(context.Background(), Subscription{URL: srv.URL, Secret: "x"}, Delivery{EventType: "order.placed"})
	if err == nil {
		t.Fatal("non-2xx must be an error")
	}
	if len(err.Error()) >= len(huge) {
		t.Fatalf("captured body must be bounded by maxErrorLen, got a %d-byte error", len(err.Error()))
	}
}

// TestNewSender_DoesNotMutateTheCallersClient proves NewSender takes a
// shallow copy: a caller's *http.Client (e.g. one shared with other code,
// or a test's srv.Client()) must not silently inherit the no-redirect
// policy NewSender sets up for itself.
func TestNewSender_DoesNotMutateTheCallersClient(t *testing.T) {
	client := &http.Client{}
	_ = NewSender(allowAll(), client)
	if client.CheckRedirect != nil {
		t.Fatal("NewSender must not mutate the caller's http.Client in place")
	}
}

func TestBackoff_GrowsAndIsBounded(t *testing.T) {
	prev := time.Duration(0)
	for a := 1; a <= MaxAttempts; a++ {
		d := backoff(a)
		if d <= prev {
			t.Fatalf("backoff must increase: attempt %d gave %v after %v", a, d, prev)
		}
		if d > 4*time.Hour {
			t.Fatalf("backoff unbounded at attempt %d: %v", a, d)
		}
		prev = d
	}
}

// TestDialPinnedTo_DialsTheGivenIPRegardlessOfAddrsHost proves the sender's
// dial-pinning mechanism does what it claims: the returned DialContext
// connects to the IP it was built with, not to whatever host the caller
// (the http.Transport, in production) passes in addr.
//
// The addr host used here, "definitely-not-a-real-host.invalid", is chosen
// from the IANA-reserved .invalid TLD (RFC 6761) specifically so that it
// cannot resolve — if dialPinnedTo re-resolved addr's host the way a plain
// net.Dialer would, this would fail with "no such host". It succeeds only
// because the pin bypasses that resolution and connects to ip directly,
// which is exactly the DNS-rebinding protection sender.go depends on: the
// address dialled is the one the guard already validated, not a second,
// independent lookup that an attacker's rebind could answer differently.
func TestDialPinnedTo_DialsTheGivenIPRegardlessOfAddrsHost(t *testing.T) {
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen: %v", err)
	}
	defer ln.Close()

	accepted := make(chan struct{})
	go func() {
		conn, err := ln.Accept()
		if err == nil {
			close(accepted)
			conn.Close()
		}
	}()

	_, port, err := net.SplitHostPort(ln.Addr().String())
	if err != nil {
		t.Fatalf("split listener addr: %v", err)
	}

	dial := dialPinnedTo(net.ParseIP("127.0.0.1"))
	addr := net.JoinHostPort("definitely-not-a-real-host.invalid", port)
	conn, err := dial(context.Background(), "tcp", addr)
	if err != nil {
		t.Fatalf("pinned dial should ignore addr's unresolvable host and connect to the pinned IP, got: %v", err)
	}
	defer conn.Close()

	select {
	case <-accepted:
	case <-time.After(2 * time.Second):
		t.Fatal("listener never accepted a connection from the pinned dial")
	}
}

// TestPinnedAddress_PrefersTheLiteralHostOverResolverOutput documents why
// pinnedClient does not blindly use whatever the guard's resolver returned:
// when the URL already names a literal IP (as every httptest server URL
// does), that literal IS the address — there is no hostname for a
// transport to re-resolve, so nothing to pin against besides the literal
// itself, regardless of what a resolver mock happens to answer for it.
func TestPinnedAddress_PrefersTheLiteralHostOverResolverOutput(t *testing.T) {
	got := pinnedAddress("127.0.0.1", []net.IP{net.ParseIP("93.184.216.34")})
	if !got.Equal(net.ParseIP("127.0.0.1")) {
		t.Fatalf("want literal host address pinned, got %v", got)
	}
}
