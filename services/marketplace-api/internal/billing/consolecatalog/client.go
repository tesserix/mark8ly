package consolecatalog

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"
)

// ErrUnavailable signals the console could not be reached or answered 5xx.
// Callers must never treat it as an empty catalog: "we could not ask" and
// "there are no prices" are different answers, and conflating them would
// leave this service pricing nothing at all.
var ErrUnavailable = errors.New("consolecatalog: console unavailable")

// ErrNotPublished signals the mode has never been published (404).
//
// The console returns 404 rather than 200 with an empty list precisely so
// this case cannot be mistaken for a legitimate priced state — see the
// route's own reasoning in tesserix-home#427.
var ErrNotPublished = errors.New("consolecatalog: mode has never been published")

// maxBody caps what we will read from the console.
const maxBody = 4 << 20

// tokenSkew renews a token this long before it actually expires, so a fetch
// cannot lose a race with expiry mid-flight.
const tokenSkew = 60 * time.Second

// Config is everything needed to read one mode's catalog.
type Config struct {
	CatalogURL   string
	TokenURL     string
	ClientID     string
	ClientSecret string
	// Scope must include both the project-audience scope and the roles
	// scope. Both are load-bearing: the first puts the project in the
	// token's `aud` (the audience check is what proves the token was minted
	// for this route), the second makes the token carry the roles claim
	// without which it verifies but holds no capability.
	Scope string
	Mode  string
}

// Configured reports whether enough is set to attempt a read. An
// unconfigured client is a supported state — the caller falls back to the
// compiled catalog — so this is a question, not a validation error.
func (c Config) Configured() bool {
	return c.CatalogURL != "" && c.TokenURL != "" && c.ClientID != "" && c.ClientSecret != ""
}

// Client reads one mode's catalog from the console.
//
// It holds the last successful response and its ETag so a 304 can be
// answered from memory. That retention is not an optimisation: a 304 has no
// body, and returning an empty catalog for one would be the worst possible
// failure this package could have.
type Client struct {
	cfg    Config
	http   *http.Client
	logger *slog.Logger

	mu        sync.Mutex
	token     string
	tokenExp  time.Time
	etag      string
	lastGood  Catalog
	haveCatal bool
}

func NewClient(cfg Config, logger *slog.Logger) *Client {
	return &Client{
		cfg:    cfg,
		http:   &http.Client{Timeout: 10 * time.Second},
		logger: logger,
	}
}

// Fetch returns the console's current catalog for the configured mode.
//
// It returns an error rather than degrading: how to degrade is the cache
// layer's decision, and a client that hid failures would make the fail-open
// behaviour untestable and invisible in logs.
func (c *Client) Fetch(ctx context.Context) (Catalog, error) {
	tok, err := c.accessToken(ctx)
	if err != nil {
		return Catalog{}, err
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, c.cfg.CatalogURL+"?mode="+url.QueryEscape(c.cfg.Mode), nil)
	if err != nil {
		return Catalog{}, fmt.Errorf("consolecatalog: build request: %w", err)
	}
	req.Header.Set("Authorization", "Bearer "+tok)

	c.mu.Lock()
	etag, lastGood, have := c.etag, c.lastGood, c.haveCatal
	c.mu.Unlock()
	if etag != "" && have {
		req.Header.Set("If-None-Match", etag)
	}

	resp, err := c.http.Do(req)
	if err != nil {
		return Catalog{}, fmt.Errorf("%w: %v", ErrUnavailable, err)
	}
	defer func() { _, _ = io.Copy(io.Discard, resp.Body); _ = resp.Body.Close() }()

	switch {
	case resp.StatusCode == http.StatusNotModified:
		if !have {
			// A 304 without a retained body should be impossible — we only
			// send If-None-Match when we hold one — but returning an empty
			// catalog here would be silent mispricing, so refuse instead.
			return Catalog{}, fmt.Errorf("%w: 304 with no retained catalog", ErrUnavailable)
		}
		return lastGood, nil
	case resp.StatusCode == http.StatusNotFound:
		return Catalog{}, ErrNotPublished
	case resp.StatusCode != http.StatusOK:
		return Catalog{}, fmt.Errorf("%w: status %d", ErrUnavailable, resp.StatusCode)
	}

	var out Catalog
	if err := json.NewDecoder(io.LimitReader(resp.Body, maxBody)).Decode(&out); err != nil {
		return Catalog{}, fmt.Errorf("consolecatalog: decode: %w", err)
	}
	if len(out.Prices) == 0 {
		// The console answers 404 for an unpublished mode, so a 200 with no
		// prices is a contract violation rather than a state to cache.
		return Catalog{}, fmt.Errorf("%w: 200 with an empty price list", ErrUnavailable)
	}

	c.mu.Lock()
	c.etag = resp.Header.Get("ETag")
	c.lastGood, c.haveCatal = out, true
	c.mu.Unlock()
	return out, nil
}

func (c *Client) accessToken(ctx context.Context) (string, error) {
	c.mu.Lock()
	if c.token != "" && time.Now().Before(c.tokenExp.Add(-tokenSkew)) {
		tok := c.token
		c.mu.Unlock()
		return tok, nil
	}
	c.mu.Unlock()

	form := url.Values{}
	form.Set("grant_type", "client_credentials")
	form.Set("client_id", c.cfg.ClientID)
	form.Set("client_secret", c.cfg.ClientSecret)
	form.Set("scope", c.cfg.Scope)

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.cfg.TokenURL, strings.NewReader(form.Encode()))
	if err != nil {
		return "", fmt.Errorf("consolecatalog: build token request: %w", err)
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")

	resp, err := c.http.Do(req)
	if err != nil {
		return "", fmt.Errorf("%w: token: %v", ErrUnavailable, err)
	}
	defer func() { _, _ = io.Copy(io.Discard, resp.Body); _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("%w: token status %d", ErrUnavailable, resp.StatusCode)
	}

	var body struct {
		AccessToken string `json:"access_token"`
		ExpiresIn   int    `json:"expires_in"`
	}
	if err := json.NewDecoder(io.LimitReader(resp.Body, maxBody)).Decode(&body); err != nil {
		return "", fmt.Errorf("consolecatalog: decode token: %w", err)
	}
	if body.AccessToken == "" {
		return "", fmt.Errorf("%w: token response carried no access_token", ErrUnavailable)
	}

	c.mu.Lock()
	c.token = body.AccessToken
	c.tokenExp = time.Now().Add(time.Duration(body.ExpiresIn) * time.Second)
	c.mu.Unlock()
	return body.AccessToken, nil
}
