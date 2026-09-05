//go:build integration

package platformadmin_test

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/mark8ly/marketplace-api/internal/emailtemplates"
	"github.com/mark8ly/marketplace-api/internal/handlers/platformadmin"
	"github.com/mark8ly/marketplace-api/pkg/testdb"
)

// The acceptance criterion for #730: the SAME key replays the stored body
// and performs NO second UPSERT, while a DIFFERENT key is a new write.
//
// Counting the store's Upsert calls is what distinguishes real idempotency
// from a coincidentally identical response — the retry hazard here is not a
// wrong body, it is the version bump and the extra revision row a second
// UPSERT would leave behind.
func putRouter(t *testing.T, store platformadmin.EmailTemplateStore) *gin.Engine {
	t.Helper()
	db := testdb.NewDB(t, "idempotency_keys")

	gin.SetMode(gin.TestMode)
	r := gin.New()
	g := r.Group("", func(c *gin.Context) {
		c.Set(platformadmin.CtxOperatorID, testOperatorID)
		c.Set(platformadmin.CtxCapability, "platform.email_templates.write")
	})
	platformadmin.NewEmailTemplatesHandler(store, newStubRegistry(nil), nil, true, db, nil).Register(g)
	return r
}

func putWithKey(r *gin.Engine, idemKey string) *httptest.ResponseRecorder {
	req := httptest.NewRequest(http.MethodPut,
		"/admin/email-templates/orderdoc_invoice", strings.NewReader(validPut))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Idempotency-Key", idemKey)
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, req)
	return rec
}

func TestUpsertIsIdempotentPerKey(t *testing.T) {
	store := newStubTemplateStore(fixtureRow("orderdoc_invoice", emailtemplates.StatusPublished))
	r := putRouter(t, store)

	first := putWithKey(r, "key-1")
	require.Equal(t, http.StatusOK, first.Code, first.Body.String())
	firstVersion := decodeData(t, first)["version"]

	replay := putWithKey(r, "key-1")
	require.Equal(t, http.StatusOK, replay.Code)

	assert.JSONEq(t, first.Body.String(), replay.Body.String(),
		"a retry must replay the stored body verbatim")
	assert.Equal(t, firstVersion, decodeData(t, replay)["version"],
		"the version must NOT bump on a replay — that is the whole defect")

	// A different key is a genuinely new write, and does bump.
	fresh := putWithKey(r, "key-2")
	require.Equal(t, http.StatusOK, fresh.Code)
	assert.NotEqual(t, firstVersion, decodeData(t, fresh)["version"],
		"a new key is a new write, not a replay")
}

// The stored response must be the exact bytes the first caller received —
// not a re-render, which could drift as the row changes underneath.
func TestReplayReturnsTheStoredBytesNotAFreshRead(t *testing.T) {
	store := newStubTemplateStore(fixtureRow("orderdoc_invoice", emailtemplates.StatusPublished))
	r := putRouter(t, store)

	first := putWithKey(r, "key-1")
	require.Equal(t, http.StatusOK, first.Code)

	// Move the row on underneath, the way another operator's edit would.
	other := putWithKey(r, "key-other")
	require.Equal(t, http.StatusOK, other.Code)

	replay := putWithKey(r, "key-1")
	assert.JSONEq(t, first.Body.String(), replay.Body.String(),
		"the replay answers the original request, not the current row")

	var body map[string]any
	require.NoError(t, json.Unmarshal(replay.Body.Bytes(), &body))
	assert.Contains(t, replay.Header().Get("Content-Type"), "application/json")
}

// A key scoped to one template must not replay against another. Without
// namespacing, idempotency_keys.key is a bare service-wide primary key and
// the same header would silently skip the second template's write.
func TestKeysDoNotReplayAcrossTemplateKeys(t *testing.T) {
	store := newStubTemplateStore(
		fixtureRow("orderdoc_invoice", emailtemplates.StatusPublished),
		fixtureRow("giftcard_delivery", emailtemplates.StatusPublished),
	)
	r := putRouter(t, store)

	require.Equal(t, http.StatusOK, putWithKey(r, "shared-key").Code)

	req := httptest.NewRequest(http.MethodPut,
		"/admin/email-templates/giftcard_delivery", strings.NewReader(validPut))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Idempotency-Key", "shared-key")
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, req)

	require.Equal(t, http.StatusOK, rec.Code)
	assert.Equal(t, "giftcard_delivery", decodeData(t, rec)["key"],
		"the second template must be written, not answered with the first's response")
}
