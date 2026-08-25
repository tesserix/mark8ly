package platformadmin_test

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
	"time"
	"unicode/utf8"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/stretchr/testify/require"

	"github.com/mark8ly/marketplace-api/internal/audit"
	"github.com/mark8ly/marketplace-api/internal/handlers/platformadmin"
	"github.com/mark8ly/marketplace-api/internal/tenantdirectory"
	"github.com/mark8ly/marketplace-api/internal/tenantlifecycle"
	"github.com/mark8ly/marketplace-api/internal/tenantpurge"
)

// tenantID is a fixed, valid UUID used across this file's fixtures, so the
// golden fixture and the assertions cannot drift apart.
const tenantID = "55555555-5555-5555-5555-555555555555"

// Fakes record a monotonic sequence number so ORDER is assertable. Order
// is the whole design here: an audit row written before the purge is
// deleted BY the purge (purgePlan contains DELETE FROM audit_logs WHERE
// tenant_id = ?), so "all three were called" is not the property.
type seq struct{ n int }

func (s *seq) next() int { s.n++; return s.n }

type fakeTeardown struct {
	seq      *seq
	at       int
	gotSlugs []string
	res      *tenantlifecycle.TeardownResult
	err      error
}

func (f *fakeTeardown) Teardown(_ context.Context, _ string, slugs []string) (*tenantlifecycle.TeardownResult, error) {
	f.at, f.gotSlugs = f.seq.next(), slugs
	return f.res, f.err
}

type fakePurger struct {
	seq        *seq
	at         int
	gotTenant  string
	gotStores  []string
	countCalls int
	rep        tenantpurge.Report
	err        error
}

func (f *fakePurger) Purge(_ context.Context, tenantID string, storeIDs []string) (tenantpurge.Report, error) {
	f.at, f.gotTenant, f.gotStores = f.seq.next(), tenantID, storeIDs
	return f.rep, f.err
}

func (f *fakePurger) Count(_ context.Context, _ string, _ []string) (tenantpurge.Report, error) {
	f.countCalls++
	return f.rep, nil
}

// noopEmit is an explicit, visible "throw the event away" choice, used by
// tests that don't care about the audit side effect.
func noopEmit(*gin.Context, uuid.UUID, audit.Event) error { return nil }

// errUnavailableTestEmit is a canned failure a test double returns from
// its emit func, to prove the handler surfaces an audit failure rather
// than swallowing it.
var errUnavailableTestEmit = errors.New("audit backend unavailable (test)")

// jsonUnmarshal is a tiny alias so test bodies read consistently.
func jsonUnmarshal(data []byte, v any) error { return json.Unmarshal(data, v) }

// stripVolatileField removes the named field from the response's `data`
// object (e.g. a wall-clock timestamp) and returns the re-marshalled JSON,
// so a golden-fixture comparison isn't flaky against the current time.
func stripVolatileField(t *testing.T, body []byte, field string) string {
	t.Helper()
	var envelope struct {
		Data map[string]any `json:"data"`
	}
	require.NoError(t, json.Unmarshal(body, &envelope))
	_, present := envelope.Data[field]
	require.True(t, present, "expected volatile field %q to be present before stripping", field)
	delete(envelope.Data, field)
	out, err := json.Marshal(envelope)
	require.NoError(t, err)
	return string(out)
}

// purgeRouter builds a gin engine with the platform-auth middleware and
// the purge handler mounted, backed by td/pg/emit and an inert directory
// + invalidator unless overridden by the caller.
func purgeRouter(t *testing.T, td platformadmin.TenantTeardown, pg platformadmin.Purger, dir platformadmin.TenantDirectory, emit func(*gin.Context, uuid.UUID, audit.Event) error, inv platformadmin.TenantGateInvalidator) *gin.Engine {
	t.Helper()
	gin.SetMode(gin.TestMode)
	r := gin.New()
	r.Use(platformadmin.RequirePlatformAuth(platformadmin.AuthConfig{
		Secret:     testSecret,
		NonceStore: newMemNonces(),
		Now:        func() time.Time { return fixedNow },
	}))
	h := platformadmin.NewTenantPurgeHandler(td, pg, dir, emit, inv, nil)
	h.Register(r.Group(""))
	return r
}

// doPurge builds the gin engine with the three fakes and serves one POST
// to /admin/tenants/{tenantID}/purge with the given body.
func doPurge(t *testing.T, td platformadmin.TenantTeardown, pg platformadmin.Purger, emit func(*gin.Context, uuid.UUID, audit.Event) error, body string) *httptest.ResponseRecorder {
	t.Helper()
	return doPurgeTenant(t, td, pg, emit, tenantID, body)
}

func doPurgeTenant(t *testing.T, td platformadmin.TenantTeardown, pg platformadmin.Purger, emit func(*gin.Context, uuid.UUID, audit.Event) error, id, body string) *httptest.ResponseRecorder {
	t.Helper()
	r := purgeRouter(t, td, pg, &stubDirectory{}, emit, nil)
	target := "/admin/tenants/" + id + "/purge"
	req := signedRequest(t, http.MethodPost, target, []byte(body))
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, req)
	return rec
}

// doPreview serves one GET to /admin/tenants/{tenantID}/purge/preview.
func doPreview(t *testing.T, dir platformadmin.TenantDirectory, pg platformadmin.Purger) *httptest.ResponseRecorder {
	t.Helper()
	r := purgeRouter(t, &fakeTeardown{seq: &seq{}}, pg, dir, noopEmit, nil)
	target := "/admin/tenants/" + tenantID + "/purge/preview"
	req := signedRequest(t, http.MethodGet, target, nil)
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, req)
	return rec
}

func TestPurge_HappyPathTearsDownThenPurgesThenAudits(t *testing.T) {
	sq := &seq{}
	td := &fakeTeardown{seq: sq, res: &tenantlifecycle.TeardownResult{
		TenantID: tenantID, TenantName: "The Bondi Store",
		StoreIDs: []string{"s-1", "s-2"}, StoreSlugs: []string{"a", "b"},
	}}
	pg := &fakePurger{seq: sq, rep: tenantpurge.Report{
		Tables: []tenantpurge.TableResult{{Table: "products", RowsDeleted: 3}}, TotalRows: 3,
	}}
	var auditAt int
	emit := func(*gin.Context, uuid.UUID, audit.Event) error { auditAt = sq.next(); return nil }

	rec := doPurge(t, td, pg, emit, `{"store_slugs":["a","b"],"reason_code":"merchant_request"}`)

	require.Equal(t, http.StatusOK, rec.Code, rec.Body.String())
	require.Equal(t, 1, td.at, "teardown must run first")
	require.Equal(t, 2, pg.at, "purge must run after teardown")
	require.Equal(t, 3, auditAt, "the audit row must be written AFTER the purge that would delete it")
}

func TestPurge_PurgeIsScopedToTheStoreIDsTeardownReturned(t *testing.T) {
	sq := &seq{}
	// TWO store ids. One could not distinguish "passes them through" from
	// "passes the first" or "passes an empty slice".
	td := &fakeTeardown{seq: sq, res: &tenantlifecycle.TeardownResult{
		TenantID: tenantID, StoreIDs: []string{"s-1", "s-2"}, StoreSlugs: []string{"a", "b"},
	}}
	pg := &fakePurger{seq: sq}

	doPurge(t, td, pg, noopEmit, `{"store_slugs":["a","b"],"reason_code":"merchant_request"}`)

	require.Equal(t, tenantID, pg.gotTenant)
	require.Equal(t, []string{"s-1", "s-2"}, pg.gotStores)
}

func TestPurge_ReasonIsCappedByRunesNotBytes(t *testing.T) {
	sq := &seq{}
	td := &fakeTeardown{seq: sq, res: &tenantlifecycle.TeardownResult{TenantID: tenantID, StoreIDs: []string{}, StoreSlugs: []string{}}}
	pg := &fakePurger{seq: sq}
	var got audit.Event
	emit := func(_ *gin.Context, _ uuid.UUID, ev audit.Event) error { got = ev; return nil }

	// 600 two-byte runes: a 500-BYTE cut lands mid-rune and yields invalid
	// UTF-8, which Postgres rejects on the jsonb write, which fails the
	// audit emit — an irreversible destruction recorded nowhere.
	long := strings.Repeat("é", 600)

	rec := doPurge(t, td, pg, emit, `{"store_slugs":[],"reason_code":"merchant_request","reason":"`+long+`"}`)
	require.Equal(t, http.StatusOK, rec.Code, rec.Body.String())

	stored, _ := got.Metadata["reason"].(string)
	require.Equal(t, 500, utf8.RuneCountInString(stored), "cap counts runes")
	require.True(t, utf8.ValidString(stored), "a byte-truncated multibyte string is invalid UTF-8")
	require.Less(t, len(stored), 1200)
}

// The full report — table names AND counts — is passed through, not
// summarised or dropped.
func TestPurge_ResponseCarriesTheReport(t *testing.T) {
	sq := &seq{}
	td := &fakeTeardown{seq: sq, res: &tenantlifecycle.TeardownResult{
		TenantID: tenantID, TenantName: "Bondi", StoreIDs: []string{"s-1"}, StoreSlugs: []string{"a"},
	}}
	pg := &fakePurger{seq: sq, rep: tenantpurge.Report{
		Tables: []tenantpurge.TableResult{
			{Table: "products", RowsDeleted: 7},
			{Table: "orders", RowsDeleted: 4},
			{Table: "carts", RowsDeleted: 0},
		},
		TotalRows: 11,
	}}

	rec := doPurge(t, td, pg, noopEmit, `{"store_slugs":["a"],"reason_code":"merchant_request"}`)
	require.Equal(t, http.StatusOK, rec.Code, rec.Body.String())

	var body struct {
		Data struct {
			Tables []struct {
				Table string `json:"table"`
				Rows  int64  `json:"rows"`
			} `json:"tables"`
			TotalRows int64 `json:"total_rows"`
		} `json:"data"`
	}
	require.NoError(t, jsonUnmarshal(rec.Body.Bytes(), &body))
	require.Equal(t, int64(11), body.Data.TotalRows)
	require.Len(t, body.Data.Tables, 3)
	require.Equal(t, "products", body.Data.Tables[0].Table)
	require.Equal(t, int64(7), body.Data.Tables[0].Rows)
	require.Equal(t, "orders", body.Data.Tables[1].Table)
	require.Equal(t, int64(4), body.Data.Tables[1].Rows)
	require.Equal(t, "carts", body.Data.Tables[2].Table)
	require.Equal(t, int64(0), body.Data.Tables[2].Rows)
}

func TestPurge_MismatchIs409WithExpected(t *testing.T) {
	sq := &seq{}
	td := &fakeTeardown{seq: sq, err: &tenantlifecycle.ConfirmationMismatchError{Expected: []string{"a", "b"}}}
	pg := &fakePurger{seq: sq}

	rec := doPurge(t, td, pg, noopEmit, `{"store_slugs":["a"],"reason_code":"merchant_request"}`)

	require.Equal(t, http.StatusConflict, rec.Code)
	require.Contains(t, rec.Body.String(), `"error":"confirmation_mismatch"`)
	require.Contains(t, rec.Body.String(), `"expected":["a","b"]`)
	require.Zero(t, pg.at, "the purger must never be called on a refusal")
}

func TestPurge_NotFoundIs404AndPurgesNothing(t *testing.T) {
	sq := &seq{}
	td := &fakeTeardown{seq: sq, err: tenantlifecycle.ErrNotFound}
	pg := &fakePurger{seq: sq}

	rec := doPurge(t, td, pg, noopEmit, `{"store_slugs":["a"],"reason_code":"merchant_request"}`)

	require.Equal(t, http.StatusNotFound, rec.Code)
	require.Zero(t, pg.at, "the purger must never be called on a refusal")
}

func TestPurge_UnavailableIs503AndPurgesNothing(t *testing.T) {
	sq := &seq{}
	td := &fakeTeardown{seq: sq, err: tenantlifecycle.ErrUnavailable}
	pg := &fakePurger{seq: sq}

	rec := doPurge(t, td, pg, noopEmit, `{"store_slugs":["a"],"reason_code":"merchant_request"}`)

	require.Equal(t, http.StatusServiceUnavailable, rec.Code)
	require.Contains(t, rec.Body.String(), "upstream_unavailable")
	require.Zero(t, pg.at, "an unreachable upstream must never read as 'nothing to do' and must never purge")
}

// An unmounted upstream route must reach the operator as 503, NOT as the
// 404 this API defines as "tenant not found, including already purged".
// See tenantlifecycle.ErrUpstreamRouteMissing.
func TestPurge_UpstreamRouteMissingIs503NotAlreadyPurged(t *testing.T) {
	sq := &seq{}
	td := &fakeTeardown{seq: sq, err: tenantlifecycle.ErrUpstreamRouteMissing}
	pg := &fakePurger{seq: sq}

	rec := doPurge(t, td, pg, noopEmit, `{"store_slugs":["a"],"reason_code":"erasure_request"}`)

	require.Equal(t, http.StatusServiceUnavailable, rec.Code, rec.Body.String())
	require.Contains(t, rec.Body.String(), "upstream_unavailable")
	require.NotContains(t, rec.Body.String(), "tenant_not_found",
		"a missing ROUTE must never be reported as a missing TENANT — an operator working an erasure would close the ticket on a live tenant")
	require.Zero(t, pg.at, "the purger must never be called on a refusal")
}

// `purge_incomplete` — the teardown committed upstream but the local purge
// failed — had NO test at all, and the audit row it wrote said the purge
// SUCCEEDED: Status was left at its zero value, which the emitter defaults
// to StatusSuccess, with total_rows: 0 from the empty report.
//
// The operator sees 500. The permanent record says success. On the one
// endpoint whose entire purpose is producing a trustworthy record of an
// irreversible act, those must agree.
func TestPurge_LocalPurgeFailureIsAuditedAsFailureWithTheError(t *testing.T) {
	sq := &seq{}
	td := &fakeTeardown{seq: sq, res: &tenantlifecycle.TeardownResult{
		TenantID: tenantID, TenantName: "The Bondi Store",
		StoreIDs: []string{"33333333-3333-3333-3333-333333333333"}, StoreSlugs: []string{"bondi"},
	}}
	purgeFailure := errors.New("tenantpurge: delete from orders: connection reset")
	pg := &fakePurger{seq: sq, err: purgeFailure}

	var got audit.Event
	emits := 0
	emit := func(_ *gin.Context, _ uuid.UUID, ev audit.Event) error {
		emits++
		got = ev
		return nil
	}

	rec := doPurge(t, td, pg, emit, `{"store_slugs":["bondi"],"reason_code":"erasure_request"}`)

	require.Equal(t, http.StatusInternalServerError, rec.Code, rec.Body.String())
	require.Contains(t, rec.Body.String(), "purge_incomplete")

	// The event is still written: the tenant row is already gone upstream,
	// so "no record at all" is the worse outcome.
	require.Equal(t, 1, emits, "a failed local purge must still be audited — the teardown already committed")
	require.Equal(t, "tenant.purged", got.Action)
	require.Equal(t, audit.StatusFailure, got.Status,
		"the operator was told 500; the audit row must not say the purge succeeded")
	require.Equal(t, audit.SeverityCritical, got.Severity)
	require.Equal(t, purgeFailure.Error(), got.Metadata["purge_error"],
		"the failure that produced the 500 must be recorded, not just its existence implied by the status")
	require.Equal(t, "erasure_request", got.Metadata["reason_code"])
}

// The success path must NOT be stamped failure — otherwise the assertion
// above would pass against a handler that marked every purge failed.
func TestPurge_SuccessIsAuditedAsSuccessWithNoPurgeError(t *testing.T) {
	sq := &seq{}
	td := &fakeTeardown{seq: sq, res: &tenantlifecycle.TeardownResult{
		TenantID: tenantID, TenantName: "The Bondi Store",
		StoreIDs: []string{"33333333-3333-3333-3333-333333333333"}, StoreSlugs: []string{"bondi"},
	}}
	pg := &fakePurger{seq: sq, rep: tenantpurge.Report{
		Tables:    []tenantpurge.TableResult{{Table: "products", RowsDeleted: 2}},
		TotalRows: 2,
	}}

	var got audit.Event
	emit := func(_ *gin.Context, _ uuid.UUID, ev audit.Event) error { got = ev; return nil }

	rec := doPurge(t, td, pg, emit, `{"store_slugs":["bondi"],"reason_code":"erasure_request"}`)

	require.Equal(t, http.StatusOK, rec.Code, rec.Body.String())
	require.Equal(t, audit.StatusSuccess, got.Status)
	require.NotContains(t, got.Metadata, "purge_error")
	require.EqualValues(t, 2, got.Metadata["total_rows"])
}

func TestPurge_UnknownReasonCodeIs400(t *testing.T) {
	cases := []struct {
		code   string
		wantOK bool
	}{
		{"", false},
		{"nonsense", false},
		{"merchant_request", true}, // a valid code, in the SAME test, so a validator that refuses everything fails too
	}
	for _, tc := range cases {
		sq := &seq{}
		td := &fakeTeardown{seq: sq, res: &tenantlifecycle.TeardownResult{TenantID: tenantID, StoreIDs: []string{}, StoreSlugs: []string{}}}
		pg := &fakePurger{seq: sq}
		rec := doPurge(t, td, pg, noopEmit, `{"store_slugs":[],"reason_code":"`+tc.code+`"}`)
		if tc.wantOK {
			require.Equal(t, http.StatusOK, rec.Code, "code %q must be accepted", tc.code)
		} else {
			require.Equal(t, http.StatusBadRequest, rec.Code, "code %q must be rejected", tc.code)
			require.Contains(t, rec.Body.String(), "reason_code")
		}
	}
}

func TestPurge_EveryDeclaredReasonCodeIsAccepted(t *testing.T) {
	for _, code := range platformadmin.PurgeReasonCodes {
		sq := &seq{}
		td := &fakeTeardown{seq: sq, res: &tenantlifecycle.TeardownResult{TenantID: tenantID, StoreIDs: []string{}, StoreSlugs: []string{}}}
		pg := &fakePurger{seq: sq}
		rec := doPurge(t, td, pg, noopEmit, `{"store_slugs":[],"reason_code":"`+code+`"}`)
		require.Equal(t, http.StatusOK, rec.Code, "declared code %q must be accepted", code)
	}
}

func TestPurge_AbsentStoreSlugsIs400_EmptyIsForwarded(t *testing.T) {
	sq := &seq{}
	td := &fakeTeardown{seq: sq, res: &tenantlifecycle.TeardownResult{TenantID: tenantID, StoreIDs: []string{}, StoreSlugs: []string{}}}
	pg := &fakePurger{seq: sq}

	rec := doPurge(t, td, pg, noopEmit, `{"reason_code":"merchant_request"}`)
	require.Equal(t, http.StatusBadRequest, rec.Code, "absent store_slugs must be rejected")
	require.Zero(t, td.at, "teardown must not be called when store_slugs is absent")

	sq2 := &seq{}
	td2 := &fakeTeardown{seq: sq2, res: &tenantlifecycle.TeardownResult{TenantID: tenantID, StoreIDs: []string{}, StoreSlugs: []string{}}}
	pg2 := &fakePurger{seq: sq2}
	rec2 := doPurge(t, td2, pg2, noopEmit, `{"store_slugs":[],"reason_code":"merchant_request"}`)
	require.Equal(t, http.StatusOK, rec2.Code, rec2.Body.String())
	require.NotNil(t, td2.gotSlugs, "empty store_slugs must be forwarded as a non-nil empty slice")
	require.Empty(t, td2.gotSlugs)
}

func TestPurge_AuditFailureIsSurfaced(t *testing.T) {
	sq := &seq{}
	td := &fakeTeardown{seq: sq, res: &tenantlifecycle.TeardownResult{TenantID: tenantID, StoreIDs: []string{}, StoreSlugs: []string{}}}
	pg := &fakePurger{seq: sq}
	emit := func(*gin.Context, uuid.UUID, audit.Event) error { return errUnavailableTestEmit }

	rec := doPurge(t, td, pg, emit, `{"store_slugs":[],"reason_code":"merchant_request"}`)
	require.NotEqual(t, http.StatusOK, rec.Code, "an audit failure must not read as a plain 200")
	require.Contains(t, rec.Body.String(), "purge_unaudited")
}

func TestPurge_AuditRowCarriesOperatorCapabilityReasonAndCounts(t *testing.T) {
	sq := &seq{}
	td := &fakeTeardown{seq: sq, res: &tenantlifecycle.TeardownResult{
		TenantID: tenantID, TenantName: "Bondi", StoreIDs: []string{"s-1", "s-2"}, StoreSlugs: []string{"a", "b"},
	}}
	pg := &fakePurger{seq: sq, rep: tenantpurge.Report{
		Tables: []tenantpurge.TableResult{{Table: "products", RowsDeleted: 9}}, TotalRows: 9,
	}}
	var got audit.Event
	emit := func(_ *gin.Context, id uuid.UUID, ev audit.Event) error {
		got = ev
		ev.TenantID = id
		got = ev
		return nil
	}

	rec := doPurge(t, td, pg, emit, `{"store_slugs":["a","b"],"reason_code":"fraud","reason":"confirmed fraud"}`)
	require.Equal(t, http.StatusOK, rec.Code, rec.Body.String())

	require.Equal(t, "tenant.purged", got.Action)
	require.Equal(t, "tenant", got.ResourceType)
	require.Equal(t, audit.SeverityCritical, got.Severity)
	require.Equal(t, "fraud", got.Metadata["reason_code"])
	require.Equal(t, "confirmed fraud", got.Metadata["reason"])
	require.Equal(t, []string{"a", "b"}, got.Metadata["store_slugs"])
	require.Equal(t, []string{"s-1", "s-2"}, got.Metadata["store_ids"])
	require.Equal(t, int64(9), got.Metadata["total_rows"])
	require.Equal(t, "audit.read", got.Metadata["capability"])
}

func TestPurge_InvalidTenantIDIs400(t *testing.T) {
	sq := &seq{}
	td := &fakeTeardown{seq: sq, res: &tenantlifecycle.TeardownResult{TenantID: tenantID, StoreIDs: []string{}, StoreSlugs: []string{}}}
	pg := &fakePurger{seq: sq}

	rec := doPurgeTenant(t, td, pg, noopEmit, "not-a-uuid", `{"store_slugs":[],"reason_code":"merchant_request"}`)
	require.Equal(t, http.StatusBadRequest, rec.Code)
	require.Contains(t, rec.Body.String(), "invalid_tenant_id")
	require.Zero(t, td.at, "teardown must not be called for an invalid tenant id")
}

func TestPurge_UnparseableBodyIs400(t *testing.T) {
	sq := &seq{}
	td := &fakeTeardown{seq: sq, res: &tenantlifecycle.TeardownResult{TenantID: tenantID, StoreIDs: []string{}, StoreSlugs: []string{}}}
	pg := &fakePurger{seq: sq}

	rec := doPurge(t, td, pg, noopEmit, `{`)
	require.Equal(t, http.StatusBadRequest, rec.Code)
	require.Contains(t, rec.Body.String(), "invalid_request")
	require.Zero(t, td.at, "teardown must never be called for an unparseable body")
}

type fakeInvalidator struct {
	calls []string
}

func (f *fakeInvalidator) Invalidate(tenantID string) { f.calls = append(f.calls, tenantID) }

func TestPurge_GateIsInvalidatedAfterASuccessfulPurge(t *testing.T) {
	sq := &seq{}
	td := &fakeTeardown{seq: sq, res: &tenantlifecycle.TeardownResult{TenantID: tenantID, StoreIDs: []string{}, StoreSlugs: []string{}}}
	pg := &fakePurger{seq: sq}
	inv := &fakeInvalidator{}
	r := purgeRouter(t, td, pg, &stubDirectory{}, noopEmit, inv)
	target := "/admin/tenants/" + tenantID + "/purge"
	req := signedRequest(t, http.MethodPost, target, []byte(`{"store_slugs":[],"reason_code":"merchant_request"}`))
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, req)

	require.Equal(t, http.StatusOK, rec.Code, rec.Body.String())
	require.Equal(t, []string{tenantID}, inv.calls, "the gate must be invalidated after a successful purge")
}

func TestPurge_GateIsNotInvalidatedOnMismatch(t *testing.T) {
	sq := &seq{}
	td := &fakeTeardown{seq: sq, err: &tenantlifecycle.ConfirmationMismatchError{Expected: []string{"a"}}}
	pg := &fakePurger{seq: sq}
	inv := &fakeInvalidator{}
	r := purgeRouter(t, td, pg, &stubDirectory{}, noopEmit, inv)
	target := "/admin/tenants/" + tenantID + "/purge"
	req := signedRequest(t, http.MethodPost, target, []byte(`{"store_slugs":["a"],"reason_code":"merchant_request"}`))
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, req)

	require.Equal(t, http.StatusConflict, rec.Code)
	require.Empty(t, inv.calls, "a 409 must never invalidate the gate")
}

// countRecordingPurger is a Purger whose Count records the tenant and
// store ids it was called with, so the preview route's scoping can be
// asserted directly. fakePurger's own Count (verbatim per the brief)
// deliberately does not record these.
type countRecordingPurger struct {
	rep        tenantpurge.Report
	gotTenant  string
	gotStores  []string
	purgeCalls int
}

func (p *countRecordingPurger) Purge(context.Context, string, []string) (tenantpurge.Report, error) {
	p.purgeCalls++
	return tenantpurge.Report{}, nil
}

func (p *countRecordingPurger) Count(_ context.Context, tenantID string, storeIDs []string) (tenantpurge.Report, error) {
	p.gotTenant, p.gotStores = tenantID, storeIDs
	return p.rep, nil
}

func TestPurgePreview_ReturnsSlugsAndCounts(t *testing.T) {
	dir := &stubDirectory{detail: &tenantdirectory.TenantDetail{
		Tenant: tenantdirectory.Tenant{ID: tenantID, Name: "Bondi", Status: "active"},
		Stores: []tenantdirectory.StoreSummary{
			{ID: "s-1", Slug: "bondi-main", Status: "active"},
			{ID: "s-2", Slug: "bondi-pop-up", Status: "active"},
		},
	}}
	pg := &countRecordingPurger{rep: tenantpurge.Report{
		Tables:    []tenantpurge.TableResult{{Table: "products", RowsDeleted: 12}},
		TotalRows: 12,
	}}

	rec := doPreview(t, dir, pg)
	require.Equal(t, http.StatusOK, rec.Code, rec.Body.String())

	var body struct {
		Data struct {
			TenantID   string   `json:"tenant_id"`
			TenantName string   `json:"tenant_name"`
			Status     string   `json:"status"`
			StoreSlugs []string `json:"store_slugs"`
			TotalRows  int64    `json:"total_rows"`
		} `json:"data"`
	}
	require.NoError(t, jsonUnmarshal(rec.Body.Bytes(), &body))
	require.Equal(t, tenantID, body.Data.TenantID)
	require.Equal(t, "Bondi", body.Data.TenantName)
	require.Equal(t, "active", body.Data.Status)
	require.Equal(t, []string{"bondi-main", "bondi-pop-up"}, body.Data.StoreSlugs)
	require.Equal(t, int64(12), body.Data.TotalRows)
	require.Equal(t, tenantID, pg.gotTenant)
	require.Equal(t, []string{"s-1", "s-2"}, pg.gotStores, "Count must be scoped by the directory's own store ids")
	require.Zero(t, pg.purgeCalls, "preview must never call Purge")
}

func TestPurgePreview_DestroysNothing(t *testing.T) {
	dir := &stubDirectory{detail: &tenantdirectory.TenantDetail{
		Tenant: tenantdirectory.Tenant{ID: tenantID, Name: "Bondi", Status: "active"},
		Stores: []tenantdirectory.StoreSummary{{ID: "s-1", Slug: "bondi-main", Status: "active"}},
	}}
	pg := &fakePurger{seq: &seq{}, rep: tenantpurge.Report{TotalRows: 3}}

	rec := doPreview(t, dir, pg)
	require.Equal(t, http.StatusOK, rec.Code, rec.Body.String())
	require.Zero(t, pg.at, "preview must never call Purge")
	require.Equal(t, 1, pg.countCalls, "preview must call Count exactly once")
}

func TestPurge_MatchesGoldenFixture(t *testing.T) {
	sq := &seq{}
	td := &fakeTeardown{seq: sq, res: &tenantlifecycle.TeardownResult{
		TenantID: tenantID, TenantName: "The Bondi Store",
		StoreIDs: []string{"s-1", "s-2"}, StoreSlugs: []string{"bondi-main", "bondi-pop-up"},
	}}
	pg := &fakePurger{seq: sq, rep: tenantpurge.Report{
		Tables:    []tenantpurge.TableResult{{Table: "products", RowsDeleted: 7}, {Table: "orders", RowsDeleted: 4}},
		TotalRows: 11,
	}}

	rec := doPurge(t, td, pg, noopEmit, `{"store_slugs":["bondi-main","bondi-pop-up"],"reason_code":"merchant_request","reason":"merchant closed shop"}`)
	require.Equal(t, http.StatusOK, rec.Code, rec.Body.String())

	got := stripVolatileField(t, rec.Body.Bytes(), "purged_at")
	want, err := os.ReadFile("testdata/tenant_purge.golden.json")
	require.NoError(t, err)
	require.JSONEq(t, string(want), got)
}

func TestPurgePreview_MatchesGoldenFixture(t *testing.T) {
	dir := &stubDirectory{detail: &tenantdirectory.TenantDetail{
		Tenant: tenantdirectory.Tenant{ID: tenantID, Name: "The Bondi Store", Status: "active"},
		Stores: []tenantdirectory.StoreSummary{
			{ID: "s-1", Slug: "bondi-main", Status: "active"},
			{ID: "s-2", Slug: "bondi-pop-up", Status: "active"},
		},
	}}
	pg := &fakePurger{seq: &seq{}, rep: tenantpurge.Report{
		Tables:    []tenantpurge.TableResult{{Table: "products", RowsDeleted: 7}, {Table: "orders", RowsDeleted: 4}},
		TotalRows: 11,
	}}

	rec := doPreview(t, dir, pg)
	require.Equal(t, http.StatusOK, rec.Code, rec.Body.String())

	want, err := os.ReadFile("testdata/tenant_purge_preview.golden.json")
	require.NoError(t, err)
	require.JSONEq(t, string(want), rec.Body.String())
}
