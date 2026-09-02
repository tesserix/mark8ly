package webhook

import (
	"bytes"
	"context"
	"crypto/tls"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"time"

	"github.com/mark8ly/marketplace-api/internal/webhook/ssrfguard"
)

// RequestTimeout bounds one delivery attempt. Short on purpose: these loops
// run in-process alongside admin API request handling, so a slow endpoint
// must not hold a goroutine for long.
const RequestTimeout = 5 * time.Second

// maxErrorLen bounds what we store from a failing endpoint's response. The
// body is surfaced to the merchant to make a broken endpoint debuggable; it
// is never logged server-side, since it is arbitrary remote content.
const maxErrorLen = 500

type Sender struct {
	guard  *ssrfguard.Guard
	client *http.Client
}

func NewSender(guard *ssrfguard.Guard, client *http.Client) *Sender {
	if client == nil {
		client = &http.Client{Timeout: RequestTimeout}
	}
	if client.Transport == nil {
		if dt, ok := http.DefaultTransport.(*http.Transport); ok {
			client.Transport = dt.Clone()
		}
	}
	if client.CheckRedirect == nil {
		// Never follow redirects: a 302 to an internal address would walk
		// straight around the guard, which only checked the original host.
		client.CheckRedirect = func(*http.Request, []*http.Request) error {
			return http.ErrUseLastResponse
		}
	}
	return &Sender{guard: guard, client: client}
}

// notification is the notify-and-fetch body. Identifiers only — the merchant
// calls the REST API for detail, so no customer data reaches a
// merchant-supplied URL and a retry cannot deliver a stale entity.
type notification struct {
	Event      string    `json:"event"`
	ID         string    `json:"id"`
	OccurredAt time.Time `json:"occurred_at"`
}

// Send makes one attempt. It re-checks the URL through the guard FIRST:
// registration-time validation alone is defeated by DNS rebinding.
//
// The connection itself is then pinned to the address the guard just
// validated (see pinnedTransport) rather than left to the transport's own,
// separate DNS lookup at dial time — otherwise a rebind between the two
// resolutions defeats the check entirely instead of merely narrowing its
// window.
func (s *Sender) Send(ctx context.Context, sub Subscription, d Delivery) (int, error) {
	u, ips, err := s.guard.CheckResolved(sub.URL)
	if err != nil {
		return 0, fmt.Errorf("webhook: destination refused: %w", err)
	}

	occurred := d.CreatedAt
	if occurred.IsZero() {
		occurred = time.Now()
	}
	body, err := json.Marshal(notification{
		Event:      d.EventType,
		ID:         d.AggregateID.String(),
		OccurredAt: occurred.UTC(),
	})
	if err != nil {
		return 0, fmt.Errorf("webhook: marshal notification: %w", err)
	}

	reqCtx, cancel := context.WithTimeout(ctx, RequestTimeout)
	defer cancel()
	req, err := http.NewRequestWithContext(reqCtx, http.MethodPost, sub.URL, bytes.NewReader(body))
	if err != nil {
		return 0, fmt.Errorf("webhook: build request: %w", err)
	}
	now := time.Now()
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("User-Agent", "Mark8ly-Webhooks/1")
	req.Header.Set(SignatureHeader, Sign(sub.Secret, now, body))

	client := s.pinnedClient(u.Hostname(), ips)
	res, err := client.Do(req)
	if err != nil {
		return 0, fmt.Errorf("webhook: request failed: %w", err)
	}
	defer res.Body.Close()
	_, _ = io.CopyN(io.Discard, res.Body, 1<<16)

	if res.StatusCode < 200 || res.StatusCode > 299 {
		return res.StatusCode, fmt.Errorf("webhook: endpoint returned %d", res.StatusCode)
	}
	return res.StatusCode, nil
}

// pinnedClient returns a client that dials one of ips directly rather than
// re-resolving host, while still verifying the TLS certificate — and
// setting SNI — against host. Dialling an IP but checking the certificate
// against the IP instead of the hostname would break every valid HTTPS
// endpoint, since certificates are issued for hostnames, not addresses.
//
// A fresh transport per call (rather than one shared across the sender's
// lifetime) is deliberate: every delivery targets a different merchant
// host and a different validated address, so nothing here is reusable
// across calls the way keep-alive pooling assumes.
func (s *Sender) pinnedClient(host string, ips []net.IP) *http.Client {
	pin := pinnedAddress(host, ips)
	if pin == nil {
		return s.client
	}
	tr := s.baseTransport()
	tr.DialContext = dialPinnedTo(pin)
	tr.TLSClientConfig = cloneTLSConfigForHost(tr.TLSClientConfig, host)
	return &http.Client{
		Transport:     tr,
		Timeout:       s.client.Timeout,
		CheckRedirect: s.client.CheckRedirect,
	}
}

func (s *Sender) baseTransport() *http.Transport {
	if tr, ok := s.client.Transport.(*http.Transport); ok {
		return tr.Clone()
	}
	if dt, ok := http.DefaultTransport.(*http.Transport); ok {
		return dt.Clone()
	}
	return &http.Transport{}
}

// pinnedAddress picks the address to dial for host.
//
// When host is already a literal IP, that literal IS the address the URL
// names — there is no hostname for a transport to re-resolve, so no rebind
// window exists, and the guard's resolver output (a mock, in tests) must
// not override it. Otherwise the guard-validated address is used.
func pinnedAddress(host string, ips []net.IP) net.IP {
	if literal := net.ParseIP(host); literal != nil {
		return literal
	}
	if len(ips) == 0 {
		return nil
	}
	return ips[0]
}

// dialPinnedTo returns a DialContext that connects to ip on the port named
// in addr, ignoring addr's host entirely. That is the pin: whatever the
// transport believes the hostname resolves to, this always dials the
// address the guard already validated.
func dialPinnedTo(ip net.IP) func(ctx context.Context, network, addr string) (net.Conn, error) {
	return func(ctx context.Context, network, addr string) (net.Conn, error) {
		_, port, err := net.SplitHostPort(addr)
		if err != nil {
			port = "443"
		}
		d := net.Dialer{Timeout: RequestTimeout}
		return d.DialContext(ctx, network, net.JoinHostPort(ip.String(), port))
	}
}

// cloneTLSConfigForHost clones base (preserving any configured RootCAs,
// notably a test server's) and points ServerName at host, so certificate
// verification runs against the hostname the merchant registered rather
// than the IP address the connection actually dials.
func cloneTLSConfigForHost(base *tls.Config, host string) *tls.Config {
	var cfg *tls.Config
	if base != nil {
		cfg = base.Clone()
	} else {
		cfg = &tls.Config{}
	}
	cfg.ServerName = host
	return cfg
}

// backoff returns the delay before attempt n (1-based): roughly 30s, 2m,
// 8m, 32m, 2h, capped. Spread over hours so a merchant restarting a server
// has time to recover before attempts are exhausted.
func backoff(attempt int) time.Duration {
	d := 30 * time.Second
	for i := 1; i < attempt; i++ {
		d *= 4
		if d > 4*time.Hour {
			return 4 * time.Hour
		}
	}
	return d
}
