package consolepromo

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
//
// It must never be treated as an empty catalog: "we could not ask" and
// "there are no promo codes" are different answers, and conflating them here
// would expire every console-sourced code in the table on the first network
// blip.
var ErrUnavailable = errors.New("consolepromo: console unavailable")

// ErrNotPublished signals the mode has never been published (404). Like
// ErrUnavailable it is not an empty catalog, and for the same reason.
var ErrNotPublished = errors.New("consolepromo: mode has never been published")

// maxBody caps what we will read from the console.
const maxBody = 4 << 20

// tokenSkew renews a token this long before it actually expires, so a fetch
// cannot lose a race with expiry mid-flight.
const tokenSkew = 60 * time.Second

// Config is everything needed to read one mode's promo catalog.
//
// # Only the URL is new
//
// Every credential here is the one internal/billing/consolecatalog already
// uses. The console gates the promo route on the `read-promo-catalog`
// capability, which rides in the token's roles claim, and one machine
// identity can hold that capability alongside the plan catalog's — so a
// second OAuth client would be a second thing to rotate, expire and get
// wrong for no gain. main.go builds this from the same CONSOLE_CATALOG_*
// settings plus CONSOLE_PROMO_CATALOG_URL.
type Config struct {
	CatalogURL   string
	TokenURL     string
	ClientID     string
	ClientSecret string
	// Scope must include both the project-audience scope and the roles
	// scope; see consolecatalog.Config.Scope for why both are load-bearing.
	Scope string
	Mode  string
}

// Configured reports whether enough is set to attempt a read.
//
// An unconfigured client is a SUPPORTED state, not an error: no URL means no
// ingest, the service starts exactly as it did before this existed, and any
// rows a previous ingest wrote stay valid. This mirrors
// consolecatalog.Config.Configured deliberately — enabling and disabling the
// ingest is a config change with no second code path.
func (c Config) Configured() bool {
	return c.CatalogURL != "" && c.TokenURL != "" && c.ClientID != "" && c.ClientSecret != ""
}

// Fetcher is the console read the Syncer needs. Narrow on purpose: it is
// what lets the sync logic be tested without a network.
type Fetcher interface {
	Fetch(ctx context.Context) (Catalog, error)
}

// Client reads one mode's promo catalog from the console.
//
// It retains the last successful body and its ETag so a 304 can be answered
// from memory. That retention is not an optimisation: a 304 carries no body,
// and answering one with an empty catalog would expire every code.
type Client struct {
	cfg    Config
	http   *http.Client
	logger *slog.Logger

	mu       sync.Mutex
	token    string
	tokenExp time.Time
	etag     string
	lastGood Catalog
	haveCat  bool
}

// NewClient builds a Client. It performs no I/O.
func NewClient(cfg Config, logger *slog.Logger) *Client {
	return &Client{
		cfg:    cfg,
		http:   &http.Client{Timeout: 10 * time.Second},
		logger: logger,
	}
}

// Fetch returns the console's current promo catalog for the configured mode.
//
// It returns an error rather than degrading; how to degrade is the caller's
// decision, and here the caller's degradation is simply "change nothing",
// which is the safest possible response to an unreadable catalog.
//
// # One deliberate difference from consolecatalog.Client
//
// A 200 carrying zero codes is ACCEPTED here, where consolecatalog treats it
// as a contract violation. The reasoning differs because the data differs: a
// mode with no prices cannot sell anything, so an empty price list is
// necessarily a bug; a mode with no promo codes is an ordinary state that
// every deployment starts in and returns to whenever the last campaign ends.
// Refusing it would make "no campaigns running" indistinguishable from an
// outage.
func (c *Client) Fetch(ctx context.Context) (Catalog, error) {
	tok, err := c.accessToken(ctx)
	if err != nil {
		return Catalog{}, err
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodGet,
		c.cfg.CatalogURL+"?mode="+url.QueryEscape(c.cfg.Mode), nil)
	if err != nil {
		return Catalog{}, fmt.Errorf("consolepromo: build request: %w", err)
	}
	req.Header.Set("Authorization", "Bearer "+tok)

	c.mu.Lock()
	etag, lastGood, have := c.etag, c.lastGood, c.haveCat
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
			// Impossible by construction — If-None-Match is only sent when a
			// body is held — but answering with an empty catalog would
			// expire every code, so refuse instead.
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
		return Catalog{}, fmt.Errorf("consolepromo: decode: %w", err)
	}

	c.mu.Lock()
	c.etag = resp.Header.Get("ETag")
	c.lastGood, c.haveCat = out, true
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
		return "", fmt.Errorf("consolepromo: build token request: %w", err)
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
		return "", fmt.Errorf("consolepromo: decode token: %w", err)
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
