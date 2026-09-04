package main

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	gsmcp "github.com/tesserix/go-shared/mcp"
	"github.com/tesserix/go-shared/mcp/auth"
	"github.com/tesserix/go-shared/mcp/observe"

	"github.com/mark8ly/mcp/internal/catalog"
	"github.com/mark8ly/mcp/internal/server"
)

// servedProtocolVersion is the protocol revision the estate's registry records
// pin. internal/server keeps the authoritative unexported copy; this is the
// wire value a client must send, and the tools/list below fails loudly if the
// two ever drift.
const servedProtocolVersion = "2026-07-28"

// TestServedSchemas_OfTheFiveRealTools closes the gap the other three schema
// tests leave open. server_test.go proves closedness for a two-field `ping`
// fixture; catalog's tools_test.go checks the registry's in-memory COPY;
// declared_test.go compares names only. None of them asserts that what is
// SERVED matches what the registry declares FOR THE TOOLS WE ACTUALLY SHIP —
// which is the assertion the whole schema design rests on.
func TestServedSchemas_OfTheFiveRealTools(t *testing.T) {
	tools := servedTools(t)

	names := make([]string, 0, len(tools))
	for name := range tools {
		names = append(names, name)
	}
	assert.ElementsMatch(t, declaredTools, names,
		"the SERVED tool list must be exactly the five declared tools")

	for _, name := range declaredTools {
		tool, ok := tools[name]
		require.True(t, ok, "tool %q is declared but not served", name)

		for _, field := range []string{"inputSchema", "outputSchema"} {
			sch, ok := tool[field].(map[string]any)
			require.True(t, ok, "served tool %q has no %s", name, field)
			assert.Equal(t, false, sch["additionalProperties"],
				"the SERVED %s of %q must be closed", field, name)
		}

		// Every input property is something a model has to fill in from a
		// description alone. One without a description is a tool the model
		// has to guess at.
		props, ok := tool["inputSchema"].(map[string]any)["properties"].(map[string]any)
		require.True(t, ok, "served tool %q has no input properties", name)
		require.NotEmpty(t, props, "served tool %q declares no inputs", name)
		for prop, raw := range props {
			desc, _ := raw.(map[string]any)["description"].(string)
			assert.NotEmpty(t, desc,
				"input property %q of served tool %q carries no description", prop, name)
		}
	}
}

// servedTools builds the REAL registry, serves it through the real server, and
// returns the tools/list response keyed by name.
func servedTools(t *testing.T) map[string]map[string]any {
	t.Helper()

	client, err := catalog.NewClient("http://storefront.invalid", "sfkey", time.Second)
	require.NoError(t, err)

	registry := gsmcp.NewRegistry()
	require.NoError(t, catalog.RegisterTools(registry, client))

	metrics, err := observe.NewToolMetrics(prometheus.NewRegistry(), metricsServiceName)
	require.NoError(t, err)

	h, err := server.New(registry, "s3cret", metrics)
	require.NoError(t, err)

	body := `{"jsonrpc":"2.0","id":1,"method":"tools/list","params":{"_meta":{` +
		`"io.modelcontextprotocol/protocolVersion":"` + servedProtocolVersion + `",` +
		`"io.modelcontextprotocol/clientCapabilities":{}}}}`
	req := httptest.NewRequest(http.MethodPost, "/mcp", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json, text/event-stream")
	// From 2026-07-28 the SDK requires the method to be mirrored into a header
	// and to agree with the body; without it the answer is a 400 that reads
	// like an auth failure.
	req.Header.Set("Mcp-Protocol-Version", servedProtocolVersion)
	req.Header.Set("Mcp-Method", "tools/list")
	req.Header.Set(auth.HeaderName, "s3cret")

	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	require.Equal(t, http.StatusOK, rec.Code, "body: %s", rec.Body.String())

	// The streamable handler may answer with bare JSON or an SSE frame.
	raw := rec.Body.String()
	i := strings.Index(raw, "{")
	require.GreaterOrEqual(t, i, 0, "no JSON in body: %s", raw)
	var payload struct {
		Result struct {
			Tools []map[string]any `json:"tools"`
		} `json:"result"`
	}
	require.NoError(t, json.Unmarshal([]byte(raw[i:]), &payload), "body: %s", raw)

	out := make(map[string]map[string]any, len(payload.Result.Tools))
	for _, tool := range payload.Result.Tools {
		name, _ := tool["name"].(string)
		require.NotEmpty(t, name, "served tool with no name; body: %s", raw)
		out[name] = tool
	}
	return out
}
