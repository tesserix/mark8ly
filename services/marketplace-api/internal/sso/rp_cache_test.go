package sso

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/google/uuid"
	"gorm.io/datatypes"
)

// discoveryServer stands in for a tenant's IdP, counting how many times its
// discovery document is fetched — which is the cost this cache exists to
// avoid paying per request.
type discoveryServer struct {
	*httptest.Server
	hits atomic.Int32
}

func newDiscoveryServer(t *testing.T) *discoveryServer {
	t.Helper()
	ds := &discoveryServer{}
	mux := http.NewServeMux()
	mux.HandleFunc("/.well-known/openid-configuration", func(w http.ResponseWriter, r *http.Request) {
		ds.hits.Add(1)
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{
			"issuer": "` + ds.URL + `",
			"authorization_endpoint": "` + ds.URL + `/auth",
			"token_endpoint": "` + ds.URL + `/token",
			"jwks_uri": "` + ds.URL + `/jwks"
		}`))
	})
	ds.Server = httptest.NewServer(mux)
	t.Cleanup(ds.Close)
	return ds
}

type stubSecrets struct {
	data map[string]string
	err  error
	// reads counts secret fetches, so a test can prove the plaintext is not
	// re-read on every login.
	reads atomic.Int32
}

func (s *stubSecrets) ReadSecret(_ context.Context, _ string) (map[string]string, int, error) {
	s.reads.Add(1)
	if s.err != nil {
		return nil, 0, s.err
	}
	return s.data, 1, nil
}

// testVaultPath is the KV PATH a config points at, not a secret — the value
// behind it comes from the stub secret store below.
//
// Deliberately named without "secret" in it and kept low-entropy: gitleaks'
// generic-api-key rule fires on a secret-ish identifier sitting next to a
// slashy string, and it does not care that this one is a lookup key rather
// than a credential. Naming it plainly is cheaper than an ignore entry, which
// would also suppress a real finding on this line later.
const testVaultPath = "kv/test/sso"

func testConfig(issuer string) *Config {
	return &Config{
		TenantID: uuid.New(),
		Provider: ProviderOIDC,
		Enabled:  true,
		Metadata: datatypes.JSONMap{
			OIDCKeyIssuer:          issuer,
			OIDCKeyClientID:        "client-abc",
			OIDCKeyClientSecretRef: testVaultPath,
			OIDCKeyRedirectURI:     "https://acme-admin.mark8ly.com/sso/acme/callback",
		},
	}
}

func goodSecrets() *stubSecrets {
	return &stubSecrets{data: map[string]string{OIDCSecretField: "s3cr3t"}}
}

func TestRelyingPartyCache_BuildsOnFirstUse(t *testing.T) {
	ds := newDiscoveryServer(t)
	secrets := goodSecrets()
	c := NewRelyingPartyCache(secrets, 0)

	rp, err := c.For(context.Background(), testConfig(ds.URL))
	if err != nil {
		t.Fatalf("For: %v", err)
	}
	if rp == nil || rp.ClientID != "client-abc" {
		t.Fatalf("rp = %+v", rp)
	}
	if rp.OAuth.ClientSecret != "s3cr3t" {
		t.Errorf("client secret not resolved from the secret store")
	}
	if got := ds.hits.Load(); got != 1 {
		t.Errorf("discovery hits = %d, want 1", got)
	}
}

// The point of caching: discovery is an outbound call to a CUSTOMER's IdP, and
// paying it per login would put their uptime in our request path.
func TestRelyingPartyCache_ReusesAcrossCalls(t *testing.T) {
	ds := newDiscoveryServer(t)
	secrets := goodSecrets()
	c := NewRelyingPartyCache(secrets, 0)
	cfg := testConfig(ds.URL)

	for i := 0; i < 5; i++ {
		if _, err := c.For(context.Background(), cfg); err != nil {
			t.Fatalf("For #%d: %v", i, err)
		}
	}
	if got := ds.hits.Load(); got != 1 {
		t.Errorf("discovery hits = %d, want 1 — the cache is not being used", got)
	}
	if got := secrets.reads.Load(); got != 1 {
		t.Errorf("secret reads = %d, want 1", got)
	}
}

// A merchant who corrects a wrong client secret must not keep failing to log
// in until the TTL expires, while the console shows the corrected value.
func TestRelyingPartyCache_InvalidateForcesARebuild(t *testing.T) {
	ds := newDiscoveryServer(t)
	c := NewRelyingPartyCache(goodSecrets(), 0)
	cfg := testConfig(ds.URL)

	if _, err := c.For(context.Background(), cfg); err != nil {
		t.Fatalf("For: %v", err)
	}
	c.Invalidate(cfg.TenantID)
	if _, err := c.For(context.Background(), cfg); err != nil {
		t.Fatalf("For after invalidate: %v", err)
	}
	if got := ds.hits.Load(); got != 2 {
		t.Errorf("discovery hits = %d, want 2 — invalidate did not force a rebuild", got)
	}
}

// The TTL bounds the DISCOVERY document, not the secret: an IdP that rotates
// signing keys is picked up without an operator doing anything.
func TestRelyingPartyCache_RebuildsAfterTheTTL(t *testing.T) {
	ds := newDiscoveryServer(t)
	c := NewRelyingPartyCache(goodSecrets(), time.Minute)
	cfg := testConfig(ds.URL)

	now := time.Now()
	c.now = func() time.Time { return now }

	if _, err := c.For(context.Background(), cfg); err != nil {
		t.Fatalf("For: %v", err)
	}
	now = now.Add(2 * time.Minute)
	if _, err := c.For(context.Background(), cfg); err != nil {
		t.Fatalf("For after ttl: %v", err)
	}
	if got := ds.hits.Load(); got != 2 {
		t.Errorf("discovery hits = %d, want 2 — the entry never expired", got)
	}
}

// A burst of logins after a config change must not fan out into one discovery
// call per request against the customer's IdP.
func TestRelyingPartyCache_ConcurrentMissesCoalesce(t *testing.T) {
	ds := newDiscoveryServer(t)
	c := NewRelyingPartyCache(goodSecrets(), 0)
	cfg := testConfig(ds.URL)

	var wg sync.WaitGroup
	for i := 0; i < 20; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			if _, err := c.For(context.Background(), cfg); err != nil {
				t.Errorf("For: %v", err)
			}
		}()
	}
	wg.Wait()

	if got := ds.hits.Load(); got != 1 {
		t.Errorf("discovery hits = %d, want 1 — 20 concurrent logins each hit the IdP", got)
	}
}

// "Your IdP is unreachable" and "your secret is missing" need different fixes,
// so they must not arrive as the same error.
func TestRelyingPartyCache_MissingSecretIsItsOwnError(t *testing.T) {
	ds := newDiscoveryServer(t)

	for _, tc := range []struct {
		name    string
		secrets *stubSecrets
	}{
		{"secret store unreachable", &stubSecrets{err: errors.New("bao down")}},
		{"secret has no client_secret field", &stubSecrets{data: map[string]string{"other": "x"}}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			c := NewRelyingPartyCache(tc.secrets, 0)
			_, err := c.For(context.Background(), testConfig(ds.URL))
			if !errors.Is(err, ErrNoClientSecret) {
				t.Fatalf("err = %v, want ErrNoClientSecret", err)
			}
		})
	}
}

func TestRelyingPartyCache_NoSecretStoreFailsRatherThanPanics(t *testing.T) {
	c := NewRelyingPartyCache(nil, 0)
	if _, err := c.For(context.Background(), testConfig("https://idp.example.com")); !errors.Is(err, ErrNoClientSecret) {
		t.Fatalf("err = %v, want ErrNoClientSecret", err)
	}
}

// A failed build must not be cached as a success, or one bad moment would
// wedge a tenant's SSO for the whole TTL.
func TestRelyingPartyCache_AFailedBuildIsNotCached(t *testing.T) {
	ds := newDiscoveryServer(t)
	secrets := &stubSecrets{err: errors.New("bao down")}
	c := NewRelyingPartyCache(secrets, 0)
	cfg := testConfig(ds.URL)

	if _, err := c.For(context.Background(), cfg); err == nil {
		t.Fatal("expected a failure")
	}
	secrets.err = nil
	secrets.data = map[string]string{OIDCSecretField: "s3cr3t"}

	if _, err := c.For(context.Background(), cfg); err != nil {
		t.Fatalf("For after the secret store recovered: %v", err)
	}
}

// One tenant's broken IdP must not affect another's. This is the property the
// startup-map design could not offer at all: there, a single unreachable
// issuer failed the whole service for everyone.
func TestRelyingPartyCache_OneBrokenTenantDoesNotAffectAnother(t *testing.T) {
	ds := newDiscoveryServer(t)
	c := NewRelyingPartyCache(goodSecrets(), 0)

	broken := testConfig("http://127.0.0.1:1/nope")
	if _, err := c.For(context.Background(), broken); err == nil {
		t.Fatal("a broken issuer built successfully")
	}

	working := testConfig(ds.URL)
	if _, err := c.For(context.Background(), working); err != nil {
		t.Fatalf("a working tenant failed because another tenant is broken: %v", err)
	}
}
