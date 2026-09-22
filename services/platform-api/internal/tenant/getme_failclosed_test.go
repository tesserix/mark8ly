package tenant

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

// getMe decides the caller's role, and the admin BFF forwards whatever it
// says into server components. Answering "owner" without asking anything
// is a privilege escalation, so it must never be the answer to "I could
// not check".
func TestGetMe_WithoutAuthzDoesNotGrantOwner(t *testing.T) {
	gin.SetMode(gin.TestMode)

	// fga nil is the state a failed OpenFGA store discovery used to leave
	// the process in: reachable, serving, and authorising nothing.
	h := NewHandler(nil, nil)

	r := gin.New()
	r.GET("/tenants/:id/me", h.getMe)

	w := httptest.NewRecorder()
	r.ServeHTTP(w, httptest.NewRequest(
		"GET", "/tenants/"+testTenantID+"/me?uid=someone-with-no-role", nil))

	require.NotEqual(t, http.StatusOK, w.Code,
		"a role lookup that could not run must not succeed")

	var body map[string]any
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &body))
	require.NotContains(t, w.Body.String(), "owner",
		"no unauthenticated caller may be told they are the owner")

	if data, ok := body["data"].(map[string]any); ok {
		require.Empty(t, data["role"])
	}
}
