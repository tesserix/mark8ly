// Package server binds the go-shared MCP registry to the Model Context
// Protocol SDK and serves it over streamable HTTP behind a shared-key gate.
//
// # Which schema is authoritative: go-shared (approach A)
//
// Both halves of this wiring can derive a JSON Schema from a Go type.
// go-shared's Register[In, Out] derives one and closes it
// (additionalProperties:false); the SDK's generic AddTool[In, Out] derives its
// own from the type parameters. Two derivations of one contract is a fork
// waiting to happen, and if the SDK's derivation were the one served and it
// were open, the closed-schema guarantee would be broken on the wire no matter
// what our registry held.
//
// So the registry is authoritative and the SDK derives nothing. Every tool is
// registered with mcp.Tool.InputSchema and .OutputSchema already populated
// from the registry's own map[string]any. The SDK's setSchema only reflects
// over the type parameters when the corresponding field is nil (see
// server.go:setSchema in modelcontextprotocol/go-sdk v1.7.0); a schema that is
// already set is kept, resolved for validation, and — because Tool.InputSchema
// is typed `any` — serialised to the wire exactly as supplied. The maps are
// passed through verbatim rather than round-tripped into the SDK's
// *jsonschema.Schema, so no field can be dropped in translation.
//
// The type parameters are therefore deliberately degenerate: In is
// json.RawMessage and Out is any, which is precisely the pair for which the
// SDK derives nothing. Arguments are handed to the registry's own Invoke,
// which decodes them into the handler's real input type with
// DisallowUnknownFields — so the closed schema is enforced by the same code
// that published it. TestServer_ServesClosedSchemaFromRegistry asserts the
// SERVED schemas carry additionalProperties:false; that assertion, not this
// comment, is what keeps the guarantee true.
//
// # Why the handler is stateless
//
// Stateless is mandatory, not a preference. The SDK accepts protocol version
// 2026-07-28 over streamable HTTP only in stateless mode; a stateful handler
// rejects that version outright, and every product MCP record in the estate's
// registry pins it. Stateless also means the server cannot make
// client-initiated requests, which costs nothing here: v1 is read-only and no
// catalog tool calls back to the client.
package server

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"time"

	mcpsdk "github.com/modelcontextprotocol/go-sdk/mcp"
	gsmcp "github.com/tesserix/go-shared/mcp"
	"github.com/tesserix/go-shared/mcp/auth"
	"github.com/tesserix/go-shared/mcp/observe"
)

// protocolVersion is the MCP protocol revision every product MCP record in
// the estate's registry pins. It lives in this package — the SDK binding —
// rather than in go-shared, so a protocol bump is a change to the connectors
// that serve MCP and not a go-shared release affecting ~30 services.
const protocolVersion = "2026-07-28"

// methodCallTool is the JSON-RPC method name for a tool invocation. The SDK
// keeps its own copy unexported, so the middleware carries this one.
const methodCallTool = "tools/call"

const (
	// serverName is the implementation name reported in the MCP initialize
	// handshake. It matches the estate registry's record for this connector.
	serverName = "mark8ly-catalog"

	// serverVersion is the implementation version reported in the handshake.
	// It is a hardcoded placeholder, not the build version: link-time
	// stamping needs a Dockerfile build-arg plus either a signature change to
	// New or a settable package var, and that is deployment work deferred
	// alongside the chart, the ExternalSecret, and the CI image build. Until
	// that follow-up lands, every deployed build reports "0.1.0" in the MCP
	// handshake regardless of what is actually running.
	serverVersion = "0.1.0"

	// maxRequestBodyBytes caps a single MCP request. The tools are read-only
	// lookups whose arguments are a handful of slugs; anything larger is a
	// mistake or an attack, and refusing it early keeps the parse bounded.
	maxRequestBodyBytes = 1 << 20 // 1 MiB

	// emptyArguments is what an omitted `arguments` object decodes as. The
	// registry's Invoke expects a JSON object, and an absent object means "no
	// arguments were supplied", not "malformed request".
	emptyArguments = "{}"
)

// New returns an http.Handler serving the registry's tools over MCP streamable
// HTTP, gated on the shared key.
//
// The key check wraps everything, so an unauthenticated request is refused
// before any MCP parsing happens. A blank key is an error rather than an open
// endpoint: the key arrives from a mounted secret, so a missing one means the
// deployment is misconfigured.
func New(reg *gsmcp.Registry, key string, m *observe.ToolMetrics) (http.Handler, error) {
	if reg == nil {
		return nil, errors.New("server: registry is nil")
	}
	if key == "" {
		return nil, errors.New("server: shared key is empty; refusing to serve an unauthenticated endpoint")
	}
	if m == nil {
		return nil, errors.New("server: tool metrics are nil")
	}

	tools := reg.Tools()
	if len(tools) == 0 {
		return nil, errors.New("server: registry has no tools")
	}

	// Registered names, captured once. The name in a tools/call request is
	// caller-supplied; recording a metric under it without checking it against
	// this set would let a caller mint unbounded label cardinality.
	registered := make(map[string]struct{}, len(tools))
	for _, tool := range tools {
		registered[tool.Name] = struct{}{}
	}

	srv := mcpsdk.NewServer(&mcpsdk.Implementation{
		Name:    serverName,
		Version: serverVersion,
	}, nil)

	srv.AddReceivingMiddleware(observeCalls(registered, m))

	for _, tool := range tools {
		if err := addTool(srv, tool); err != nil {
			return nil, err
		}
	}

	handler := mcpsdk.NewStreamableHTTPHandler(
		func(*http.Request) *mcpsdk.Server { return srv },
		&mcpsdk.StreamableHTTPOptions{
			Stateless:           true,
			MaxRequestBodyBytes: maxRequestBodyBytes,
		},
	)

	return auth.RequireKey(key, handler), nil
}

// outcomeCell carries the outcome the registry handler observed back up to
// observeCalls. It exists because the two halves of a tools/call live on
// opposite sides of the SDK: the middleware is the only place that sees a call
// the SDK rejected before any handler ran, and the handler is the only place
// that knows WHY a call that did run failed.
type outcomeCell struct {
	set     bool
	outcome observe.Outcome
}

// outcomeCellKey keys the cell in the request context.
type outcomeCellKey struct{}

// observeCalls records exactly one metric per tools/call, including the calls
// that never reach a handler.
//
// The SDK validates `arguments` against the input schema and unmarshals them
// BEFORE invoking the tool handler (see toolForErr in the SDK's server.go);
// both failures are returned as a CallToolResult with IsError set and no error
// to the middleware. Observing only inside the handler therefore counted
// nothing for `{}`, `{"page":1}`, `arguments:null`, or an undeclared field —
// so a connector being hammered with malformed calls was indistinguishable
// from one nobody was calling.
//
// Every one of those pre-handler rejections is a caller-input failure, so a
// tools/call for a REGISTERED tool that left the cell unset is recorded as
// OutcomeInvalidInput. A call that did reach the handler carries the outcome
// the handler classified. A call naming a tool that is not registered records
// nothing at all.
func observeCalls(registered map[string]struct{}, m *observe.ToolMetrics) mcpsdk.Middleware {
	return func(next mcpsdk.MethodHandler) mcpsdk.MethodHandler {
		return func(ctx context.Context, method string, req mcpsdk.Request) (mcpsdk.Result, error) {
			// CallToolParamsRaw, not CallToolParams: at the method-handler
			// seam the arguments are still raw — unmarshalling them is what
			// the tool handler does next, and what can fail.
			params, ok := req.GetParams().(*mcpsdk.CallToolParamsRaw)
			if method != methodCallTool || !ok {
				return next(ctx, method, req)
			}
			name := params.Name
			if _, known := registered[name]; !known {
				return next(ctx, method, req)
			}

			cell := &outcomeCell{}
			start := time.Now()
			res, err := next(context.WithValue(ctx, outcomeCellKey{}, cell), method, req)
			outcome := observe.OutcomeInvalidInput
			if cell.set {
				outcome = cell.outcome
			}
			m.Observe(name, outcome, time.Since(start))
			return res, err
		}
	}
}

// addTool registers one registry tool with the SDK server.
//
// The SDK's AddTool panics on a tool it will not accept — an invalid name, a
// schema it cannot resolve. A panic during construction of an HTTP handler is
// not a useful failure mode for a service that has a config error to report,
// so it is recovered into an error here. Nothing else in this package recovers
// a panic.
func addTool(srv *mcpsdk.Server, tool gsmcp.Tool) (err error) {
	defer func() {
		if r := recover(); r != nil {
			err = fmt.Errorf("server: registering tool %q: %v", tool.Name, r)
		}
	}()

	invoke := tool.Invoke

	mcpsdk.AddTool(srv, &mcpsdk.Tool{
		Name:         tool.Name,
		Description:  tool.Description,
		InputSchema:  tool.InputSchema,
		OutputSchema: tool.OutputSchema,
	}, func(ctx context.Context, _ *mcpsdk.CallToolRequest, args json.RawMessage) (*mcpsdk.CallToolResult, any, error) {
		if len(args) == 0 {
			args = json.RawMessage(emptyArguments)
		}

		out, err := invoke(ctx, args)
		// Hand the classification up to observeCalls, which owns the single
		// Observe per call and is also what covers the calls that never get
		// this far. A missing cell means this handler was reached some other
		// way than through the middleware, which cannot happen in New.
		if cell, ok := ctx.Value(outcomeCellKey{}).(*outcomeCell); ok {
			cell.set = true
			cell.outcome = observe.OutcomeFor(err)
		}
		if err != nil {
			// Returned as a tool error, not a protocol error: the SDK packs it
			// into CallToolResult with IsError set, which is what an agent can
			// actually act on.
			return nil, nil, err
		}
		return nil, out, nil
	})

	return nil
}
