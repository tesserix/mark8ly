//go:build integration

package platformadmin_test

import (
	"net/http"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/mark8ly/platform-api/internal/emailtemplates"
	"github.com/mark8ly/platform-api/pkg/testdb"
)

// TestUpsertReplaysIdempotentRetryFromDB proves the PUT actually honours
// its Idempotency-Key against a real database — the bug mark8ly#730 fixed
// on marketplace-api's identical route was that this header was accepted
// and then silently ignored, so a client retry re-ran the UPSERT and
// bumped the version a second time. This is written directly against the
// FIXED behaviour and exercises the real internal/idempotency package
// through pkg/testdb, not a stub: a stub store's own Reserve/Lookup could
// not have caught that class of bug in the first place, since the bug was
// in whether the handler called the real reservation machinery at all.
//
// Build-tagged `integration` and excluded from the default `go test ./...`
// run, matching every other DB-backed test in this service — the shared
// test database is not safe for concurrent or ad-hoc runs.
func TestUpsertReplaysIdempotentRetryFromDB(t *testing.T) {
	db := testdb.NewTx(t)
	store := newStubTemplateStore(fixtureRow("login_otp", emailtemplates.StatusPublished))
	r := templateRouter(store, newStubRegistry(nil), nil, true, db)

	first := do(t, r, http.MethodPut, "/admin/email-templates/login_otp", validBody)
	require.Equal(t, http.StatusOK, first.Code, first.Body.String())
	firstData := decodeData(t, first)
	require.Equal(t, float64(4), firstData["version"])

	// Same Idempotency-Key (do() derives it from method+path, which is
	// identical on this retry) must replay the FIRST response verbatim —
	// no second UPSERT, no second version bump.
	second := do(t, r, http.MethodPut, "/admin/email-templates/login_otp", validBody)
	require.Equal(t, http.StatusOK, second.Code, second.Body.String())
	require.JSONEq(t, first.Body.String(), second.Body.String(),
		"a retried PUT with the same Idempotency-Key must replay the stored response, not re-run the write")
}
