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

	srv := mcpsdk.NewServer(&mcpsdk.Implementation{
		Name:    serverName,
		Version: serverVersion,
	}, nil)

	for _, tool := range tools {
		if err := addTool(srv, tool, m); err != nil {
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

// addTool registers one registry tool with the SDK server.
//
// The SDK's AddTool panics on a tool it will not accept — an invalid name, a
// schema it cannot resolve. A panic during construction of an HTTP handler is
// not a useful failure mode for a service that has a config error to report,
// so it is recovered into an error here. Nothing else in this package recovers
// a panic.
func addTool(srv *mcpsdk.Server, tool gsmcp.Tool, m *observe.ToolMetrics) (err error) {
	defer func() {
		if r := recover(); r != nil {
			err = fmt.Errorf("server: registering tool %q: %v", tool.Name, r)
		}
	}()

	// name is captured for metrics from the REGISTRY, never from the wire: the
	// name in a tools/call request is caller-supplied, and feeding it into a
	// label would let a caller mint unbounded metric cardinality.
	name := tool.Name
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

		start := time.Now()
		out, err := invoke(ctx, args)
		m.Observe(name, observe.OutcomeFor(err), time.Since(start))
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
