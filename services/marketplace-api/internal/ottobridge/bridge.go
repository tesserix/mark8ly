// Package ottobridge mounts the mobile-facing support-chat routes that
// proxy to the otto service. Both the storefront (customer→merchant) and
// the admin (merchant→platform) surfaces share the exact same wire shape
// with otto — only the tenant/store scope + caller identity differ — so
// the proxy, session-token handling, and ws-ticket augmentation live here
// once and each surface supplies a small Resolver for its scope/identity.
//
// otto's session cookie is HttpOnly, so a native app can't read it. On
// create, otto mints the cookie; the bridge lifts the token out of the
// Set-Cookie header into the JSON body as "session_token" (and echoes it
// as an X-Otto-Session response header). The app stores it and sends it
// back as X-Otto-Session on every later call, which otto accepts in lieu
// of the cookie.
package ottobridge

import (
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/url"

	"github.com/gin-gonic/gin"

	"github.com/mark8ly/marketplace-api/internal/ottoclient"
)

// maxBody caps request/relayed bodies — chat payloads are small.
const maxBody = 1 << 20

// Resolver extracts the otto scope + caller identity for the current
// request, typically from middleware-populated context keys. Returning
// ok=false signals the resolver already wrote an error response and the
// bridge must stop.
type Resolver func(c *gin.Context) (ottoclient.ForwardScope, ottoclient.ForwardIdentity, bool)

// Bridge proxies mobile support-chat calls to otto.
type Bridge struct {
	otto         *ottoclient.Client
	wsPublicBase string
	logger       *slog.Logger
}

// New builds a Bridge. otto may be nil (not wired in local dev) — every
// route then returns 503. wsPublicBase is the public WebSocket origin the
// app dials (e.g. "wss://api.mark8ly.com"); empty derives it per-request.
func New(otto *ottoclient.Client, wsPublicBase string, logger *slog.Logger) *Bridge {
	return &Bridge{otto: otto, wsPublicBase: wsPublicBase, logger: logger}
}

// Mount registers the standard support routes on g, using resolve to
// obtain the per-request otto scope + identity. The group MUST already
// have whatever auth/scope middleware the resolver reads from.
func (b *Bridge) Mount(g *gin.RouterGroup, resolve Resolver) {
	g.POST("/conversations", b.proxy(http.MethodPost, staticPath("/conversations"), resolve))
	g.GET("/resume", b.proxy(http.MethodGet, staticPath("/resume"), resolve))
	g.GET("/conversations/:id", b.proxy(http.MethodGet, convPath(""), resolve))
	g.GET("/conversations/:id/messages", b.proxy(http.MethodGet, convPath("/messages"), resolve))
	g.POST("/conversations/:id/messages", b.proxy(http.MethodPost, convPath("/messages"), resolve))
	g.POST("/conversations/:id/close", b.proxy(http.MethodPost, convPath("/close"), resolve))
	g.POST("/conversations/:id/feedback", b.proxy(http.MethodPost, convPath("/feedback"), resolve))
	g.GET("/conversations/:id/queue", b.proxy(http.MethodGet, convPath("/queue"), resolve))
	g.POST("/conversations/:id/ws-ticket", b.wsTicket(resolve))
}

type pathFunc func(c *gin.Context) string

func staticPath(p string) pathFunc { return func(*gin.Context) string { return p } }

func convPath(suffix string) pathFunc {
	return func(c *gin.Context) string {
		return "/conversations/" + url.PathEscape(c.Param("id")) + suffix
	}
}

func (b *Bridge) proxy(method string, path pathFunc, resolve Resolver) gin.HandlerFunc {
	return func(c *gin.Context) {
		res, ok := b.forward(c, method, path(c), resolve)
		if !ok {
			return
		}
		b.relay(c, res)
	}
}

func (b *Bridge) forward(c *gin.Context, method, path string, resolve Resolver) (*ottoclient.ForwardResponse, bool) {
	if b.otto == nil {
		c.JSON(http.StatusServiceUnavailable, gin.H{"error": "support_unavailable"})
		return nil, false
	}
	scope, identity, ok := resolve(c)
	if !ok {
		return nil, false // resolver already wrote the error
	}

	var body []byte
	if method != http.MethodGet && c.Request.Body != nil {
		body, _ = io.ReadAll(io.LimitReader(c.Request.Body, maxBody))
	}

	res, err := b.otto.Forward(c.Request.Context(), ottoclient.ForwardRequest{
		Method:       method,
		Path:         path,
		Scope:        scope,
		Identity:     identity,
		SessionToken: c.GetHeader("X-Otto-Session"),
		Body:         body,
	})
	if err != nil {
		if b.logger != nil {
			b.logger.Error("ottobridge: forward failed", "err", err, "path", path)
		}
		c.JSON(http.StatusBadGateway, gin.H{"error": "support_unreachable"})
		return nil, false
	}
	return res, true
}

// wsTicket mints an otto WebSocket ticket and augments the response with
// the public ws_url the app dials directly (real-time delivery bypasses
// this BFF and goes straight to otto via the gateway).
func (b *Bridge) wsTicket(resolve Resolver) gin.HandlerFunc {
	return func(c *gin.Context) {
		res, ok := b.forward(c, http.MethodPost, convPath("/ws-ticket")(c), resolve)
		if !ok {
			return
		}
		if res.Status != http.StatusOK {
			b.relay(c, res)
			return
		}
		var minted struct {
			Ticket string `json:"ticket"`
		}
		if err := json.Unmarshal(res.Body, &minted); err != nil || minted.Ticket == "" {
			c.JSON(http.StatusBadGateway, gin.H{"error": "ticket_mint_failed"})
			return
		}
		c.JSON(http.StatusOK, gin.H{
			"ticket": minted.Ticket,
			"ws_url": b.wsURL(c, c.Param("id")),
		})
	}
}

// relay writes otto's response back to the client, surfacing a freshly
// minted session token into the JSON body + an X-Otto-Session header.
func (b *Bridge) relay(c *gin.Context, res *ottoclient.ForwardResponse) {
	contentType := res.ContentType
	if contentType == "" {
		contentType = "application/json"
	}
	body := res.Body
	if res.SessionToken != "" {
		if merged, ok := mergeSessionToken(res.Body, res.SessionToken); ok {
			body = merged
		}
		c.Header("X-Otto-Session", res.SessionToken)
	}
	c.Data(res.Status, contentType, body)
}

// wsURL builds the public WebSocket URL for a conversation, preferring the
// configured public base and otherwise deriving it from the request.
func (b *Bridge) wsURL(c *gin.Context, convID string) string {
	base := b.wsPublicBase
	if base == "" {
		host := c.GetHeader("X-Forwarded-Host")
		if host == "" {
			host = c.Request.Host
		}
		scheme := "wss"
		if c.GetHeader("X-Forwarded-Proto") == "http" {
			scheme = "ws"
		}
		base = scheme + "://" + host
	}
	return base + "/api/v1/storefront/otto/conversations/" + url.PathEscape(convID) + "/ws"
}

// mergeSessionToken injects a top-level "session_token" into a JSON object
// body. Returns false when the body isn't a JSON object.
func mergeSessionToken(body []byte, token string) ([]byte, bool) {
	var obj map[string]json.RawMessage
	if err := json.Unmarshal(body, &obj); err != nil {
		return nil, false
	}
	tok, err := json.Marshal(token)
	if err != nil {
		return nil, false
	}
	obj["session_token"] = tok
	out, err := json.Marshal(obj)
	if err != nil {
		return nil, false
	}
	return out, true
}
