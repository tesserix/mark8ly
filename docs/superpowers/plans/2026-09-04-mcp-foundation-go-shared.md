# `go-shared/mcp` Foundation Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Ship `go-shared/mcp` — the domain-free, SDK-free foundation every mark8ly and tesserix-home MCP server will be built on — as a go-shared minor release.

**Architecture:** Five small packages under `mcp/`. `schema` derives closed JSON Schema from Go types. The `mcp` root registers tools through a generic function whose signature makes an untyped tool unrepresentable. `upstream` is an HTTP client with exactly one exported method, `Get`. `auth` verifies `X-MCP-Key`, failing closed. `observe` emits per-tool metrics that distinguish a dead server from a failing one. Nothing here imports an MCP SDK, a protocol type, or any product package.

**Tech Stack:** Go 1.26, `reflect`, `crypto/subtle`, `net/http`, `github.com/prometheus/client_golang`, `github.com/stretchr/testify` (all already in `go-shared/go.mod` — this plan adds **no** new dependency).

**Spec:** `docs/superpowers/specs/2026-09-04-mark8ly-mcp-connectors-design.md` (in the `mark8ly` repo; decisions D2, D5, D6, D9 are the ones this plan implements)

## Global Constraints

- **Repository: `go-shared`, not `mark8ly`.** Every path below is relative to the `go-shared` checkout (`../go-shared` from mark8ly). Branch from `origin/main`.
- **No new module dependency.** `go.mod` must be unchanged by this plan. If a task seems to need a dependency, stop and raise it — D9 exists to keep this surface small.
- **No MCP SDK import, anywhere.** Not in code, not in tests. (D9)
- **No product package import.** Nothing from `mark8ly`, `tesserix-home`, or any service. (D2)
- **Read-only.** No package here may issue an HTTP method other than GET. (D6)
- **Closed schemas.** Every derived object schema sets `additionalProperties: false`. (D5)
- **Go 1.26**, matching `go-shared/go.mod`.
- `go build ./... && go vet ./... && go test -race ./...` must pass at the end of every task.
- Commit messages: single-line subject, conventional-commit prefix, no body.

---

## File Structure

| File | Responsibility |
| --- | --- |
| `mcp/doc.go` | Package doc: what the foundation is, and why the SDK is absent (D9) |
| `mcp/schema/schema.go` | Go type → closed JSON Schema |
| `mcp/schema/schema_test.go` | Derivation, closedness, unsupported-type rejection |
| `mcp/tool.go` | `Tool`, `Registry`, generic `Register` — D5's structural enforcement |
| `mcp/tool_test.go` | Registration, duplicate rejection, untyped-output rejection |
| `mcp/upstream/client.go` | GET-only HTTP client, deadline, three distinct failure modes |
| `mcp/upstream/client_test.go` | Happy path, 404, 5xx, timeout, and the method-set assertion (D6) |
| `mcp/auth/key.go` | `X-MCP-Key` verification, fail-closed, constant-time |
| `mcp/auth/key_test.go` | Accept, reject, empty-expected-key, timing-safe comparison |
| `mcp/observe/metrics.go` | Per-tool latency + outcome counters |
| `mcp/observe/metrics_test.go` | Registration, outcome labelling |
| `mcp/boundaries_test.go` | D2 and D9 asserted against the real import graph |

---

### Task 1: `mcp/schema` — closed JSON Schema from Go types

**Files:**
- Create: `mcp/schema/schema.go`
- Test: `mcp/schema/schema_test.go`

**Interfaces:**
- Consumes: nothing.
- Produces: `schema.For(v any) (map[string]any, error)` — returns a JSON Schema
  object for `v`'s type. Object schemas always carry
  `"additionalProperties": false`. Returns an error for channels, funcs, maps
  with non-string keys, and `interface{}`.

- [ ] **Step 1: Write the failing test**

```go
package schema

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type product struct {
	Handle string  `json:"handle" desc:"URL-safe product slug"`
	Title  string  `json:"title"`
	Price  float64 `json:"price"`
	Tags   []string `json:"tags"`
	Note   *string `json:"note,omitempty"`
	hidden string  //nolint:unused // unexported fields must not appear
}

func TestFor_StructIsClosedAndDescribed(t *testing.T) {
	s, err := For(product{})
	require.NoError(t, err)

	assert.Equal(t, "object", s["type"])
	// A closed schema is the whole point of D5 — an agent must not receive
	// fields nobody declared.
	assert.Equal(t, false, s["additionalProperties"])

	props := s["properties"].(map[string]any)
	assert.Equal(t, "string", props["handle"].(map[string]any)["type"])
	assert.Equal(t, "URL-safe product slug", props["handle"].(map[string]any)["description"])
	assert.Equal(t, "number", props["price"].(map[string]any)["type"])
	assert.Equal(t, "array", props["tags"].(map[string]any)["type"])
	assert.Equal(t, "string", props["tags"].(map[string]any)["items"].(map[string]any)["type"])

	assert.NotContains(t, props, "hidden", "unexported fields must never be exposed")

	// omitempty and pointers mean optional; everything else is required.
	assert.ElementsMatch(t, []any{"handle", "title", "price", "tags"}, s["required"])
}

func TestFor_RejectsUntypedInterface(t *testing.T) {
	var v any
	_, err := For(v)
	require.Error(t, err, "an interface carries no schema, so it cannot be closed")
}

func TestFor_RejectsUnsupportedKind(t *testing.T) {
	_, err := For(make(chan int))
	require.Error(t, err)
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `cd ../go-shared && go test ./mcp/schema/ -run TestFor -v`
Expected: FAIL — `no required module provides package .../mcp/schema` (the package does not exist yet).

- [ ] **Step 3: Write minimal implementation**

```go
// Package schema derives closed JSON Schema documents from Go types.
//
// "Closed" means every object schema carries additionalProperties:false. That
// is not a stylistic preference: a tool result an agent may cite has to be a
// declared shape, and a schema that permits unknown fields permits an
// undeclared one to reach a model. See the design's D5.
package schema

import (
	"fmt"
	"reflect"
	"strings"
)

// For derives a JSON Schema object for v's type.
func For(v any) (map[string]any, error) {
	t := reflect.TypeOf(v)
	if t == nil {
		return nil, fmt.Errorf("schema: cannot derive a schema from an untyped nil or interface")
	}
	return forType(t)
}

func forType(t reflect.Type) (map[string]any, error) {
	switch t.Kind() {
	case reflect.Pointer:
		return forType(t.Elem())
	case reflect.String:
		return map[string]any{"type": "string"}, nil
	case reflect.Bool:
		return map[string]any{"type": "boolean"}, nil
	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64,
		reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64:
		return map[string]any{"type": "integer"}, nil
	case reflect.Float32, reflect.Float64:
		return map[string]any{"type": "number"}, nil
	case reflect.Slice, reflect.Array:
		items, err := forType(t.Elem())
		if err != nil {
			return nil, err
		}
		return map[string]any{"type": "array", "items": items}, nil
	case reflect.Struct:
		return forStruct(t)
	default:
		return nil, fmt.Errorf("schema: unsupported kind %s for type %s", t.Kind(), t)
	}
}

func forStruct(t reflect.Type) (map[string]any, error) {
	props := map[string]any{}
	var required []any

	for i := range t.NumField() {
		f := t.Field(i)
		if !f.IsExported() {
			continue
		}
		name, opts, ok := jsonName(f)
		if !ok {
			continue
		}
		sub, err := forType(f.Type)
		if err != nil {
			return nil, fmt.Errorf("field %s: %w", f.Name, err)
		}
		if d := f.Tag.Get("desc"); d != "" {
			sub["description"] = d
		}
		props[name] = sub

		if !opts.omitempty && f.Type.Kind() != reflect.Pointer {
			required = append(required, name)
		}
	}

	s := map[string]any{
		"type":                 "object",
		"properties":           props,
		"additionalProperties": false,
	}
	if len(required) > 0 {
		s["required"] = required
	}
	return s, nil
}

type fieldOpts struct{ omitempty bool }

func jsonName(f reflect.StructField) (string, fieldOpts, bool) {
	tag := f.Tag.Get("json")
	if tag == "-" {
		return "", fieldOpts{}, false
	}
	parts := strings.Split(tag, ",")
	name := parts[0]
	if name == "" {
		name = f.Name
	}
	var o fieldOpts
	for _, p := range parts[1:] {
		if p == "omitempty" {
			o.omitempty = true
		}
	}
	return name, o, true
}
```

- [ ] **Step 4: Run test to verify it passes**

Run: `cd ../go-shared && go test -race ./mcp/schema/ -v`
Expected: PASS, 3 tests.

- [ ] **Step 5: Commit**

```bash
cd ../go-shared
git add mcp/schema/
git commit -m "feat(mcp): derive closed JSON Schema from Go types"
```

---

### Task 2: `mcp` root — a registry that cannot hold an untyped tool

**Files:**
- Create: `mcp/tool.go`, `mcp/doc.go`
- Test: `mcp/tool_test.go`

**Interfaces:**
- Consumes: `schema.For` from Task 1.
- Produces:
  - `type Tool struct { Name, Description string; InputSchema, OutputSchema map[string]any; Invoke func(context.Context, json.RawMessage) (any, error) }`
  - `func NewRegistry() *Registry`
  - `func Register[In, Out any](r *Registry, name, description string, h func(context.Context, In) (Out, error)) error`
  - `func (r *Registry) Tools() []Tool` — sorted by name, safe for concurrent use.
  - `func (r *Registry) Names() []string` — sorted; used by the declared-vs-served CI test.

- [ ] **Step 1: Write the failing test**

```go
package mcp

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type listIn struct {
	StoreSlug string `json:"store_slug" desc:"Public store slug"`
}

type listOut struct {
	Found bool     `json:"found"`
	Items []string `json:"items"`
}

func listHandler(_ context.Context, in listIn) (listOut, error) {
	return listOut{Found: true, Items: []string{in.StoreSlug}}, nil
}

func TestRegister_DerivesBothSchemas(t *testing.T) {
	r := NewRegistry()
	require.NoError(t, Register(r, "list_store_products", "List products.", listHandler))

	tools := r.Tools()
	require.Len(t, tools, 1)

	// D5: BOTH schemas exist. A tool with an input schema and no output schema
	// is exactly the thing OpenAPI ingestion produced, and the reason it was
	// rejected.
	require.NotNil(t, tools[0].InputSchema)
	require.NotNil(t, tools[0].OutputSchema)
	assert.Equal(t, false, tools[0].OutputSchema["additionalProperties"])
}

func TestRegister_RejectsUntypedOutput(t *testing.T) {
	r := NewRegistry()
	err := Register(r, "bad", "Untyped.", func(_ context.Context, _ listIn) (any, error) {
		return nil, nil
	})
	require.Error(t, err, "an `any` result cannot be closed, so it cannot be cited")
}

func TestRegister_RejectsDuplicateName(t *testing.T) {
	r := NewRegistry()
	require.NoError(t, Register(r, "dup", "First.", listHandler))
	require.Error(t, Register(r, "dup", "Second.", listHandler))
}

func TestInvoke_DecodesAndReturnsTypedResult(t *testing.T) {
	r := NewRegistry()
	require.NoError(t, Register(r, "list_store_products", "List products.", listHandler))

	out, err := r.Tools()[0].Invoke(context.Background(), json.RawMessage(`{"store_slug":"bondi"}`))
	require.NoError(t, err)
	assert.Equal(t, listOut{Found: true, Items: []string{"bondi"}}, out)
}

func TestInvoke_RejectsUnknownInputField(t *testing.T) {
	r := NewRegistry()
	require.NoError(t, Register(r, "list_store_products", "List products.", listHandler))

	_, err := r.Tools()[0].Invoke(context.Background(), json.RawMessage(`{"store_id":"7"}`))
	require.Error(t, err, "input is a closed schema; an undeclared field is a caller error")
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `cd ../go-shared && go test ./mcp/ -v`
Expected: FAIL — package `mcp` does not exist.

- [ ] **Step 3: Write minimal implementation**

`mcp/doc.go`:

```go
// Package mcp is the foundation every Tesserix MCP connector is built on:
// tool registration with closed input and output schemas, a GET-only upstream
// client, key verification, and per-tool metrics.
//
// # What is deliberately NOT here
//
// There is no MCP SDK import, and no protocol type, anywhere in this package
// tree. The binding to a specific SDK and protocol version lives in the
// consuming service.
//
// Two reasons. Every Go service in the estate imports go-shared, so an SDK
// dependency here enters ~30 module graphs whether those services serve MCP or
// not. And the protocol is where the movement is — the estate's registry
// records pin protocolVersion 2026-07-28 — so a protocol revision would
// otherwise force a go-shared release affecting every service to change
// something only MCP servers care about.
//
// boundaries_test.go enforces both halves of that. See the design's D9.
package mcp
```

`mcp/tool.go`:

```go
package mcp

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"reflect"
	"sort"
	"sync"

	"github.com/tesserix/go-shared/mcp/schema"
)

// Tool is a registered, fully-described tool.
type Tool struct {
	Name         string
	Description  string
	InputSchema  map[string]any
	OutputSchema map[string]any
	Invoke       func(context.Context, json.RawMessage) (any, error)
}

// Registry holds the tools a server serves. Safe for concurrent use.
type Registry struct {
	mu    sync.RWMutex
	tools map[string]Tool
}

// NewRegistry returns an empty Registry.
func NewRegistry() *Registry {
	return &Registry{tools: map[string]Tool{}}
}

// Register adds a tool, deriving both schemas from the handler's own signature.
//
// The generic signature is the enforcement of D5, not a convenience: there is
// no way to call this without an input type and an output type, so a tool
// cannot exist without both. The only remaining hole is naming `any` as the
// output, which is rejected below.
func Register[In, Out any](r *Registry, name, description string, h func(context.Context, In) (Out, error)) error {
	if name == "" {
		return fmt.Errorf("mcp: tool name must not be empty")
	}
	if description == "" {
		return fmt.Errorf("mcp: tool %q must carry a description — it is what the model reads", name)
	}

	var out Out
	if reflect.TypeOf(&out).Elem().Kind() == reflect.Interface {
		return fmt.Errorf("mcp: tool %q declares an interface result; an untyped result cannot be closed or cited", name)
	}

	var in In
	inSchema, err := schema.For(in)
	if err != nil {
		return fmt.Errorf("mcp: tool %q input: %w", name, err)
	}
	outSchema, err := schema.For(out)
	if err != nil {
		return fmt.Errorf("mcp: tool %q output: %w", name, err)
	}

	r.mu.Lock()
	defer r.mu.Unlock()
	if _, exists := r.tools[name]; exists {
		return fmt.Errorf("mcp: tool %q is already registered", name)
	}

	r.tools[name] = Tool{
		Name:         name,
		Description:  description,
		InputSchema:  inSchema,
		OutputSchema: outSchema,
		Invoke: func(ctx context.Context, raw json.RawMessage) (any, error) {
			var args In
			dec := json.NewDecoder(bytes.NewReader(raw))
			// The input schema is closed, so the decoder must be too —
			// otherwise a caller can pass a field the schema forbids and be
			// silently ignored rather than corrected.
			dec.DisallowUnknownFields()
			if len(raw) > 0 {
				if err := dec.Decode(&args); err != nil {
					return nil, fmt.Errorf("mcp: tool %q arguments: %w", name, err)
				}
			}
			return h(ctx, args)
		},
	}
	return nil
}

// Tools returns every registered tool, sorted by name.
func (r *Registry) Tools() []Tool {
	r.mu.RLock()
	defer r.mu.RUnlock()

	out := make([]Tool, 0, len(r.tools))
	for _, t := range r.tools {
		out = append(out, t)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Name < out[j].Name })
	return out
}

// Names returns registered tool names, sorted. The declared-vs-served check
// compares this against the registry record.
func (r *Registry) Names() []string {
	tools := r.Tools()
	names := make([]string, len(tools))
	for i, t := range tools {
		names[i] = t.Name
	}
	return names
}
```

- [ ] **Step 4: Run test to verify it passes**

Run: `cd ../go-shared && go test -race ./mcp/ -v`
Expected: PASS, 5 tests.

- [ ] **Step 5: Commit**

```bash
cd ../go-shared
git add mcp/doc.go mcp/tool.go mcp/tool_test.go
git commit -m "feat(mcp): a tool cannot be registered without both schemas"
```

---

### Task 3: `mcp/upstream` — a client with no write verb

**Files:**
- Create: `mcp/upstream/client.go`
- Test: `mcp/upstream/client_test.go`

**Interfaces:**
- Consumes: nothing from earlier tasks.
- Produces:
  - `func New(baseURL string, opts ...Option) (*Client, error)`
  - `func WithHeader(k, v string) Option`, `func WithTimeout(d time.Duration) Option`, `func WithHTTPClient(c *http.Client) Option`
  - `func (c *Client) Get(ctx context.Context, path string, params url.Values, out any) error`
  - Sentinels: `ErrNotFound`, `ErrUnavailable`, `ErrDeadlineExceeded`

- [ ] **Step 1: Write the failing test**

```go
package upstream

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"net/url"
	"reflect"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type result struct {
	Handle string `json:"handle"`
}

func TestGet_HappyPathSendsHeadersAndParams(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, http.MethodGet, r.Method)
		assert.Equal(t, "secret", r.Header.Get("X-Storefront-Key"))
		assert.Equal(t, "20", r.URL.Query().Get("limit"))
		assert.Equal(t, "/api/v1/storefront/stores/bondi/products", r.URL.Path)
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"handle":"mug"}`))
	}))
	defer srv.Close()

	c, err := New(srv.URL, WithHeader("X-Storefront-Key", "secret"))
	require.NoError(t, err)

	var got result
	require.NoError(t, c.Get(context.Background(),
		"/api/v1/storefront/stores/bondi/products", url.Values{"limit": {"20"}}, &got))
	assert.Equal(t, "mug", got.Handle)
}

func TestGet_404IsNotFoundNotUnavailable(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusNotFound)
	}))
	defer srv.Close()

	c, err := New(srv.URL)
	require.NoError(t, err)

	var got result
	err = c.Get(context.Background(), "/missing", nil, &got)
	// These must stay distinct: "this store does not exist" and "we could not
	// ask" lead to different agent behaviour.
	require.ErrorIs(t, err, ErrNotFound)
	assert.NotErrorIs(t, err, ErrUnavailable)
}

func TestGet_5xxIsUnavailable(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusBadGateway)
	}))
	defer srv.Close()

	c, err := New(srv.URL)
	require.NoError(t, err)

	var got result
	require.ErrorIs(t, c.Get(context.Background(), "/x", nil, &got), ErrUnavailable)
}

func TestGet_TimeoutIsDeadlineExceeded(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		time.Sleep(150 * time.Millisecond)
	}))
	defer srv.Close()

	c, err := New(srv.URL, WithTimeout(20*time.Millisecond))
	require.NoError(t, err)

	var got result
	require.ErrorIs(t, c.Get(context.Background(), "/slow", nil, &got), ErrDeadlineExceeded)
}

// D6, asserted structurally rather than by review. If someone adds Post, this
// fails — which is the entire point: a write must not be something an author
// merely remembers not to do.
func TestClient_ExposesOnlyGet(t *testing.T) {
	var methods []string
	ct := reflect.TypeOf(&Client{})
	for i := range ct.NumMethod() {
		methods = append(methods, ct.Method(i).Name)
	}
	assert.Equal(t, []string{"Get"}, methods,
		"the upstream client must be incapable of expressing a write")
}

var _ = errors.Is
```

- [ ] **Step 2: Run test to verify it fails**

Run: `cd ../go-shared && go test ./mcp/upstream/ -v`
Expected: FAIL — package does not exist.

- [ ] **Step 3: Write minimal implementation**

```go
// Package upstream is the only way a connector reaches a product API.
//
// It exposes exactly one verb. That is D6 made structural: a connector cannot
// perform a write because there is no method that issues one, not because an
// author remembered to avoid it.
package upstream

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"strings"
	"time"
)

// Failure modes a caller must be able to tell apart. A missing store and an
// unreachable API produce very different agent behaviour: one is an answer,
// the other must degrade to a document-only reply with disclosure.
var (
	ErrNotFound         = errors.New("upstream: not found")
	ErrUnavailable      = errors.New("upstream: unavailable")
	ErrDeadlineExceeded = errors.New("upstream: deadline exceeded")
)

const defaultTimeout = 400 * time.Millisecond

// Client performs GET requests against one product API.
type Client struct {
	base    *url.URL
	headers map[string]string
	http    *http.Client
}

// Option configures a Client.
type Option func(*Client)

// WithHeader adds a header sent on every request — the credential path.
func WithHeader(k, v string) Option {
	return func(c *Client) { c.headers[k] = v }
}

// WithTimeout overrides the per-request deadline. Defaults to 400ms, the
// per-tool budget the engine contracts for.
func WithTimeout(d time.Duration) Option {
	return func(c *Client) { c.http.Timeout = d }
}

// WithHTTPClient replaces the underlying client. The timeout is preserved.
func WithHTTPClient(h *http.Client) Option {
	return func(c *Client) {
		t := c.http.Timeout
		c.http = h
		if c.http.Timeout == 0 {
			c.http.Timeout = t
		}
	}
}

// New returns a Client rooted at baseURL.
func New(baseURL string, opts ...Option) (*Client, error) {
	u, err := url.Parse(baseURL)
	if err != nil {
		return nil, fmt.Errorf("upstream: base URL: %w", err)
	}
	if u.Scheme == "" || u.Host == "" {
		return nil, fmt.Errorf("upstream: base URL %q must be absolute", baseURL)
	}
	c := &Client{
		base:    u,
		headers: map[string]string{},
		http:    &http.Client{Timeout: defaultTimeout},
	}
	for _, o := range opts {
		o(c)
	}
	return c, nil
}

// Get fetches path and decodes the JSON body into out.
func (c *Client) Get(ctx context.Context, path string, params url.Values, out any) error {
	ref := &url.URL{Path: path}
	if params != nil {
		ref.RawQuery = params.Encode()
	}
	target := c.base.ResolveReference(ref)

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, target.String(), nil)
	if err != nil {
		return fmt.Errorf("%w: %s", ErrUnavailable, err)
	}
	req.Header.Set("Accept", "application/json")
	for k, v := range c.headers {
		req.Header.Set(k, v)
	}

	resp, err := c.http.Do(req)
	if err != nil {
		if errors.Is(err, context.DeadlineExceeded) || isTimeout(err) {
			return fmt.Errorf("%w: %s", ErrDeadlineExceeded, err)
		}
		return fmt.Errorf("%w: %s", ErrUnavailable, err)
	}
	defer func() { _ = resp.Body.Close() }()

	switch {
	case resp.StatusCode == http.StatusNotFound:
		return ErrNotFound
	case resp.StatusCode >= 400:
		// Everything else — including a 4xx that means our credential is
		// wrong — is "we could not ask". It must never be reported as an
		// empty catalogue.
		return fmt.Errorf("%w: status %d", ErrUnavailable, resp.StatusCode)
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return fmt.Errorf("%w: reading body: %s", ErrUnavailable, err)
	}
	if err := json.Unmarshal(body, out); err != nil {
		return fmt.Errorf("%w: decoding body: %s", ErrUnavailable, err)
	}
	return nil
}

func isTimeout(err error) bool {
	var ne net.Error
	if errors.As(err, &ne) {
		return ne.Timeout()
	}
	return strings.Contains(err.Error(), "Client.Timeout")
}
```


- [ ] **Step 4: Run test to verify it passes**

Run: `cd ../go-shared && go test -race ./mcp/upstream/ -v`
Expected: PASS, 5 tests.

- [ ] **Step 5: Commit**

```bash
cd ../go-shared
git add mcp/upstream/
git commit -m "feat(mcp): a GET-only upstream client that cannot express a write"
```

---

### Task 4: `mcp/auth` — fail-closed key verification

**Files:**
- Create: `mcp/auth/key.go`
- Test: `mcp/auth/key_test.go`

**Interfaces:**
- Consumes: nothing from earlier tasks.
- Produces: `func RequireKey(expected string, next http.Handler) http.Handler` —
  wraps a handler, requiring header `X-MCP-Key`. Header name exported as
  `const HeaderName = "X-MCP-Key"`.

- [ ] **Step 1: Write the failing test**

```go
package auth

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/assert"
)

func ok() http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	})
}

func do(t *testing.T, h http.Handler, key string) int {
	t.Helper()
	req := httptest.NewRequest(http.MethodPost, "/mcp", nil)
	if key != "" {
		req.Header.Set(HeaderName, key)
	}
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	return rec.Code
}

func TestRequireKey_AcceptsMatching(t *testing.T) {
	assert.Equal(t, http.StatusOK, do(t, RequireKey("s3cret", ok()), "s3cret"))
}

func TestRequireKey_RejectsWrongAndMissing(t *testing.T) {
	h := RequireKey("s3cret", ok())
	assert.Equal(t, http.StatusUnauthorized, do(t, h, "wrong"))
	assert.Equal(t, http.StatusUnauthorized, do(t, h, ""))
}

// The case that matters most: an unset secret must not become an open door.
// A missing ExternalSecret is a deployment mistake, and the safe reading of it
// is "nobody may call", never "everybody may".
func TestRequireKey_EmptyExpectedFailsClosed(t *testing.T) {
	h := RequireKey("", ok())
	assert.Equal(t, http.StatusUnauthorized, do(t, h, ""))
	assert.Equal(t, http.StatusUnauthorized, do(t, h, "anything"))
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `cd ../go-shared && go test ./mcp/auth/ -v`
Expected: FAIL — package does not exist.

- [ ] **Step 3: Write minimal implementation**

```go
// Package auth verifies the shared key a connector is called with.
package auth

import (
	"crypto/subtle"
	"net/http"
)

// HeaderName is the header every product MCP server in the estate is called
// with; the registry records name it in their credentialRef.
const HeaderName = "X-MCP-Key"

// RequireKey wraps next, rejecting any request whose HeaderName does not match
// expected.
//
// An empty expected key rejects everything. That is deliberate: the key comes
// from a mounted secret, and an absent secret means the deployment is
// misconfigured — serving an open endpoint would turn a configuration mistake
// into an exposure.
func RequireKey(expected string, next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if expected == "" {
			http.Error(w, "unauthorized", http.StatusUnauthorized)
			return
		}
		got := r.Header.Get(HeaderName)
		if subtle.ConstantTimeCompare([]byte(got), []byte(expected)) != 1 {
			http.Error(w, "unauthorized", http.StatusUnauthorized)
			return
		}
		next.ServeHTTP(w, r)
	})
}
```

- [ ] **Step 4: Run test to verify it passes**

Run: `cd ../go-shared && go test -race ./mcp/auth/ -v`
Expected: PASS, 3 tests.

- [ ] **Step 5: Commit**

```bash
cd ../go-shared
git add mcp/auth/
git commit -m "feat(mcp): verify the connector key, failing closed on an unset secret"
```

---

### Task 5: `mcp/observe` — metrics that distinguish dead from failing

**Files:**
- Create: `mcp/observe/metrics.go`
- Test: `mcp/observe/metrics_test.go`

**Interfaces:**
- Consumes: nothing from earlier tasks.
- Produces:
  - `type Outcome string` with `OutcomeOK`, `OutcomeNotFound`, `OutcomeUnavailable`, `OutcomeDeadline`, `OutcomeInvalidInput`
  - `func NewToolMetrics(reg prometheus.Registerer, service string) (*ToolMetrics, error)`
  - `func (m *ToolMetrics) Observe(tool string, outcome Outcome, d time.Duration)`

- [ ] **Step 1: Write the failing test**

```go
package observe

import (
	"testing"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/testutil"
	"github.com/stretchr/testify/require"
)

func TestObserve_CountsPerToolAndOutcome(t *testing.T) {
	reg := prometheus.NewRegistry()
	m, err := NewToolMetrics(reg, "mcp-catalog")
	require.NoError(t, err)

	m.Observe("list_store_products", OutcomeOK, 12*time.Millisecond)
	m.Observe("list_store_products", OutcomeUnavailable, 8*time.Millisecond)
	m.Observe("get_store_product", OutcomeNotFound, 3*time.Millisecond)

	require.Equal(t, 1.0, testutil.ToFloat64(
		m.calls.WithLabelValues("list_store_products", string(OutcomeOK))))
	require.Equal(t, 1.0, testutil.ToFloat64(
		m.calls.WithLabelValues("list_store_products", string(OutcomeUnavailable))))
	require.Equal(t, 1.0, testutil.ToFloat64(
		m.calls.WithLabelValues("get_store_product", string(OutcomeNotFound))))
}

// Registering twice against one registry is a startup bug, and it must be an
// error rather than a panic in a library ~30 services import.
func TestNewToolMetrics_DuplicateRegistrationIsAnError(t *testing.T) {
	reg := prometheus.NewRegistry()
	_, err := NewToolMetrics(reg, "mcp-catalog")
	require.NoError(t, err)
	_, err = NewToolMetrics(reg, "mcp-catalog")
	require.Error(t, err)
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `cd ../go-shared && go test ./mcp/observe/ -v`
Expected: FAIL — package does not exist.

- [ ] **Step 3: Write minimal implementation**

```go
// Package observe carries a connector's per-tool metrics.
//
// Outcome is a label rather than separate metrics because the distinction that
// matters is between a server that has STOPPED SERVING and one that KEEPS
// FAILING — and both are invisible if success and failure share a counter.
package observe

import (
	"time"

	"github.com/prometheus/client_golang/prometheus"
)

// Outcome is the controlled vocabulary for how a tool call ended.
type Outcome string

const (
	OutcomeOK           Outcome = "ok"
	OutcomeNotFound     Outcome = "not_found"
	OutcomeUnavailable  Outcome = "unavailable"
	OutcomeDeadline     Outcome = "deadline"
	OutcomeInvalidInput Outcome = "invalid_input"
)

// ToolMetrics records tool call counts and latency.
type ToolMetrics struct {
	calls    *prometheus.CounterVec
	duration *prometheus.HistogramVec
}

// NewToolMetrics registers the metrics on reg.
func NewToolMetrics(reg prometheus.Registerer, service string) (*ToolMetrics, error) {
	labels := prometheus.Labels{"service": service}

	calls := prometheus.NewCounterVec(prometheus.CounterOpts{
		Name:        "tesserix_mcp_tool_calls_total",
		Help:        "Tool calls by tool and outcome. Read outcome!=ok against outcome=ok — a tool that only fails and a tool nobody calls look identical in a single total.",
		ConstLabels: labels,
	}, []string{"tool", "outcome"})

	duration := prometheus.NewHistogramVec(prometheus.HistogramOpts{
		Name: "tesserix_mcp_tool_duration_seconds",
		Help: "Tool call latency. The engine budgets 400ms p99 per tool; this is how you know whether that contract holds.",
		// Bucketed around the 400ms contract rather than Prometheus defaults.
		Buckets:     []float64{0.01, 0.025, 0.05, 0.1, 0.2, 0.4, 0.8, 2},
		ConstLabels: labels,
	}, []string{"tool", "outcome"})

	if err := reg.Register(calls); err != nil {
		return nil, err
	}
	if err := reg.Register(duration); err != nil {
		return nil, err
	}
	return &ToolMetrics{calls: calls, duration: duration}, nil
}

// Observe records one completed tool call.
func (m *ToolMetrics) Observe(tool string, outcome Outcome, d time.Duration) {
	m.calls.WithLabelValues(tool, string(outcome)).Inc()
	m.duration.WithLabelValues(tool, string(outcome)).Observe(d.Seconds())
}
```

- [ ] **Step 4: Run test to verify it passes**

Run: `cd ../go-shared && go test -race ./mcp/observe/ -v`
Expected: PASS, 2 tests.

- [ ] **Step 5: Commit**

```bash
cd ../go-shared
git add mcp/observe/
git commit -m "feat(mcp): per-tool call and latency metrics"
```

---

### Task 6: The boundaries, asserted against the real import graph

**Files:**
- Create: `mcp/boundaries_test.go`

**Interfaces:**
- Consumes: every package from Tasks 1–5 (by import path only).
- Produces: nothing importable. This task's deliverable is a gate.

- [ ] **Step 1: Write the failing test**

The obvious tool for this is `golang.org/x/tools/go/packages`, but it is not a
dependency and the Global Constraints forbid adding one to enforce a rule about
dependencies. Shell out to the toolchain instead:

```go
package mcp_test

import (
	"os/exec"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

// deps returns the full transitive import list of the mcp package tree, as the
// toolchain itself computes it. Shelling out to `go list` avoids adding
// golang.org/x/tools as a dependency purely to enforce a rule about
// dependencies.
func deps(t *testing.T) []string {
	t.Helper()
	out, err := exec.Command("go", "list", "-deps", "./...").Output()
	require.NoError(t, err, "go list must succeed")
	return strings.Fields(string(out))
}

// D9: the SDK binding lives in the consuming service, never here. If this
// fails, a protocol bump has just become a go-shared release for ~30 services.
func TestFoundationImportsNoMCPSDK(t *testing.T) {
	banned := []string{
		"modelcontextprotocol",
		"mcp-go",
		"mark3labs",
	}
	for _, dep := range deps(t) {
		for _, b := range banned {
			require.NotContains(t, dep, b,
				"go-shared/mcp must not import an MCP SDK (D9); found %s", dep)
		}
	}
}

// D2: the foundation is domain-free. A product import here would make every
// service that imports go-shared depend on one product's model.
func TestFoundationImportsNoProductPackage(t *testing.T) {
	banned := []string{
		"github.com/mark8ly/",
		"github.com/tesserix/tesserix-home",
		"github.com/tesserix/australis",
	}
	for _, dep := range deps(t) {
		for _, b := range banned {
			require.NotContains(t, dep, b,
				"go-shared/mcp must not import a product package (D2); found %s", dep)
		}
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Prove the gate has teeth before trusting it. Adding a real product import would
only produce a module-resolution error, which tests the toolchain rather than
the assertion — so mutate the assertion's own input instead. Temporarily add a
package the foundation genuinely does import to the banned list in
`TestFoundationImportsNoMCPSDK`:

```go
	banned := []string{
		"modelcontextprotocol",
		"mcp-go",
		"mark3labs",
		"prometheus", // TEMPORARY — mcp/observe really does import this
	}
```

Run: `cd ../go-shared && go test ./mcp/ -run TestFoundationImportsNoMCPSDK -v`
Expected: FAIL, naming the prometheus dependency. That proves `go list -deps`
is returning a real graph and the assertion reads it correctly — a test that
passes because it inspected an empty list would look identical to a passing
gate.

Remove that line again and confirm it returns to PASS.

- [ ] **Step 3: Run the tests to verify they pass**

Run: `cd ../go-shared && go test -race ./mcp/... -v`
Expected: PASS — every test from Tasks 1–5 plus both boundary tests.

- [ ] **Step 4: Full module verification**

Run: `cd ../go-shared && go build ./... && go vet ./... && go test -race ./...`
Expected: all pass. Confirm `git diff --stat origin/main -- go.mod go.sum` is
**empty** — this plan adds no dependency.

- [ ] **Step 5: Commit**

```bash
cd ../go-shared
git add mcp/boundaries_test.go
git commit -m "test(mcp): keep the foundation free of SDKs and product packages"
```

---

### Task 7: Release the foundation

**Files:**
- Modify: none — this task produces a tag and a PR.

**Interfaces:**
- Consumes: Tasks 1–6.
- Produces: a `go-shared` minor version consumable as
  `github.com/tesserix/go-shared/mcp`. mark8ly's `mcp-catalog` plan starts here.

- [ ] **Step 1: Push the branch and open the PR**

`gh` writes must be direct Bash commands, never inside a script, and the PR
must be created from the main checkout rather than a worktree.

```bash
cd ../go-shared
git push -u origin feat/mcp-foundation
gh pr create -R tesserix/go-shared -B main -H feat/mcp-foundation \
  --title "feat(mcp): the connector foundation — closed schemas, GET-only upstream, fail-closed auth" \
  --body-file /tmp/mcp-foundation-pr.md
```

Write `/tmp/mcp-foundation-pr.md` first with a quoted heredoc (`<<'BODY'`) so
backticks survive. It must state: no new dependency; the four structural
guarantees (both schemas required, GET-only, fail-closed key, SDK-free); and
that no service consumes it yet, so the blast radius of this release is zero.

- [ ] **Step 2: Confirm CI is green**

The repo's workflow runs `go build`, `go vet`, `go test -race -coverprofile`
and a non-blocking `gofmt` report. Before pushing, run `gofmt -l mcp/` locally
and fix anything it lists — the format job is non-blocking precisely because of
a pre-existing backlog, and new files must not add to it.

- [ ] **Step 3: Merge, then tag**

After the PR merges, tag the release. The current tag is `v1.9.1`; this is a new
package with no change to existing ones, so it is a **minor** bump.

```bash
cd ../go-shared
git checkout main && git pull --ff-only
git tag v1.10.0
git push origin v1.10.0
```

- [ ] **Step 4: Verify it is consumable**

```bash
cd /tmp && rm -rf mcpcheck && mkdir mcpcheck && cd mcpcheck
go mod init check
GOPROXY=direct go get github.com/tesserix/go-shared@v1.10.0
```
Expected: resolves. This is the only proof that the tag is real and fetchable —
a merged PR is not a released library.

---

## What this plan deliberately does NOT do

- **No mark8ly changes.** `mcp-catalog` is a separate plan, blocked on the SDK
  decision (spec open question 2). Nothing here depends on that decision, which
  is why this half can ship first.
- **No `gofmt` backlog cleanup** across the rest of go-shared. Tempting while
  here; unrelated, and it would bury this diff.
- **No transport, no server, no protocol handling.** By D9.
- **No registry record or Kubernetes manifest.** Those belong to the connector,
  not the foundation.

## Self-review notes

Checked against the spec: D5 is Task 2 (generic `Register` plus the interface-
output rejection), D6 is Task 3 (`TestClient_ExposesOnlyGet`), D9 is Tasks 2 and
6 (`doc.go` plus the import-graph gate), D2's domain-freedom is Task 6. The
spec's "schema registry / upstream / auth / observe" four-part foundation maps
to Tasks 1+2 / 3 / 4 / 5 respectively.

Two spec items are intentionally out of this plan's scope and belong to the
connector plan: the **declared-vs-served** CI check (needs a registry record and
a running server) and the **projection tests against recorded marketplace-api
responses** (needs the catalog domain package). `Registry.Names()` is built here
so the first of those has something to compare against.
