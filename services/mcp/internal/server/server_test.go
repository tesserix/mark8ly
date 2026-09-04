package server

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/stretchr/testify/require"
	gsmcp "github.com/tesserix/go-shared/mcp"
	"github.com/tesserix/go-shared/mcp/auth"
	"github.com/tesserix/go-shared/mcp/observe"
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

func testMetrics(t *testing.T) *observe.ToolMetrics {
	t.Helper()
	m, err := observe.NewToolMetrics(prometheus.NewRegistry(), "mcp-catalog")
	require.NoError(t, err)
	return m
}

// An unauthenticated call must never reach the MCP machinery.
func TestServer_RejectsMissingKey(t *testing.T) {
	h, err := New(testRegistry(t), "s3cret", testMetrics(t))
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
	h, err := New(testRegistry(t), "s3cret", testMetrics(t))
	require.NoError(t, err)

	rec := post(t, h, "tools/list", "", `{"jsonrpc":"2.0","id":1,"method":"tools/list","params":{"_meta":{`+
		`"io.modelcontextprotocol/protocolVersion":"`+protocolVersion+`",`+
		`"io.modelcontextprotocol/clientCapabilities":{}}}}`)

	require.Equal(t, http.StatusOK, rec.Code, "body: %s", rec.Body.String())
	raw := rec.Body.String()
	require.Contains(t, raw, `"ping"`)

	// Dig the served schemas out of the response and assert BOTH are closed.
	tool := firstTool(t, raw)
	for _, field := range []string{"inputSchema", "outputSchema"} {
		sch, ok := tool[field].(map[string]any)
		require.True(t, ok, "served tool has no %s; body: %s", field, raw)
		require.Equal(t, false, sch["additionalProperties"],
			"the SERVED %s must be closed, not merely the registry's copy; body: %s", field, raw)
	}

	// The description the registry derived must survive the crossing too — a
	// schema that is closed but stripped of its property descriptions is not
	// the registry's schema.
	props := tool["inputSchema"].(map[string]any)["properties"].(map[string]any)
	require.Equal(t, "Text to echo back",
		props["echo"].(map[string]any)["description"], "body: %s", raw)
}

// A tool call must reach the registry handler and come back as structured
// content, so the schema wiring is not merely decorative.
func TestServer_CallsToolThroughRegistry(t *testing.T) {
	h, err := New(testRegistry(t), "s3cret", testMetrics(t))
	require.NoError(t, err)

	rec := post(t, h, "tools/call", "ping", `{"jsonrpc":"2.0","id":1,"method":"tools/call","params":{`+
		`"name":"ping","arguments":{"echo":"bondi"},"_meta":{`+
		`"io.modelcontextprotocol/protocolVersion":"`+protocolVersion+`",`+
		`"io.modelcontextprotocol/clientCapabilities":{}}}}`)

	require.Equal(t, http.StatusOK, rec.Code, "body: %s", rec.Body.String())
	result := resultOf(t, rec.Body.String())
	require.NotEqual(t, true, result["isError"], "body: %s", rec.Body.String())
	structured, ok := result["structuredContent"].(map[string]any)
	require.True(t, ok, "body: %s", rec.Body.String())
	require.Equal(t, "bondi", structured["echo"])
}

// An argument the closed input schema does not declare must be refused, not
// quietly dropped.
func TestServer_RejectsUndeclaredArgument(t *testing.T) {
	h, err := New(testRegistry(t), "s3cret", testMetrics(t))
	require.NoError(t, err)

	rec := post(t, h, "tools/call", "ping", `{"jsonrpc":"2.0","id":1,"method":"tools/call","params":{`+
		`"name":"ping","arguments":{"echo":"bondi","sneaky":"x"},"_meta":{`+
		`"io.modelcontextprotocol/protocolVersion":"`+protocolVersion+`",`+
		`"io.modelcontextprotocol/clientCapabilities":{}}}}`)

	require.Equal(t, http.StatusOK, rec.Code, "body: %s", rec.Body.String())
	result := resultOf(t, rec.Body.String())
	require.Equal(t, true, result["isError"],
		"an undeclared argument must be an error; body: %s", rec.Body.String())
}

func TestNew_RejectsBlankKeyAndEmptyRegistry(t *testing.T) {
	_, err := New(testRegistry(t), "", testMetrics(t))
	require.Error(t, err, "a blank key would serve an open endpoint")

	_, err = New(gsmcp.NewRegistry(), "s3cret", testMetrics(t))
	require.Error(t, err, "a server with no tools is a misconfiguration, not a server")

	_, err = New(nil, "s3cret", testMetrics(t))
	require.Error(t, err)

	_, err = New(testRegistry(t), "s3cret", nil)
	require.Error(t, err)
}

// --- helpers ---------------------------------------------------------------

// post sends one JSON-RPC request at the pinned protocol version. From
// 2026-07-28 the SDK requires the method (and, for tools/call, the tool name)
// to be mirrored into headers and to agree with the body.
func post(t *testing.T, h http.Handler, method, toolName, body string) *httptest.ResponseRecorder {
	t.Helper()
	req := httptest.NewRequest(http.MethodPost, "/mcp", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json, text/event-stream")
	// The estate's registry records pin this protocol version, and the SDK
	// accepts it over streamable HTTP only when the handler is stateless.
	req.Header.Set("Mcp-Protocol-Version", protocolVersion)
	req.Header.Set("Mcp-Method", method)
	if toolName != "" {
		req.Header.Set("Mcp-Name", toolName)
	}
	req.Header.Set(auth.HeaderName, "s3cret")
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	return rec
}

// decode pulls the JSON-RPC response object out of the recorded body. The
// streamable handler may answer with either bare JSON or a text/event-stream
// frame, so skip to the first '{'.
func decode(t *testing.T, raw string) map[string]any {
	t.Helper()
	i := strings.Index(raw, "{")
	require.GreaterOrEqual(t, i, 0, "no JSON in body: %s", raw)
	var payload map[string]any
	require.NoError(t, json.Unmarshal([]byte(raw[i:]), &payload), "body: %s", raw)
	return payload
}

func resultOf(t *testing.T, raw string) map[string]any {
	t.Helper()
	payload := decode(t, raw)
	result, ok := payload["result"].(map[string]any)
	require.True(t, ok, "no result in body: %s", raw)
	return result
}

func firstTool(t *testing.T, raw string) map[string]any {
	t.Helper()
	tools, ok := resultOf(t, raw)["tools"].([]any)
	require.True(t, ok, "no tools in body: %s", raw)
	require.Len(t, tools, 1)
	return tools[0].(map[string]any)
}
