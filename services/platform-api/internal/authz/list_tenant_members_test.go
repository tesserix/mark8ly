package authz

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

// newTestFGAClient builds a real (non-fake) Client wired to an httptest
// server running handler, the way the production authz.New would be wired
// to a real OpenFGA instance. The server is closed automatically via
// t.Cleanup.
func newTestFGAClient(t *testing.T, handler http.HandlerFunc) Client {
	t.Helper()

	srv := httptest.NewServer(handler)
	t.Cleanup(srv.Close)

	c, err := New(Config{APIURL: srv.URL, StoreID: "01H8XGJVBWQAK1SDS1KWQZKZ8J"})
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	return c
}

// readTuple builds one JSON tuple in the shape OpenFGA's Read response
// returns.
func readTuple(user, relation, object string) map[string]any {
	return map[string]any{
		"key": map[string]any{
			"user":     user,
			"relation": relation,
			"object":   object,
		},
		"timestamp": time.Now().UTC().Format(time.RFC3339),
	}
}

// TestListTenantMembers_PagesThroughAllResults pins the pagination
// contract: OpenFGA's Read endpoint returns results one page at a time
// with a continuation_token, and ListTenantMembers MUST keep calling
// Read with that token until the server reports none left. A version
// that only reads the first page would silently drop every member past
// page 1 — exactly the bug #361 warns is easy to introduce.
func TestListTenantMembers_PagesThroughAllResults(t *testing.T) {
	pages := [][]map[string]any{
		{
			readTuple("user:owner-1", "owner", "tenant:t1"),
			readTuple("user:admin-1", "admin", "tenant:t1"),
		},
		{
			readTuple("user:staff-1", "staff", "tenant:t1"),
		},
		{
			readTuple("user:viewer-1", "viewer", "tenant:t1"),
		},
	}
	tokens := []string{"page-2-token", "page-3-token", ""}

	var callCount int
	client := newTestFGAClient(t, func(w http.ResponseWriter, r *http.Request) {
		var body struct {
			ContinuationToken string `json:"continuation_token"`
		}
		_ = json.NewDecoder(r.Body).Decode(&body)

		// Figure out which page was requested from the continuation
		// token the client sent (empty string means "first page").
		pageIdx := 0
		for i := 1; i < len(tokens); i++ {
			if tokens[i-1] == body.ContinuationToken {
				pageIdx = i
				break
			}
		}
		callCount++

		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"tuples":             pages[pageIdx],
			"continuation_token": tokens[pageIdx],
		})
	})

	members, err := client.ListTenantMembers(context.Background(), "t1")
	require.NoError(t, err)
	require.Equal(t, 3, callCount, "must follow the continuation token across all 3 pages")

	require.ElementsMatch(t, []Member{
		{UserID: "owner-1", Relation: "owner"},
		{UserID: "admin-1", Relation: "admin"},
		{UserID: "staff-1", Relation: "staff"},
		{UserID: "viewer-1", Relation: "viewer"},
	}, members)
}

// TestListTenantMembers_EmptyResult verifies a tenant with no direct role
// tuples (e.g. already torn down) returns an empty slice and no error,
// rather than panicking on a nil response.
func TestListTenantMembers_EmptyResult(t *testing.T) {
	client := newTestFGAClient(t, func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"tuples":             []map[string]any{},
			"continuation_token": "",
		})
	})

	members, err := client.ListTenantMembers(context.Background(), "t1")
	require.NoError(t, err)
	require.Empty(t, members)
}

// TestListTenantMembers_ServerError verifies a transport/API failure is
// wrapped and returned rather than swallowed.
func TestListTenantMembers_ServerError(t *testing.T) {
	client := newTestFGAClient(t, func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
		_, _ = w.Write([]byte(`{"code":"internal_error","message":"boom"}`))
	})

	_, err := client.ListTenantMembers(context.Background(), "t1")
	require.Error(t, err)
}
