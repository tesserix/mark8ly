// Package stripewebhook provisions a store's Stripe webhook endpoint
// automatically, so a merchant never has to open the Stripe dashboard.
//
// Why this exists: mark8ly uses the direct-key model — each store pastes
// its own Stripe secret key. Payment then succeeds at Stripe while the
// order stays Pending forever unless someone ALSO registers a webhook
// endpoint pointing back at us and copies the signing secret across.
// Nothing surfaced when they hadn't: the webhook handler answers 200 even
// when it rejects an unverifiable event (so Stripe stops retrying), so
// Stripe's dashboard reports delivery as successful while nothing is
// processed. A store could take money and never mark a single order paid.
//
// The key the merchant already gave us can create the endpoint itself.
package stripewebhook

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"strings"
	"time"
)

// DefaultEvents are the events the order lifecycle actually depends on.
// Deliberately narrow: subscribing to everything means paying attention
// to nothing, and every extra event is another row in webhook_events.
var DefaultEvents = []string{
	"checkout.session.completed",
	"payment_intent.succeeded",
	"payment_intent.payment_failed",
	"charge.refunded",
}

// Action describes what Ensure did, for logging and for telling the
// merchant what happened.
type Action string

const (
	// ActionCreated — no endpoint existed for this URL; one was created
	// and its signing secret captured.
	ActionCreated Action = "created"
	// ActionReplaced — an endpoint for this URL existed but we had no
	// signing secret for it. Stripe only returns the secret at creation
	// time and never on read, so the only way to obtain one is to delete
	// and recreate. See Ensure for why that is safe here.
	ActionReplaced Action = "replaced"
	// ActionUnchanged — an endpoint exists and we already hold its
	// secret. Nothing to do.
	ActionUnchanged Action = "unchanged"
)

// Result is the outcome of Ensure. Secret is empty for ActionUnchanged —
// the caller already has it stored.
type Result struct {
	Action Action
	Secret string
	// EndpointID is Stripe's `we_…` id, useful in logs when a merchant
	// asks which endpoint we own.
	EndpointID string
}

// Provisioner talks to Stripe's webhook_endpoints API.
type Provisioner struct {
	httpClient *http.Client
	apiBase    string
}

// New builds a Provisioner against the real Stripe API.
func New() *Provisioner {
	return &Provisioner{
		httpClient: &http.Client{Timeout: 15 * time.Second},
		apiBase:    "https://api.stripe.com",
	}
}

// WithHTTPClient overrides the client (tests point this at httptest).
func (p *Provisioner) WithHTTPClient(c *http.Client) *Provisioner {
	p.httpClient = c
	return p
}

// WithAPIBase overrides the Stripe base URL (tests only).
func (p *Provisioner) WithAPIBase(base string) *Provisioner {
	p.apiBase = strings.TrimRight(base, "/")
	return p
}

// Ensure makes the store's webhook endpoint exist and returns its signing
// secret when one had to be obtained.
//
// haveSecret says whether we already hold a usable signing secret for this
// store. It is what keeps the operation idempotent: without it, every save
// of the payment settings would delete and recreate the endpoint, churning
// the signing secret and dropping any event in flight.
//
// The delete-and-recreate path only ever touches an endpoint whose URL is
// byte-identical to the one we construct for this store, which no other
// system would have reason to register. We never touch a merchant's other
// endpoints.
func (p *Provisioner) Ensure(
	ctx context.Context,
	secretKey, endpointURL string,
	haveSecret bool,
	events []string,
) (Result, error) {
	if strings.TrimSpace(secretKey) == "" {
		return Result{}, fmt.Errorf("stripewebhook: secret key is required")
	}
	if strings.TrimSpace(endpointURL) == "" {
		return Result{}, fmt.Errorf("stripewebhook: endpoint url is required")
	}
	if len(events) == 0 {
		events = DefaultEvents
	}

	existing, err := p.findByURL(ctx, secretKey, endpointURL)
	if err != nil {
		return Result{}, err
	}

	if existing != "" && haveSecret {
		return Result{Action: ActionUnchanged, EndpointID: existing}, nil
	}

	action := ActionCreated
	if existing != "" {
		// Stripe returns `secret` only from the create call, so an
		// endpoint we cannot verify against is worse than no endpoint:
		// every event would be rejected while Stripe reports success.
		if err := p.deleteEndpoint(ctx, secretKey, existing); err != nil {
			return Result{}, err
		}
		action = ActionReplaced
	}

	id, secret, err := p.createEndpoint(ctx, secretKey, endpointURL, events)
	if err != nil {
		return Result{}, err
	}
	return Result{Action: action, Secret: secret, EndpointID: id}, nil
}

type endpointListResponse struct {
	Data []struct {
		ID  string `json:"id"`
		URL string `json:"url"`
	} `json:"data"`
	HasMore bool `json:"has_more"`
}

// findByURL returns the id of an endpoint registered for exactly this URL,
// or "" when none exists.
func (p *Provisioner) findByURL(ctx context.Context, secretKey, endpointURL string) (string, error) {
	// 100 is Stripe's maximum page size. A merchant with more than 100
	// endpoints AND ours beyond the first page would read as "not found"
	// and we would create a duplicate, so paginate rather than assume.
	startingAfter := ""
	for {
		q := "limit=100"
		if startingAfter != "" {
			q += "&starting_after=" + url.QueryEscape(startingAfter)
		}
		var out endpointListResponse
		if err := p.do(ctx, http.MethodGet, "/v1/webhook_endpoints?"+q, secretKey, nil, &out); err != nil {
			return "", fmt.Errorf("stripewebhook: list endpoints: %w", err)
		}
		for _, e := range out.Data {
			if e.URL == endpointURL {
				return e.ID, nil
			}
		}
		if !out.HasMore || len(out.Data) == 0 {
			return "", nil
		}
		startingAfter = out.Data[len(out.Data)-1].ID
	}
}

func (p *Provisioner) deleteEndpoint(ctx context.Context, secretKey, id string) error {
	if err := p.do(ctx, http.MethodDelete, "/v1/webhook_endpoints/"+url.PathEscape(id), secretKey, nil, nil); err != nil {
		return fmt.Errorf("stripewebhook: delete endpoint %s: %w", id, err)
	}
	return nil
}

func (p *Provisioner) createEndpoint(
	ctx context.Context, secretKey, endpointURL string, events []string,
) (id, secret string, err error) {
	form := url.Values{}
	form.Set("url", endpointURL)
	for _, e := range events {
		form.Add("enabled_events[]", e)
	}
	form.Set("description", "mark8ly — order payment events (managed automatically)")

	var out struct {
		ID     string `json:"id"`
		Secret string `json:"secret"`
	}
	body := strings.NewReader(form.Encode())
	if err := p.do(ctx, http.MethodPost, "/v1/webhook_endpoints", secretKey, body, &out); err != nil {
		return "", "", fmt.Errorf("stripewebhook: create endpoint: %w", err)
	}
	if out.Secret == "" {
		// Without a signing secret the endpoint is worse than useless —
		// it would receive events we can never verify.
		return "", "", fmt.Errorf("stripewebhook: create endpoint: response carried no signing secret")
	}
	return out.ID, out.Secret, nil
}

func (p *Provisioner) do(ctx context.Context, method, path, secretKey string, body *strings.Reader, out any) error {
	var req *http.Request
	var err error
	if body != nil {
		req, err = http.NewRequestWithContext(ctx, method, p.apiBase+path, body)
	} else {
		req, err = http.NewRequestWithContext(ctx, method, p.apiBase+path, nil)
	}
	if err != nil {
		return err
	}
	req.Header.Set("Authorization", "Bearer "+secretKey)
	if body != nil {
		req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	}

	resp, err := p.httpClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		var e struct {
			Error struct {
				Message string `json:"message"`
				Code    string `json:"code"`
			} `json:"error"`
		}
		_ = json.NewDecoder(resp.Body).Decode(&e)
		msg := e.Error.Message
		if msg == "" {
			msg = resp.Status
		}
		return fmt.Errorf("stripe %d: %s", resp.StatusCode, msg)
	}
	if out == nil {
		return nil
	}
	return json.NewDecoder(resp.Body).Decode(out)
}

// EndpointURL builds the scoped webhook URL for a store. Mirrors the route
// registered in handlers/storefront/routes.go.
func EndpointURL(publicAPIBase, storeSlug string) string {
	return fmt.Sprintf("%s/api/v1/webhooks/%s/stripe",
		strings.TrimRight(publicAPIBase, "/"), url.PathEscape(storeSlug))
}
