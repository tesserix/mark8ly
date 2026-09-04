# `mcp-catalog` Connector Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** mark8ly's first MCP server it actually owns — five read-only catalog tools over the storefront API, built on `go-shared/mcp` v1.10.0.

**Architecture:** A new Go module at `services/mcp`, one binary `cmd/mcp-catalog`. `go-shared/mcp` supplies the closed-schema registry, the GET-only upstream client, `X-MCP-Key` auth and per-tool metrics; `internal/server` binds that to the official MCP SDK; `internal/catalog` holds the five tools and the projection from marketplace-api's storefront DTOs into closed, agent-facing result types.

**Tech Stack:** Go 1.26, `github.com/tesserix/go-shared/mcp` v1.10.0, `github.com/modelcontextprotocol/go-sdk` v1.7.0+ (the only new dependency), stdlib `net/http`.

**Spec:** `docs/superpowers/specs/2026-09-04-mark8ly-mcp-connectors-design.md` — this plan is step 2 of its sequencing. Step 1 (`go-shared/mcp`) is done and released.

**Worktree:** `/Users/Mahesh.Sangawar/personal/tesserix-new/m8-wt-mcp-catalog`, branch `feat/mcp-catalog`.

## Global Constraints

- **Read-only.** Every upstream call goes through `upstream.Client.Get`. No other verb is reachable, by construction.
- **Closed results.** Every tool's result is a declared Go struct. Never return an upstream DTO or a `map[string]any` — the projection is the point (spec D5).
- **Three fields must never reach an agent:** `tax_code`, `tax_rate_override`, `tax_category` on the product DTO. They are cart-mechanics for the storefront to echo back on checkout, not facts about a product. They are excluded by being absent from our structs, not by a filter someone must maintain.
- **Store scoping is by SLUG only.** No tool accepts a `store_id`, an internal UUID, or a tenant id.
- `ZITADEL`-style secret hygiene: every secret-sourced config value is `TrimSpace`d **on assignment**. A trailing newline from a mounted secret caused a ~25-hour outage in this codebase before.
- No token, key, or bearer value in any log line.
- The MCP SDK is the ONLY new module dependency. It lives here, never in go-shared (spec D9).
- `go build ./... && go vet ./... && go test -race ./...` must pass in `services/mcp`.
- Commit messages: single-line subject, conventional-commit prefix, **no body, no trailers of any kind.**

---

## What the audit found, and why it changes the shape

Verified against the handlers on 2026-09-04, not read from the spec.

**The existing OpenAPI document is wrong in three ways**, which is a second and independent reason the ingestion route was never going to work — beyond #412's unreachable URL:

| The spec says | The handler actually does |
| --- | --- |
| `limit` and `offset` query params | `page` and `page_size` (`listPublishedQuery`, `products.go:51`), `page_size` capped at 100 |
| categories return "product counts" | `StorefrontCategoryResponse` is `{name, slug, position, featured}` — **no count exists** |
| operations return bare payloads | list endpoints wrap in `{"data": […]}`, paginated ones add `{"meta": {page, page_size}}` |

Had #412's URL been correct, the generated tools would have sent `limit`/`offset`, marketplace-api would have ignored them, and **every call would have silently returned page 1**. Tools derived from a stale hand-written document are wrong in ways nobody sees; that is what this plan replaces.

**Do not copy parameter names from the OpenAPI file.** Read the handler.

---

## File Structure

| File | Responsibility |
| --- | --- |
| `services/mcp/go.mod` | new module `github.com/mark8ly/mcp` |
| `services/mcp/internal/config/config.go` | env → typed config, trimmed and validated, fail-closed |
| `services/mcp/internal/catalog/client.go` | typed calls to the 5 storefront endpoints (correct params, envelope unwrapping) |
| `services/mcp/internal/catalog/types.go` | the closed, agent-facing result structs |
| `services/mcp/internal/catalog/project.go` | upstream DTO → result struct |
| `services/mcp/internal/catalog/tools.go` | the 5 tool registrations |
| `services/mcp/internal/server/server.go` | go-shared registry → MCP SDK wiring, stateless HTTP, auth, metrics |
| `services/mcp/cmd/mcp-catalog/main.go` | startup, graceful shutdown |
| `services/mcp/Dockerfile` | multi-stage, matching the other services |

---

### Task 1: A walking skeleton that proves the SDK wiring

**This task exists because of a genuine unknown, and it must be resolved with running code before the other four tools are written.**

`go-shared/mcp` derives closed schemas from Go types via `Register[In, Out]`. The MCP SDK ALSO derives schemas from type parameters via `AddTool[In, Out](s *Server, t *Tool, h ToolHandlerFor[In, Out])`. Two derivations of the same thing is a fork waiting to happen: the SDK's `jsonschema.Schema` may not set `additionalProperties:false`, which is the entire guarantee our registry exists to provide.

**Decide, with evidence, which is authoritative** and record the decision in `internal/server/server.go`'s doc comment. The two candidates:

- **A — go-shared authoritative.** Build the SDK `Tool` with `InputSchema`/`OutputSchema` set explicitly from our registry's maps (JSON round-trip into `*jsonschema.Schema`), so the SDK serves OUR closed schema. One source of truth; costs a conversion.
- **B — SDK authoritative for the wire, go-shared for the contract.** Use `AddTool`'s derivation for serving, and keep our registry as the declared contract the parity test compares against. Two derivations, but their disagreement becomes a detectable bug rather than an invisible one.

**Prefer A if the conversion works**, because a served schema that is not closed silently breaks D5. Fall back to B only if the SDK overrides or rejects an explicitly-set schema — and if so, add a test asserting the SDK-served schema carries `additionalProperties:false`, so the guarantee is verified rather than assumed.

**Files:**
- Create: `services/mcp/go.mod`, `services/mcp/internal/server/server.go`, `services/mcp/cmd/mcp-catalog/main.go`
- Test: `services/mcp/internal/server/server_test.go`

**Interfaces:**
- Consumes: `go-shared/mcp` (`Registry`, `Register`, `Tool`), `go-shared/mcp/auth.RequireKey`, `go-shared/mcp/observe`.
- Produces: `server.New(reg *mcp.Registry, key string, m *observe.ToolMetrics) (http.Handler, error)` — an HTTP handler serving MCP over streamable HTTP, key-gated.

- [ ] **Step 1: Initialise the module and pin the dependencies**

```bash
cd services/mcp
go mod init github.com/mark8ly/mcp
go get github.com/tesserix/go-shared@v1.10.0
go get github.com/modelcontextprotocol/go-sdk@latest
go mod tidy
```
Record the resolved SDK version in the report. It MUST be v1.7.0 or later — earlier versions do not speak `2026-07-28`.

- [ ] **Step 2: Write the failing test**

```go
package server

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
	gsmcp "github.com/tesserix/go-shared/mcp"
	"github.com/tesserix/go-shared/mcp/auth"
	"github.com/tesserix/go-shared/mcp/observe"
	"github.com/prometheus/client_golang/prometheus"
)

type pingIn struct {
	Echo string `json:"echo" desc:"Text to echo back"`
}

type pingOut struct {
	Echo string `json:"echo"`
}

func testRegistry(t *testing.T) *gsmcp.Registry {
	t.Helper()
	r := gsmcp.NewRegistry()
	require.NoError(t, gsmcp.Register(r, "ping", "Echo a string back.",
		func(_ context.Context, in pingIn) (pingOut, error) {
			return pingOut{Echo: in.Echo}, nil
		}))
	return r
}

// An unauthenticated call must never reach the MCP machinery.
func TestServer_RejectsMissingKey(t *testing.T) {
	m, err := observe.NewToolMetrics(prometheus.NewRegistry(), "mcp-catalog")
	require.NoError(t, err)

	h, err := New(testRegistry(t), "s3cret", m)
	require.NoError(t, err)

	req := httptest.NewRequest(http.MethodPost, "/mcp", strings.NewReader(`{}`))
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)

	require.Equal(t, http.StatusUnauthorized, rec.Code)
}

// The served tool list must carry the tool the registry declares, and its
// input schema must be CLOSED. If the SDK serves its own derivation and that
// derivation is open, D5 is broken on the wire regardless of what our registry
// holds — this is the assertion that catches it.
func TestServer_ServesClosedSchemaFromRegistry(t *testing.T) {
	m, err := observe.NewToolMetrics(prometheus.NewRegistry(), "mcp-catalog")
	require.NoError(t, err)

	h, err := New(testRegistry(t), "s3cret", m)
	require.NoError(t, err)

	body := `{"jsonrpc":"2.0","id":1,"method":"tools/list","params":{"_meta":{` +
		`"io.modelcontextprotocol/protocolVersion":"2026-07-28",` +
		`"io.modelcontextprotocol/clientCapabilities":{}}}}`

	req := httptest.NewRequest(http.MethodPost, "/mcp", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json, text/event-stream")
	req.Header.Set(auth.HeaderName, "s3cret")
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)

	require.Equal(t, http.StatusOK, rec.Code, "body: %s", rec.Body.String())
	require.Contains(t, rec.Body.String(), `"ping"`)

	// Dig the served input schema out of the response and assert it is closed.
	// The exact envelope depends on the SDK; parse defensively and fail with
	// the body on mismatch so the shape is visible when this first runs.
	var payload map[string]any
	raw := rec.Body.String()
	if i := strings.Index(raw, "{"); i >= 0 {
		require.NoError(t, json.Unmarshal([]byte(raw[i:]), &payload), "body: %s", raw)
	}
	require.Contains(t, raw, `"additionalProperties":false`,
		"the SERVED schema must be closed, not merely the registry's copy; body: %s", raw)
}
```

- [ ] **Step 3: Run the tests and watch them fail**

Run: `cd services/mcp && go test ./internal/server/ -run TestServer -v`
Expected: FAIL — `New` is undefined.

- [ ] **Step 4: Implement `server.New`**

Wire, in this order:
1. Build an SDK server: `mcpsdk.NewServer(&mcpsdk.Implementation{Name: "mark8ly-catalog", Version: <build version>}, nil)`.
2. For each `gsmcp.Tool` in the registry, register it with the SDK. Take approach **A** first — construct the SDK `Tool` with `InputSchema`/`OutputSchema` converted from our `map[string]any` (marshal to JSON, unmarshal into `*jsonschema.Schema`).
3. Wrap the handler so each call records `observe.Observe(toolName, observe.OutcomeFor(err), elapsed)`. `OutcomeFor` already maps the upstream sentinels — do not write another switch.
4. Build the HTTP handler: `mcpsdk.NewStreamableHTTPHandler(func(*http.Request) *mcpsdk.Server { return srv }, &mcpsdk.StreamableHTTPOptions{Stateless: true})`.
   **`Stateless: true` is mandatory, not a preference** — the SDK accepts protocol `2026-07-28` on streamable HTTP ONLY in stateless mode; a stateful handler rejects that version outright. It also means the server cannot make client-initiated requests, which costs us nothing: v1 is read-only and no catalog tool calls back.
5. Wrap the whole thing in `auth.RequireKey(key, h)` so the key gate runs BEFORE any MCP parsing.

Return an error rather than panicking if the registry is empty or the key is blank.

- [ ] **Step 5: Run the tests and make them pass**

Run: `cd services/mcp && go test -race ./internal/server/ -v`
Expected: PASS. If `TestServer_ServesClosedSchemaFromRegistry` cannot pass under approach A, switch to B, keep the assertion, and record why in the doc comment and the report.

- [ ] **Step 6: Commit**

```bash
git add services/mcp
git commit -m "feat(mcp-catalog): serve the go-shared registry over stateless MCP"
```

---

### Task 2: Config, trimmed and fail-closed

**Files:**
- Create: `services/mcp/internal/config/config.go`
- Test: `services/mcp/internal/config/config_test.go`

**Interfaces:**
- Produces: `config.Load() (Config, error)` with fields `StorefrontBaseURL`, `StorefrontKey`, `MCPKey`, `Port`, `UpstreamTimeout`.

- [ ] **Step 1: Write the failing test**

```go
package config

import (
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestLoad_TrimsSecretsOnAssignment(t *testing.T) {
	t.Setenv("STOREFRONT_BASE_URL", "http://mark8ly-marketplace-api-storefront.mark8ly.svc.cluster.local:8080\n")
	t.Setenv("STOREFRONT_KEY", "  sfkey\n")
	t.Setenv("MCP_AUTH_KEY", "mcpkey\n")

	cfg, err := Load()
	require.NoError(t, err)

	// A trailing newline from a mounted secret has cost this codebase a
	// ~25-hour outage before. Assert the TRIMMED values, not just non-empty.
	require.Equal(t, "http://mark8ly-marketplace-api-storefront.mark8ly.svc.cluster.local:8080", cfg.StorefrontBaseURL)
	require.Equal(t, "sfkey", cfg.StorefrontKey)
	require.Equal(t, "mcpkey", cfg.MCPKey)
	require.Equal(t, 400*time.Millisecond, cfg.UpstreamTimeout)
}

func TestLoad_MissingRequiredFailsClosed(t *testing.T) {
	for _, missing := range []string{"STOREFRONT_BASE_URL", "STOREFRONT_KEY", "MCP_AUTH_KEY"} {
		t.Run(missing, func(t *testing.T) {
			t.Setenv("STOREFRONT_BASE_URL", "http://x:8080")
			t.Setenv("STOREFRONT_KEY", "k")
			t.Setenv("MCP_AUTH_KEY", "m")
			t.Setenv(missing, "")

			_, err := Load()
			require.Error(t, err, "%s is required; starting without it would serve an unauthenticated or unreachable connector", missing)
			require.Contains(t, err.Error(), missing)
		})
	}
}
```

- [ ] **Step 2: Run, confirm failure**

Run: `cd services/mcp && go test ./internal/config/ -v`
Expected: FAIL — package does not exist.

- [ ] **Step 3: Implement**

`Load()` reads the five variables, `strings.TrimSpace`es each **as it assigns**, defaults `PORT` to `8765` (the port every product MCP server in the estate already uses) and `UPSTREAM_TIMEOUT` to `400ms`, and returns an error naming any missing required variable. Never log a value.

- [ ] **Step 4: Run tests, confirm pass**

Run: `cd services/mcp && go test -race ./internal/config/ -v`

- [ ] **Step 5: Commit**

```bash
git commit -am "feat(mcp-catalog): config that trims secrets and refuses to start without them"
```

---

### Task 3: The upstream catalog client — with the parameters the handler actually reads

**Files:**
- Create: `services/mcp/internal/catalog/client.go`
- Test: `services/mcp/internal/catalog/client_test.go`

**Interfaces:**
- Consumes: `go-shared/mcp/upstream`.
- Produces:
  - `catalog.NewClient(baseURL, storefrontKey string, timeout time.Duration) (*Client, error)`
  - `(*Client) ListProducts(ctx, slug string, page, pageSize int) ([]storefrontProduct, error)`
  - `(*Client) GetProduct(ctx, slug, handle string) (storefrontProduct, error)`
  - `(*Client) ListCategories(ctx, slug string) ([]storefrontCategory, error)`
  - `(*Client) ListByCategory(ctx, slug, categorySlug string, page, pageSize int) ([]storefrontProduct, error)`
  - `(*Client) GetBranding(ctx, slug string) (storefrontBranding, error)`

The `storefront*` types are unexported mirrors of marketplace-api's wire DTOs — only the fields we project. They are NOT the agent-facing types (Task 4 owns those). Define exactly these, because Task 4's projection tests use these field names:

```go
type storefrontProduct struct {
	ID          string                  `json:"id"`
	Handle      string                  `json:"handle"`
	Title       string                  `json:"title"`
	Description *string                 `json:"description,omitempty"`
	Categories  []storefrontCategoryRef `json:"categories"`
	Media       []storefrontMedia       `json:"media"`
	PriceRange  storefrontPriceRange    `json:"price_range"`
	// Mirrored so the projection can be tested for dropping them. Never
	// projected — see the Global Constraints.
	TaxCode         *string `json:"tax_code,omitempty"`
	TaxRateOverride *string `json:"tax_rate_override,omitempty"`
	TaxCategory     *string `json:"tax_category,omitempty"`
}

type storefrontCategoryRef struct {
	Name string `json:"name"`
	Slug string `json:"slug"`
}

type storefrontCategory struct {
	Name     string `json:"name"`
	Slug     string `json:"slug"`
	Position int    `json:"position"`
	Featured bool   `json:"featured"`
}

type storefrontMedia struct {
	URL       string `json:"url"`
	MediaType string `json:"media_type"`
	Position  int    `json:"position"`
}

// Prices are strings on the wire: marketplace-api uses decimal.Decimal, which
// marshals as a JSON string. Keep them strings the whole way through.
type storefrontPriceRange struct {
	Min          string `json:"min"`
	Max          string `json:"max"`
	CurrencyCode string `json:"currency_code"`
}

type storefrontBranding struct {
	LogoURL          *string                `json:"logo_url,omitempty"`
	Tagline          *string                `json:"tagline,omitempty"`
	ColorAccent      string                 `json:"color_accent"`
	AnnouncementText *string                `json:"announcement_text,omitempty"`
	Promotions       []storefrontPromotion  `json:"active_promotions"`
}

type storefrontPromotion struct {
	Label      string  `json:"label"`
	CouponCode *string `json:"coupon_code,omitempty"`
}
```

**Confirm `active_promotions` against `branding.go`'s `PublicBrandingResponse` before relying on it** — the promotion field's JSON name was not captured during the audit, and a wrong tag decodes silently to an empty slice.

- [ ] **Step 1: Write the failing test**

```go
package catalog

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/tesserix/go-shared/mcp/upstream"
)

// The OpenAPI document this replaces claimed limit/offset. The handler reads
// page/page_size. Sending the wrong names does not error — it silently returns
// page 1 forever, which is why this test asserts the WIRE, not the result.
func TestListProducts_SendsPageParamsNotLimitOffset(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, "/api/v1/storefront/stores/bondi/products", r.URL.Path)
		assert.Equal(t, "2", r.URL.Query().Get("page"))
		assert.Equal(t, "50", r.URL.Query().Get("page_size"))
		assert.Empty(t, r.URL.Query().Get("limit"), "limit is not a parameter this API reads")
		assert.Empty(t, r.URL.Query().Get("offset"), "offset is not a parameter this API reads")
		assert.Equal(t, "sfkey", r.Header.Get("X-Storefront-Key"))
		_, _ = w.Write([]byte(`{"data":[{"handle":"mug","title":"Mug","price_range":{"min":"9.5","max":"9.5","currency_code":"AUD"}}],"meta":{"page":2,"page_size":50}}`))
	}))
	defer srv.Close()

	c, err := NewClient(srv.URL, "sfkey", time.Second)
	require.NoError(t, err)

	got, err := c.ListProducts(context.Background(), "bondi", 2, 50)
	require.NoError(t, err)
	require.Len(t, got, 1)
	assert.Equal(t, "mug", got[0].Handle)
}

// The list endpoints wrap results in {"data": …}. Decoding into a bare slice
// yields zero products and no error — a wrong answer that looks like an empty
// catalogue.
func TestListCategories_UnwrapsTheDataEnvelope(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(`{"data":[{"name":"Mugs","slug":"mugs","position":1,"featured":true}]}`))
	}))
	defer srv.Close()

	c, err := NewClient(srv.URL, "sfkey", time.Second)
	require.NoError(t, err)

	got, err := c.ListCategories(context.Background(), "bondi")
	require.NoError(t, err)
	require.Len(t, got, 1)
	assert.Equal(t, "mugs", got[0].Slug)
}

func TestGetProduct_UnknownHandleIsNotFound(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusNotFound)
	}))
	defer srv.Close()

	c, err := NewClient(srv.URL, "sfkey", time.Second)
	require.NoError(t, err)

	_, err = c.GetProduct(context.Background(), "bondi", "nope")
	require.ErrorIs(t, err, upstream.ErrNotFound)
}

// page_size above the handler's cap must be rejected here rather than sent.
// An agent told it received 500 products when it received 100 will summarise
// a catalogue it never saw.
func TestListProducts_RejectsPageSizeAboveCap(t *testing.T) {
	c, err := NewClient("http://example.invalid", "sfkey", time.Second)
	require.NoError(t, err)

	_, err = c.ListProducts(context.Background(), "bondi", 1, 500)
	require.Error(t, err)
	require.Contains(t, err.Error(), "100")
}
```

- [ ] **Step 2: Run, confirm failure**

Run: `cd services/mcp && go test ./internal/catalog/ -v`
Expected: FAIL — package does not exist.

- [ ] **Step 3: Implement**

Build one `upstream.Client` with `upstream.WithHeader("X-Storefront-Key", key)` and `upstream.WithTimeout(timeout)`. Each method builds its path under `/api/v1/storefront/stores/{slug}/…`, sets `page`/`page_size` where the handler accepts them, decodes into an envelope struct whose single field is `Data []storefrontProduct` tagged `json:"data"`, and returns that field.

`maxPageSize = 100`, matching `listPublishedQuery`'s `binding:"max=100"`. Reject above it with an error naming the cap; do not silently clamp.

Path segments must be escaped (`url.PathEscape`) — a store slug arrives from an agent.

- [ ] **Step 4: build, vet, `go test -race ./...`**

- [ ] **Step 5: Commit**

```bash
git commit -am "feat(mcp-catalog): call the storefront API with the parameters it actually reads"
```

---

### Task 4: The projection — closed result types

**Files:**
- Create: `services/mcp/internal/catalog/types.go`, `services/mcp/internal/catalog/project.go`
- Test: `services/mcp/internal/catalog/project_test.go`

**Interfaces:**
- Produces the agent-facing types and their constructors:
  - `type Product struct` — `Found bool`, `Handle`, `Title`, `Description`, `PriceMin`, `PriceMax`, `Currency`, `Categories []string`, `ImageURLs []string`
  - `type Category struct` — `Name`, `Slug`, `Featured bool`
  - `type Branding struct` — `Found bool`, `LogoURL`, `Tagline`, `AccentColor`, `Announcement`, `Promotions []Promotion`
  - `type Promotion struct` — `Label`, `CouponCode`

`Found` is on the SINGLE-item types (`Product`, `Branding`) from the start, not added later: a zero-valued result with no `Found` field is indistinguishable from a real product with empty strings. List results need no such flag — an empty list from a real store is a true answer; it is only a MISSING store that must not read as empty, and that is `upstream.ErrNotFound`, which propagates.
  - `func projectProduct(storefrontProduct) Product`, `projectCategory`, `projectBranding`

- [ ] **Step 1: Write the failing test**

```go
package catalog

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// The three tax fields are cart mechanics the storefront echoes back on
// checkout. They are not facts about a product and must never reach a model.
// They are absent from Product by construction — this test pins that the
// projection cannot reintroduce them.
func TestProjectProduct_DropsTaxAndInternalFields(t *testing.T) {
	in := storefrontProduct{
		ID:     "8f14e45f-ce7e-4a1b-9d3f-000000000001",
		Handle: "mug",
		Title:  "Mug",
	}
	in.TaxCode = strptr("GST")
	in.TaxCategory = strptr("standard")

	got := projectProduct(in)

	raw, err := json.Marshal(got)
	require.NoError(t, err)

	for _, banned := range []string{"tax_code", "tax_category", "tax_rate_override", "8f14e45f"} {
		assert.NotContains(t, string(raw), banned,
			"%s must not reach an agent", banned)
	}
	assert.Equal(t, "mug", got.Handle)
}

func TestProjectProduct_FlattensPriceRangeAndCategories(t *testing.T) {
	in := storefrontProduct{Handle: "mug", Title: "Mug"}
	in.PriceRange.Min = "9.50"
	in.PriceRange.Max = "12.00"
	in.PriceRange.CurrencyCode = "AUD"
	in.Categories = []storefrontCategoryRef{{Name: "Mugs", Slug: "mugs"}}
	in.Media = []storefrontMedia{{URL: "https://cdn/x.jpg", MediaType: "image"}}

	got := projectProduct(in)

	assert.Equal(t, "9.50", got.PriceMin)
	assert.Equal(t, "12.00", got.PriceMax)
	assert.Equal(t, "AUD", got.Currency)
	assert.Equal(t, []string{"mugs"}, got.Categories)
	assert.Equal(t, []string{"https://cdn/x.jpg"}, got.ImageURLs)
}

func strptr(s string) *string { return &s }
```

- [ ] **Step 2: Run, confirm failure**

Run: `cd services/mcp && go test ./internal/catalog/ -run TestProject -v`

- [ ] **Step 3: Implement**

Prices stay **strings**, exactly as marketplace-api sends them (`decimal.Decimal` marshals as a JSON string). Do not parse to `float64` — a float is the wrong type for money and would put rounding error in front of a customer.

`Categories` projects to slugs only; `ImageURLs` takes `media_type == "image"` entries in `position` order.

- [ ] **Step 4: build, vet, `go test -race ./...`**

- [ ] **Step 5: Commit**

```bash
git commit -am "feat(mcp-catalog): project storefront DTOs into closed agent-facing results"
```

---

### Task 5: The five tools

**Files:**
- Create: `services/mcp/internal/catalog/tools.go`
- Test: `services/mcp/internal/catalog/tools_test.go`

**Interfaces:**
- Produces: `catalog.RegisterTools(r *gsmcp.Registry, c *Client) error`, registering exactly:
  `list_store_products`, `get_store_product`, `list_store_categories`, `list_products_by_category`, `get_store_branding`.

- [ ] **Step 1: Write the failing test**

```go
package catalog

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	gsmcp "github.com/tesserix/go-shared/mcp"
)

func TestRegisterTools_RegistersExactlyTheFive(t *testing.T) {
	r := gsmcp.NewRegistry()
	require.NoError(t, RegisterTools(r, &Client{}))

	assert.Equal(t, []string{
		"get_store_branding",
		"get_store_product",
		"list_products_by_category",
		"list_store_categories",
		"list_store_products",
	}, r.Names())
}

// Store scoping is by slug. A tool that accepted an internal id would invite
// cross-store probing, and the id is not a public identifier.
func TestRegisterTools_NoToolAcceptsAnInternalIdentifier(t *testing.T) {
	r := gsmcp.NewRegistry()
	require.NoError(t, RegisterTools(r, &Client{}))

	for _, tool := range r.Tools() {
		props, _ := tool.InputSchema["properties"].(map[string]any)
		for name := range props {
			assert.NotContains(t, name, "store_id", "tool %s", tool.Name)
			assert.NotContains(t, name, "tenant", "tool %s", tool.Name)
			assert.NotContains(t, name, "_uuid", "tool %s", tool.Name)
		}
		assert.Equal(t, false, tool.InputSchema["additionalProperties"], "tool %s input must be closed", tool.Name)
		assert.Equal(t, false, tool.OutputSchema["additionalProperties"], "tool %s output must be closed", tool.Name)
	}
}
```

- [ ] **Step 2: Run, confirm failure**

- [ ] **Step 3: Implement**

Each tool is a `gsmcp.Register` call whose handler calls the client and projects. Input types carry `desc` tags — the model reads them, so describe the parameter's meaning, not its Go type.

**A not-found is a typed result, never an empty list.** `get_store_product` for an unknown handle returns `Product{}` with `Found: false`; an empty `[]Product` would read as "this store sells nothing", which is a plausible-looking wrong answer. `Found` is already on those types from Task 4. `upstream.ErrNotFound` maps to `Found: false`; every other error propagates so `observe.OutcomeFor` classifies it.

- [ ] **Step 4: build, vet, `go test -race ./...`**

- [ ] **Step 5: Commit**

```bash
git commit -am "feat(mcp-catalog): the five catalog tools"
```

---

### Task 6: Wire main, and pin declared-vs-served

**Files:**
- Modify: `services/mcp/cmd/mcp-catalog/main.go`
- Create: `services/mcp/cmd/mcp-catalog/declared_test.go`, `services/mcp/Dockerfile`
- Modify: `.github/workflows/ci.yml`

**Interfaces:**
- Consumes everything above.

- [ ] **Step 1: Write the failing test**

```go
package main

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/mark8ly/mcp/internal/catalog"
	gsmcp "github.com/tesserix/go-shared/mcp"
)

// declaredTools is the registry record's tool list, copied by hand from
// devai/architecture/registry-seeds/mcp-servers/mark8ly-mcp-catalog.yaml.
// #412 was not a typo — it was that nothing compared what was DECLARED
// against what was SERVED. This is that comparison.
var declaredTools = []string{
	"get_store_branding",
	"get_store_product",
	"list_products_by_category",
	"list_store_categories",
	"list_store_products",
}

func TestServedToolsMatchTheRegistryRecord(t *testing.T) {
	r := gsmcp.NewRegistry()
	require.NoError(t, catalog.RegisterTools(r, &catalog.Client{}))

	assert.Equal(t, declaredTools, r.Names(),
		"the registry seed and the server disagree — update the seed in devai "+
			"and this list together, or an agent is offered a tool that does not "+
			"exist (or denied one that does)")
}
```

- [ ] **Step 2: Run, confirm failure**

- [ ] **Step 3: Implement main and the Dockerfile**

`main.go`: load config → build client → build registry → `RegisterTools` → `observe.NewToolMetrics` → `server.New` → serve on `cfg.Port` with `/healthz` and `/readyz`, plus `promhttp` on the metrics path the other services use. Graceful shutdown on SIGTERM. Log startup WITHOUT any secret.

`Dockerfile`: multi-stage, matching `services/otto/Dockerfile`'s shape — read it, do not invent one.

- [ ] **Step 4: Add the service to CI**

In `.github/workflows/ci.yml`, add to the Go matrix:
```yaml
          - service: mcp
            directory: services/mcp
            coverage: 60
```
The existing floors range 4–31; that is not a precedent. A greenfield service with a five-tool surface starts at 60 and goes up, never down.

- [ ] **Step 5: Full verification**

```bash
cd services/mcp && go build ./... && go vet ./... && go test -race -count=1 ./...
```

- [ ] **Step 6: Commit**

```bash
git commit -am "feat(mcp-catalog): wire the binary and pin declared tools against served"
```

---

## Out of scope

- **Deploying it.** The chart, ExternalSecret, ServiceAccount, KEDA `ScaledObject` and the `devai` registry seed are a follow-up. This plan ends with a tested binary and an image build.
- **Retiring the shared `mark8ly-mcp` pod.** That is step 4 of the spec, after the support tools migrate.
- **Touching `marketplace-api`.** No handler changes. If a projection needs a field the storefront API does not expose, stop and raise it rather than adding an endpoint.
- **Deleting the old OpenAPI document.** It still serves `mcp-gateway` until that pod goes away. Its inaccuracies are recorded above; fixing them is pointless work on a file with a scheduled death.

## Open questions to settle during Task 1

1. **Which schema derivation wins** — recorded as a decision in `server.go`, with the closed-schema assertion either passing under approach A or explicitly re-pointed at approach B.
2. **Whether we serve only `2026-07-28` or accept the SDK's negotiation down to older versions.** The estate's registry records pin `2026-07-28`; the deployed shared gateway rejected every published version when probed on 2026-09-04, so there is no working precedent to copy. Recommendation: accept the SDK's default negotiation, and record the versions it advertises in the report — narrowing later is easy, widening after a consumer exists is not.
