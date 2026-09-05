package platformadmin_test

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/mark8ly/marketplace-api/internal/emailtemplates"
)

// #730. tesserix-home's platform-api sends an Idempotency-Key on every
// federated write — federation.Client refuses to make one without it
// (client.go:203, ErrIdempotencyKeyRequired) — and this handler ignored it.
//
// A template upsert is NEARLY idempotent by shape: the same body applied
// twice yields the same subject, bodies and status. But `version` bumps on
// every UPSERT and a revision row is appended per change, so a retried write
// inflates the counter and records a change nobody made. That revision trail
// is what stands in for an audit record on this surface, which is why a
// duplicate matters more here than the identical-looking row suggests.

func TestUpsertRequiresAnIdempotencyKey(t *testing.T) {
	store := newStubTemplateStore(fixtureRow("orderdoc_invoice", emailtemplates.StatusPublished))
	rec := doWithoutIdempotencyKey(t,
		templateRouter(store, newStubRegistry(nil), nil, true),
		http.MethodPut, "/admin/email-templates/orderdoc_invoice", validPut)

	require.Equal(t, http.StatusBadRequest, rec.Code,
		"a write that cannot be retried safely must refuse to start")
	require.Equal(t, "idempotency_key_required", errorCode(t, rec))
	require.Nil(t, store.gotUpsert, "nothing may be written without a key")
}

// The requirement is checked BEFORE the body is parsed, so a caller missing
// the header gets that one clear reason rather than a validation error that
// sends them fixing the wrong thing.
func TestMissingIdempotencyKeyIsReportedBeforeBodyValidation(t *testing.T) {
	store := newStubTemplateStore(fixtureRow("orderdoc_invoice", emailtemplates.StatusPublished))
	rec := doWithoutIdempotencyKey(t,
		templateRouter(store, newStubRegistry(nil), nil, true),
		http.MethodPut, "/admin/email-templates/orderdoc_invoice", `{"status":"nonsense"}`)

	require.Equal(t, http.StatusBadRequest, rec.Code)
	require.Equal(t, "idempotency_key_required", errorCode(t, rec))
}

// A whitespace-only header is not a key. Accepting one would scope every
// caller's replay to the same blank string.
func TestBlankIdempotencyKeyIsRejected(t *testing.T) {
	store := newStubTemplateStore(fixtureRow("orderdoc_invoice", emailtemplates.StatusPublished))
	r := templateRouter(store, newStubRegistry(nil), nil, true)

	req := httptest.NewRequest(http.MethodPut,
		"/admin/email-templates/orderdoc_invoice", strings.NewReader(validPut))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Idempotency-Key", "   ")
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, req)

	require.Equal(t, http.StatusBadRequest, rec.Code)
	require.Equal(t, "idempotency_key_required", errorCode(t, rec))
}

// Without a database the handler cannot record or replay a key, and it must
// still serve the write rather than refusing it — mirroring how
// BillingTrialExtendHandler treats a nil db. This is the unit-test
// configuration; replay itself needs a real table and is proven in the
// integration test.
//
// Production never reaches this branch: routes.go mounts the PUT only when
// deps.DB is non-nil, and passes that same handle here.
func TestUpsertWithoutADatabaseStillWrites(t *testing.T) {
	store := newStubTemplateStore(fixtureRow("orderdoc_invoice", emailtemplates.StatusPublished))
	rec := do(t, templateRouter(store, newStubRegistry(nil), nil, true),
		http.MethodPut, "/admin/email-templates/orderdoc_invoice", validPut)

	require.Equal(t, http.StatusOK, rec.Code, rec.Body.String())
	assert.NotNil(t, store.gotUpsert, "the write must still happen")
}
