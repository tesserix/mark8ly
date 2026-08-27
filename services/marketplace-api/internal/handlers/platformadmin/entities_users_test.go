package platformadmin_test

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"

	"github.com/mark8ly/marketplace-api/internal/estateuserdir"
	"github.com/mark8ly/marketplace-api/internal/handlers/platformadmin"
)

type stubUserDirectory struct {
	res       *estateuserdir.ListResult
	err       error
	gotParams estateuserdir.ListParams
}

func (s *stubUserDirectory) List(_ context.Context, p estateuserdir.ListParams) (*estateuserdir.ListResult, error) {
	s.gotParams = p
	return s.res, s.err
}

func usersRouter(t *testing.T, dir platformadmin.EstateUserDirectory) *gin.Engine {
	t.Helper()
	gin.SetMode(gin.TestMode)
	r := gin.New()
	platformadmin.NewEntitiesUsersHandler(dir, nil).Register(r.Group(""))
	return r
}

// §8.9: every /admin/entities/{type} row carries a non-empty id and label,
// an optional non-empty sublabel, and NO source. This is the same rule that
// the admin-conformance CronJob caught mark8ly violating on entities/tenants
// — building the second entity type without it would repeat that exactly.
func TestEntitiesUsersRowsSatisfyContract89(t *testing.T) {
	dir := &stubUserDirectory{res: &estateuserdir.ListResult{
		Total: 2, Page: 1, Limit: 50,
		Users: []estateuserdir.User{
			{Email: "owner@example.com", UserID: "uid-1", Roles: "owner", TenantName: "Acme Trading", TenantCount: 1},
			// No tenant name and multiple tenants: label must still be
			// non-empty, and sublabel must be OMITTED rather than sent empty.
			{Email: "multi@example.com", Roles: "admin,owner", TenantCount: 3},
		},
	}}

	rec := httptest.NewRecorder()
	usersRouter(t, dir).ServeHTTP(rec, httptest.NewRequest(
		http.MethodGet, "/admin/entities/users", nil))
	require.Equal(t, http.StatusOK, rec.Code)

	var body struct {
		Data []map[string]any `json:"data"`
	}
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &body))
	require.Len(t, body.Data, 2)

	for i, row := range body.Data {
		for _, field := range []string{"id", "label"} {
			v, ok := row[field]
			require.Truef(t, ok, "row %d has no %s; §8.9 requires it", i, field)
			s, ok := v.(string)
			require.Truef(t, ok, "row %d %s must be a string", i, field)
			require.NotEmptyf(t, s, "row %d has an empty %s; §8.9 requires non-empty", i, field)
		}
		if v, present := row["sublabel"]; present {
			s, ok := v.(string)
			require.Truef(t, ok, "row %d sublabel must be a string", i)
			require.NotEmptyf(t, s, "row %d sends an empty sublabel; §8.9 requires omitting it", i)
		}
		require.NotContainsf(t, row, "source", "row %d sends source; §8.9 forbids it", i)
	}

	require.Equal(t, "owner@example.com", body.Data[0]["id"])
	_, hasSublabel := body.Data[1]["sublabel"]
	require.False(t, hasSublabel, "a user with no tenant name must omit sublabel, not send \"\"")
}

// The console's global search passes q; it must reach platform-api rather
// than being applied to an already-paged response.
func TestEntitiesUsersForwardsSearchAndPaging(t *testing.T) {
	dir := &stubUserDirectory{res: &estateuserdir.ListResult{}}
	rec := httptest.NewRecorder()
	usersRouter(t, dir).ServeHTTP(rec, httptest.NewRequest(
		http.MethodGet, "/admin/entities/users?q=acme&page=3&limit=25", nil))

	require.Equal(t, http.StatusOK, rec.Code)
	require.Equal(t, "acme", dir.gotParams.Q)
	require.Equal(t, 3, dir.gotParams.Page)
	require.Equal(t, 25, dir.gotParams.Limit)
}

// Empty is 200 with [], never null.
func TestEntitiesUsersEmptyIsArray(t *testing.T) {
	dir := &stubUserDirectory{res: &estateuserdir.ListResult{}}
	rec := httptest.NewRecorder()
	usersRouter(t, dir).ServeHTTP(rec, httptest.NewRequest(
		http.MethodGet, "/admin/entities/users", nil))

	require.Equal(t, http.StatusOK, rec.Code)
	var body struct {
		Data json.RawMessage `json:"data"`
	}
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &body))
	require.Equal(t, "[]", string(body.Data))
}

// An unreachable directory must be 503, never an empty 200: "no users" and
// "we could not ask" are different answers.
func TestEntitiesUsersUnavailableIsNotAnEmptyList(t *testing.T) {
	dir := &stubUserDirectory{err: estateuserdir.ErrUnavailable}
	rec := httptest.NewRecorder()
	usersRouter(t, dir).ServeHTTP(rec, httptest.NewRequest(
		http.MethodGet, "/admin/entities/users", nil))
	require.Equal(t, http.StatusServiceUnavailable, rec.Code)
}
