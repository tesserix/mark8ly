package ottoclient

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"net/http"
)

// sessionCookieName is the cookie otto mints for a customer chat session.
// It matches otto's CUSTOMER_SESSION_COOKIE default. The mobile app can't
// read otto's HttpOnly cookie, so the BFF lifts the token out of the
// Set-Cookie header (see Forward) and hands it back in the JSON body; the
// app then echoes it via X-Otto-Session on subsequent calls.
const sessionCookieName = "otto_session"

// ForwardScope is the otto tenant+store a forwarded request runs under.
// For the per-merchant storefront surface these are the resolved store's
// tenant_id/store_id; for platform support they are the fixed
// "platform"/"default" pair.
type ForwardScope struct {
	TenantID string
	StoreID  string
}

// ForwardIdentity is the verified caller otto attributes the conversation
// to. A non-empty UserID makes otto treat the caller as a logged-in
// customer and skip the anonymous OTP step. ClientTenant* are only set on
// the platform-support surface so the Tesserix agent can see which
// merchant raised the chat.
type ForwardIdentity struct {
	UserID           string
	UserEmail        string
	UserName         string
	ClientTenantID   string
	ClientTenantName string
}

// ForwardRequest is one proxied call to otto's storefront HTTP surface
// (`/api/v1/storefront/otto`). Path is relative to that prefix, e.g.
// "/conversations" or "/conversations/{id}/messages".
type ForwardRequest struct {
	Method string
	Path   string
	Scope  ForwardScope
	// Identity is the verified caller; the zero value forwards an
	// anonymous request (otto then requires an OTP on create).
	Identity ForwardIdentity
	// SessionToken is the opaque otto session the app captured from a
	// prior create. Empty on the first call. Sent as X-Otto-Session.
	SessionToken string
	// Body is the raw JSON request body, or nil for GET/DELETE.
	Body []byte
}

// ForwardResponse relays otto's response verbatim. SessionToken is the
// freshly minted session lifted from a Set-Cookie header (non-empty only
// on the create call); the handler surfaces it to the mobile client.
type ForwardResponse struct {
	Status       int
	Body         []byte
	ContentType  string
	SessionToken string
}

// Forward proxies a single request to otto's storefront surface, injecting
// the internal-auth secret, tenant/store scope, and caller identity that
// otto's middleware expects. Non-2xx responses are relayed as data (not Go
// errors) so the BFF can pass otto's status + body straight through to the
// app; only transport failures return an error.
func (c *Client) Forward(ctx context.Context, in ForwardRequest) (*ForwardResponse, error) {
	url := c.baseURL + "/api/v1/storefront/otto" + in.Path

	var bodyReader io.Reader
	if len(in.Body) > 0 {
		bodyReader = bytes.NewReader(in.Body)
	}
	req, err := http.NewRequestWithContext(ctx, in.Method, url, bodyReader)
	if err != nil {
		return nil, fmt.Errorf("otto forward: build request: %w", err)
	}

	req.Header.Set("Accept", "application/json")
	if len(in.Body) > 0 {
		req.Header.Set("Content-Type", "application/json")
	}
	req.Header.Set("X-Internal-Auth", c.secret)
	req.Header.Set("X-Tenant-Id", in.Scope.TenantID)
	req.Header.Set("X-Store-Id", in.Scope.StoreID)
	if in.Identity.UserID != "" {
		req.Header.Set("X-User-Id", in.Identity.UserID)
	}
	if in.Identity.UserEmail != "" {
		req.Header.Set("X-User-Email", in.Identity.UserEmail)
	}
	if in.Identity.UserName != "" {
		req.Header.Set("X-User-Name", in.Identity.UserName)
	}
	if in.Identity.ClientTenantID != "" {
		req.Header.Set("X-Client-Tenant-Id", in.Identity.ClientTenantID)
	}
	if in.Identity.ClientTenantName != "" {
		req.Header.Set("X-Client-Tenant-Name", in.Identity.ClientTenantName)
	}
	if in.SessionToken != "" {
		req.Header.Set("X-Otto-Session", in.SessionToken)
	}

	resp, err := c.hc.Do(req)
	if err != nil {
		return nil, fmt.Errorf("otto forward: request: %w", err)
	}
	defer resp.Body.Close()

	// Cap the relayed body — chat payloads are small; a runaway upstream
	// shouldn't be able to balloon BFF memory.
	body, err := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if err != nil {
		return nil, fmt.Errorf("otto forward: read body: %w", err)
	}

	out := &ForwardResponse{
		Status:      resp.StatusCode,
		Body:        body,
		ContentType: resp.Header.Get("Content-Type"),
	}
	for _, ck := range resp.Cookies() {
		if ck.Name == sessionCookieName && ck.Value != "" {
			out.SessionToken = ck.Value
			break
		}
	}
	return out, nil
}
