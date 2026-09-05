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

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"

	"github.com/mark8ly/marketplace-api/internal/emailtemplates"
	"github.com/mark8ly/marketplace-api/internal/handlers/platformadmin"
)

// ─── doubles ────────────────────────────────────────────────────────────

type stubTemplateStore struct {
	rows      map[string]emailtemplates.Row
	listErr   error
	getErr    error
	upsertErr error
	gotUpsert *emailtemplates.UpsertInput
}

func newStubTemplateStore(rows ...emailtemplates.Row) *stubTemplateStore {
	s := &stubTemplateStore{rows: map[string]emailtemplates.Row{}}
	for _, r := range rows {
		s.rows[r.Key] = r
	}
	return s
}

func (s *stubTemplateStore) List(context.Context) ([]emailtemplates.Row, error) {
	if s.listErr != nil {
		return nil, s.listErr
	}
	// Sorted by the handler, so the order here is deliberately arbitrary.
	out := make([]emailtemplates.Row, 0, len(s.rows))
	for _, r := range s.rows {
		out = append(out, r)
	}
	return out, nil
}

func (s *stubTemplateStore) Get(_ context.Context, key string) (emailtemplates.Row, bool, error) {
	if s.getErr != nil {
		return emailtemplates.Row{}, false, s.getErr
	}
	r, ok := s.rows[key]
	return r, ok, nil
}

func (s *stubTemplateStore) Upsert(_ context.Context, in emailtemplates.UpsertInput) (emailtemplates.Row, error) {
	s.gotUpsert = &in
	if s.upsertErr != nil {
		return emailtemplates.Row{}, s.upsertErr
	}
	prev := s.rows[in.Key]
	saved := emailtemplates.Row{
		Key:       in.Key,
		Subject:   in.Subject,
		HTMLBody:  in.HTMLBody,
		TextBody:  in.TextBody,
		Variables: in.Variables,
		Status:    in.Status,
		Version:   prev.Version + 1,
		UpdatedAt: time.Date(2026, 9, 5, 12, 0, 0, 0, time.UTC),
		UpdatedBy: in.UpdatedBy,
	}
	s.rows[in.Key] = saved
	return saved, nil
}

// stubTemplateRegistry is a real *emailtemplates.Loader in every test that
// can use one — the point is to exercise the production accessors, not a
// reimplementation of them. It only records the Invalidate calls the
// Loader would otherwise swallow.
type stubTemplateRegistry struct {
	loader      *emailtemplates.Loader
	invalidated []string
	renderErr   error
}

func newStubRegistry(fallbacks map[string]emailtemplates.EmbeddedFallback) *stubTemplateRegistry {
	l := emailtemplates.NewLoader(nil)
	for k, fb := range fallbacks {
		l.Register(k, fb)
	}
	return &stubTemplateRegistry{loader: l}
}

func (s *stubTemplateRegistry) RegisteredKeys() []string { return s.loader.RegisteredKeys() }

func (s *stubTemplateRegistry) Fallback(key string) (emailtemplates.EmbeddedFallback, bool) {
	return s.loader.Fallback(key)
}

func (s *stubTemplateRegistry) Invalidate(key string) {
	s.invalidated = append(s.invalidated, key)
	s.loader.Invalidate(key)
}

func (s *stubTemplateRegistry) Render(ctx context.Context, key string, vars any) (emailtemplates.Rendered, error) {
	if s.renderErr != nil {
		return emailtemplates.Rendered{}, s.renderErr
	}
	return s.loader.Render(ctx, key, vars)
}

type stubTestSender struct {
	gotTo       string
	gotRendered emailtemplates.Rendered
	err         error
}

func (s *stubTestSender) SendTest(_ context.Context, to string, r emailtemplates.Rendered) error {
	s.gotTo, s.gotRendered = to, r
	return s.err
}

// ─── harness ────────────────────────────────────────────────────────────

const testOperatorID = "op_11111111"

// templateRouter mounts the handler behind a middleware that sets the same
// context keys RequirePlatformAuth sets. The operator id is what
// updated_by is stamped from, so a harness that omitted it would let a
// bug that reads the body instead pass unnoticed.
func templateRouter(
	store platformadmin.EmailTemplateStore,
	registry platformadmin.EmailTemplateRegistry,
	sender emailtemplates.TestSender,
	writable bool,
) *gin.Engine {
	gin.SetMode(gin.TestMode)
	r := gin.New()
	g := r.Group("", func(c *gin.Context) {
		c.Set(platformadmin.CtxOperatorID, testOperatorID)
		c.Set(platformadmin.CtxCapability, "platform.email_templates.write")
	})
	platformadmin.NewEmailTemplatesHandler(store, registry, sender, writable, nil).Register(g)
	return r
}

func do(t *testing.T, r *gin.Engine, method, path, body string) *httptest.ResponseRecorder {
	t.Helper()
	var req *http.Request
	if body == "" {
		req = httptest.NewRequest(method, path, nil)
	} else {
		req = httptest.NewRequest(method, path, strings.NewReader(body))
		req.Header.Set("Content-Type", "application/json")
	}
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, req)
	return rec
}

func decodeData(t *testing.T, rec *httptest.ResponseRecorder) map[string]any {
	t.Helper()
	var body struct {
		Data map[string]any `json:"data"`
	}
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &body))
	return body.Data
}

func decodeRows(t *testing.T, rec *httptest.ResponseRecorder) []map[string]any {
	t.Helper()
	var body struct {
		Data []map[string]any `json:"data"`
	}
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &body))
	return body.Data
}

func fixtureRow(key, status string) emailtemplates.Row {
	return emailtemplates.Row{
		Key:       key,
		Subject:   "Order {{.OrderNumber}}",
		HTMLBody:  "<p>{{.OrderNumber}}</p>",
		TextBody:  "{{.OrderNumber}}",
		Variables: []emailtemplates.Variable{{Name: "OrderNumber", Type: "string", Required: true}},
		Status:    status,
		Version:   3,
		UpdatedAt: time.Date(2026, 8, 1, 9, 30, 0, 0, time.UTC),
		UpdatedBy: "op_previous",
	}
}

func billingFallbacks() map[string]emailtemplates.EmbeddedFallback {
	return map[string]emailtemplates.EmbeddedFallback{
		"dunning_day_5": {
			Subject:  "Payment failed for {{.StoreName}}",
			HTMLBody: "<p>embedded html</p>",
			TextBody: "embedded text",
		},
	}
}

// ─── list ───────────────────────────────────────────────────────────────

// mark8ly#717's actual bug: the twelve billing keys are registered with the
// loader and deliberately never seeded, so a list built from DB rows alone
// cannot see them. This is the assertion that the merge happens — and that
// the unseeded key is reported as `unauthored` sending from `embedded`,
// not as an empty published template.
func TestListMergesRegisteredButUnseededKeys(t *testing.T) {
	store := newStubTemplateStore(fixtureRow("orderdoc_invoice", emailtemplates.StatusPublished))
	reg := newStubRegistry(billingFallbacks())

	rows := decodeRows(t, do(t, templateRouter(store, reg, nil, true), http.MethodGet, "/admin/email-templates", ""))

	byKey := map[string]map[string]any{}
	for _, r := range rows {
		byKey[r["key"].(string)] = r
	}
	require.Len(t, rows, 2)

	unseeded := byKey["dunning_day_5"]
	require.NotNil(t, unseeded, "a registered key with no row must still be listed")
	require.Equal(t, "unauthored", unseeded["state"])
	require.Equal(t, "embedded", unseeded["sends_from"])
	require.Equal(t, true, unseeded["has_embedded_default"])
	require.Equal(t, "Payment failed for {{.StoreName}}", unseeded["subject"],
		"an unauthored key must show the embedded subject that is actually sending")
	require.NotContains(t, unseeded, "version",
		"a version of 0 beside a template that sends fine reads as a broken row")
	require.NotContains(t, unseeded, "updated_at")

	published := byKey["orderdoc_invoice"]
	require.Equal(t, "published", published["state"])
	require.Equal(t, "row", published["sends_from"])
	require.Equal(t, float64(3), published["version"])
	require.Equal(t, "2026-08-01T09:30:00Z", published["updated_at"])
	require.Equal(t, "op_previous", published["updated_by"])
}

// A draft row and an absent row are different states that both send the
// embedded default. Collapsing them is how a console shows a saved draft
// as live, which is the one thing tesserix-home#588 asks the UI to spell
// out.
func TestListDistinguishesDraftFromUnauthored(t *testing.T) {
	store := newStubTemplateStore(fixtureRow("dunning_day_5", emailtemplates.StatusDraft))
	reg := newStubRegistry(billingFallbacks())

	rows := decodeRows(t, do(t, templateRouter(store, reg, nil, true), http.MethodGet, "/admin/email-templates", ""))
	require.Len(t, rows, 1)
	require.Equal(t, "draft", rows[0]["state"])
	require.Equal(t, "embedded", rows[0]["sends_from"],
		"the loader filters status='published', so a draft is invisible to the send path")
}

// A stored row whose key nothing registers has no embedded default to fall
// back to, so a draft on it sends NOTHING. Reporting `embedded` there
// would tell an operator their copy is covered when it is not.
func TestListReportsNothingSendingForUnregisteredDraft(t *testing.T) {
	store := newStubTemplateStore(fixtureRow("retired_key", emailtemplates.StatusDraft))
	reg := newStubRegistry(nil)

	rows := decodeRows(t, do(t, templateRouter(store, reg, nil, true), http.MethodGet, "/admin/email-templates", ""))
	require.Len(t, rows, 1)
	require.Equal(t, "nothing", rows[0]["sends_from"])
	require.Equal(t, false, rows[0]["has_embedded_default"])
}

// A nil slice marshals to null, which defeats a caller's `?? []` exactly
// when there is nothing to show.
func TestListReturnsEmptyArrayNotNull(t *testing.T) {
	rec := do(t, templateRouter(newStubTemplateStore(), newStubRegistry(nil), nil, true),
		http.MethodGet, "/admin/email-templates", "")
	require.JSONEq(t, `{"data":[]}`, rec.Body.String())
}

func TestListReportsStoreFailure(t *testing.T) {
	store := newStubTemplateStore()
	store.listErr = errors.New("boom")
	rec := do(t, templateRouter(store, newStubRegistry(nil), nil, true),
		http.MethodGet, "/admin/email-templates", "")
	require.Equal(t, http.StatusInternalServerError, rec.Code)
	require.Equal(t, "internal_error", errorCode(t, rec))
	require.NotContains(t, rec.Body.String(), "boom",
		"an internal error string must not reach the operator")
}

// ─── get ────────────────────────────────────────────────────────────────

func TestGetReturnsStoredBodies(t *testing.T) {
	store := newStubTemplateStore(fixtureRow("orderdoc_invoice", emailtemplates.StatusPublished))
	data := decodeData(t, do(t, templateRouter(store, newStubRegistry(nil), nil, true),
		http.MethodGet, "/admin/email-templates/orderdoc_invoice", ""))

	require.Equal(t, "<p>{{.OrderNumber}}</p>", data["html_body"])
	require.Equal(t, "{{.OrderNumber}}", data["text_body"])
	require.Len(t, data["variables"], 1)
}

// The editor for an unauthored key must open on the copy that is CURRENTLY
// SENDING. Handing back blanks invites an operator to publish a row that
// silently replaced working copy with nothing.
func TestGetReturnsEmbeddedBodiesForUnauthoredKey(t *testing.T) {
	data := decodeData(t, do(t, templateRouter(newStubTemplateStore(), newStubRegistry(billingFallbacks()), nil, true),
		http.MethodGet, "/admin/email-templates/dunning_day_5", ""))

	require.Equal(t, "unauthored", data["state"])
	require.Equal(t, "<p>embedded html</p>", data["html_body"])
	require.Equal(t, "embedded text", data["text_body"])
	require.Equal(t, []any{}, data["variables"],
		"an embedded default declares no variable schema; empty is the honest answer")
}

// Neither a row nor a call site: authoring here would create copy nothing
// ever sends.
func TestGetRejectsUnknownKey(t *testing.T) {
	rec := do(t, templateRouter(newStubTemplateStore(), newStubRegistry(nil), nil, true),
		http.MethodGet, "/admin/email-templates/not_a_key", "")
	require.Equal(t, http.StatusNotFound, rec.Code)
	require.Equal(t, "unknown_key", errorCode(t, rec))
}

// ─── put ────────────────────────────────────────────────────────────────

const validPut = `{"subject":"Hi {{.Name}}","html_body":"<p>{{.Name}}</p>","text_body":"{{.Name}}","status":"published"}`

func TestUpsertStampsSignedOperatorAndBumpsVersion(t *testing.T) {
	store := newStubTemplateStore(fixtureRow("orderdoc_invoice", emailtemplates.StatusPublished))
	reg := newStubRegistry(nil)
	rec := do(t, templateRouter(store, reg, nil, true), http.MethodPut,
		"/admin/email-templates/orderdoc_invoice", validPut)

	require.Equal(t, http.StatusOK, rec.Code, rec.Body.String())
	require.NotNil(t, store.gotUpsert)
	require.Equal(t, testOperatorID, store.gotUpsert.UpdatedBy,
		"updated_by must come from the signed request, never the body")
	require.Equal(t, "platform.email_templates.write", store.gotUpsert.Capability)

	data := decodeData(t, rec)
	require.Equal(t, float64(4), data["version"], "the version must bump, as the cross-DB UPSERT did")
}

// The attribution must not be forgeable through the body. A caller that
// supplies updated_by gets it ignored.
func TestUpsertIgnoresUpdatedByInBody(t *testing.T) {
	store := newStubTemplateStore(fixtureRow("orderdoc_invoice", emailtemplates.StatusPublished))
	body := `{"subject":"s","html_body":"h","text_body":"t","updated_by":"someone.else@example.com"}`
	rec := do(t, templateRouter(store, newStubRegistry(nil), nil, true), http.MethodPut,
		"/admin/email-templates/orderdoc_invoice", body)
	require.Equal(t, http.StatusOK, rec.Code)
	require.Equal(t, testOperatorID, store.gotUpsert.UpdatedBy)
}

// The write and the send path's loader are the same object in the same
// process, so the save evicts the cache directly — the HTTP round trip to
// /internal/templates/refresh is gone.
func TestUpsertInvalidatesTheLoaderCacheInProcess(t *testing.T) {
	store := newStubTemplateStore(fixtureRow("orderdoc_invoice", emailtemplates.StatusPublished))
	reg := newStubRegistry(nil)
	do(t, templateRouter(store, reg, nil, true), http.MethodPut,
		"/admin/email-templates/orderdoc_invoice", validPut)
	require.Equal(t, []string{"orderdoc_invoice"}, reg.invalidated)
}

// A failed write must not evict, because the cached entry is still correct.
func TestUpsertDoesNotInvalidateOnFailure(t *testing.T) {
	store := newStubTemplateStore(fixtureRow("orderdoc_invoice", emailtemplates.StatusPublished))
	store.upsertErr = errors.New("deadlock")
	reg := newStubRegistry(nil)
	rec := do(t, templateRouter(store, reg, nil, true), http.MethodPut,
		"/admin/email-templates/orderdoc_invoice", validPut)
	require.Equal(t, http.StatusInternalServerError, rec.Code)
	require.Empty(t, reg.invalidated)
	require.NotContains(t, rec.Body.String(), "deadlock")
}

// Server-side validation, not the client's. tesserix-home checked braces in
// its Next.js route and deferred real syntax validation to the send; both
// now happen here.
func TestUpsertRejectsUnbalancedBraces(t *testing.T) {
	store := newStubTemplateStore(fixtureRow("orderdoc_invoice", emailtemplates.StatusPublished))
	body := `{"subject":"ok","html_body":"<p>{{.Name}</p>","text_body":"t"}`
	rec := do(t, templateRouter(store, newStubRegistry(nil), nil, true), http.MethodPut,
		"/admin/email-templates/orderdoc_invoice", body)

	require.Equal(t, http.StatusBadRequest, rec.Code)
	require.Equal(t, "invalid_template", errorCode(t, rec))
	require.Contains(t, rec.Body.String(), "html_body: mismatched template braces (1 {{, 0 }})")
	require.Nil(t, store.gotUpsert, "an invalid template must never reach the store")
}

// `subject` is itself a Go template — orderdoc interpolates the order
// number into it — so it validates exactly like a body.
func TestUpsertValidatesSubjectAsATemplate(t *testing.T) {
	store := newStubTemplateStore(fixtureRow("orderdoc_invoice", emailtemplates.StatusPublished))
	body := `{"subject":"Order {{.Number","html_body":"<p>ok</p>","text_body":"ok"}`
	rec := do(t, templateRouter(store, newStubRegistry(nil), nil, true), http.MethodPut,
		"/admin/email-templates/orderdoc_invoice", body)
	require.Equal(t, http.StatusBadRequest, rec.Code)
	require.Contains(t, rec.Body.String(), "subject:")
	require.Nil(t, store.gotUpsert)
}

// Balanced braces are not valid syntax. A brace count alone accepts this;
// the parse is what rejects it.
func TestUpsertRejectsBalancedButUnparseableTemplate(t *testing.T) {
	store := newStubTemplateStore(fixtureRow("orderdoc_invoice", emailtemplates.StatusPublished))
	body := `{"subject":"ok","html_body":"{{if .A}}no end","text_body":"t"}`
	rec := do(t, templateRouter(store, newStubRegistry(nil), nil, true), http.MethodPut,
		"/admin/email-templates/orderdoc_invoice", body)
	require.Equal(t, http.StatusBadRequest, rec.Code)
	require.Equal(t, "invalid_template", errorCode(t, rec))
	require.Nil(t, store.gotUpsert)
}

func TestUpsertRejectsEmptyFields(t *testing.T) {
	store := newStubTemplateStore(fixtureRow("orderdoc_invoice", emailtemplates.StatusPublished))
	rec := do(t, templateRouter(store, newStubRegistry(nil), nil, true), http.MethodPut,
		"/admin/email-templates/orderdoc_invoice", `{"subject":"  ","html_body":"","text_body":"t"}`)
	require.Equal(t, http.StatusBadRequest, rec.Code)
	require.Contains(t, rec.Body.String(), "subject: must not be empty")
	require.Contains(t, rec.Body.String(), "html_body: must not be empty")
}

// An unrecognised status must be REJECTED, not coerced: silently reading
// "Published" as published publishes copy the operator did not mean to.
func TestUpsertRejectsUnknownStatus(t *testing.T) {
	store := newStubTemplateStore(fixtureRow("orderdoc_invoice", emailtemplates.StatusPublished))
	body := `{"subject":"s","html_body":"h","text_body":"t","status":"Published"}`
	rec := do(t, templateRouter(store, newStubRegistry(nil), nil, true), http.MethodPut,
		"/admin/email-templates/orderdoc_invoice", body)
	require.Equal(t, http.StatusBadRequest, rec.Code)
	require.Equal(t, "invalid_status", errorCode(t, rec))
	require.Nil(t, store.gotUpsert)
}

// An absent status defaults to published, matching the UPSERT this
// replaces.
func TestUpsertDefaultsAbsentStatusToPublished(t *testing.T) {
	store := newStubTemplateStore(fixtureRow("orderdoc_invoice", emailtemplates.StatusPublished))
	do(t, templateRouter(store, newStubRegistry(nil), nil, true), http.MethodPut,
		"/admin/email-templates/orderdoc_invoice", `{"subject":"s","html_body":"h","text_body":"t"}`)
	require.Equal(t, emailtemplates.StatusPublished, store.gotUpsert.Status)
}

// Editing a registered-but-unseeded key is exactly how the twelve billing
// keys become overridable — mark8ly#717's last acceptance line.
func TestUpsertCreatesTheOverrideForAnUnseededKey(t *testing.T) {
	store := newStubTemplateStore()
	rec := do(t, templateRouter(store, newStubRegistry(billingFallbacks()), nil, true),
		http.MethodPut, "/admin/email-templates/dunning_day_5", validPut)
	require.Equal(t, http.StatusOK, rec.Code, rec.Body.String())
	require.Equal(t, float64(1), decodeData(t, rec)["version"])
}

// Keys are owned by code. A console-created key with no call site sends
// nothing while looking exactly like copy that does.
func TestUpsertRefusesAnUnknownKey(t *testing.T) {
	store := newStubTemplateStore()
	rec := do(t, templateRouter(store, newStubRegistry(nil), nil, true),
		http.MethodPut, "/admin/email-templates/invented_key", validPut)
	require.Equal(t, http.StatusNotFound, rec.Code)
	require.Equal(t, "unknown_key", errorCode(t, rec))
	require.Nil(t, store.gotUpsert)
}

// ─── test-send ──────────────────────────────────────────────────────────

// It must render through the SAME loader the send path uses, so it
// exercises what is actually live — not the submitted draft.
func TestTestSendRendersThroughTheLiveRegistry(t *testing.T) {
	sender := &stubTestSender{}
	rec := do(t, templateRouter(newStubTemplateStore(), newStubRegistry(billingFallbacks()), sender, true),
		http.MethodPost, "/admin/email-templates/dunning_day_5/test-send",
		`{"to":"ops@mark8ly.com","vars":{"StoreName":"Acme"}}`)

	require.Equal(t, http.StatusOK, rec.Code, rec.Body.String())
	require.Equal(t, "ops@mark8ly.com", sender.gotTo)
	require.Equal(t, "Payment failed for Acme", sender.gotRendered.Subject)
}

func TestTestSendRequiresARecipient(t *testing.T) {
	rec := do(t, templateRouter(newStubTemplateStore(), newStubRegistry(billingFallbacks()), &stubTestSender{}, true),
		http.MethodPost, "/admin/email-templates/dunning_day_5/test-send", `{"to":"  "}`)
	require.Equal(t, http.StatusBadRequest, rec.Code)
	require.Equal(t, "invalid_recipient", errorCode(t, rec))
}

// The three upstream codes tesserix-home#588 pins to their own operator
// sentence.
func TestTestSendMapsUpstreamFailures(t *testing.T) {
	t.Run("render failure is 422", func(t *testing.T) {
		reg := newStubRegistry(billingFallbacks())
		reg.renderErr = errors.New("template: bad var")
		rec := do(t, templateRouter(newStubTemplateStore(), reg, &stubTestSender{}, true),
			http.MethodPost, "/admin/email-templates/dunning_day_5/test-send", `{"to":"a@b.c"}`)
		require.Equal(t, http.StatusUnprocessableEntity, rec.Code)
		require.Equal(t, "render_failed", errorCode(t, rec))
	})

	t.Run("no sender is 503", func(t *testing.T) {
		rec := do(t, templateRouter(newStubTemplateStore(), newStubRegistry(billingFallbacks()), nil, true),
			http.MethodPost, "/admin/email-templates/dunning_day_5/test-send", `{"to":"a@b.c"}`)
		require.Equal(t, http.StatusServiceUnavailable, rec.Code)
		require.Equal(t, "not_configured", errorCode(t, rec))
	})

	t.Run("provider rejection is 502 without the provider's text", func(t *testing.T) {
		sender := &stubTestSender{err: errors.New("sendgrid: 401 for ops@mark8ly.com")}
		rec := do(t, templateRouter(newStubTemplateStore(), newStubRegistry(billingFallbacks()), sender, true),
			http.MethodPost, "/admin/email-templates/dunning_day_5/test-send", `{"to":"a@b.c"}`)
		require.Equal(t, http.StatusBadGateway, rec.Code)
		require.Equal(t, "send_failed", errorCode(t, rec))
		require.NotContains(t, rec.Body.String(), "sendgrid",
			"a third party's message may carry the recipient back out")
	})
}

// An unknown key reaches Render and comes back as ErrUnknownKey. Reported
// as a render failure so the operator gets the reason.
func TestTestSendOnUnknownKeyReportsRenderFailure(t *testing.T) {
	rec := do(t, templateRouter(newStubTemplateStore(), newStubRegistry(nil), &stubTestSender{}, true),
		http.MethodPost, "/admin/email-templates/nope/test-send", `{"to":"a@b.c"}`)
	require.Equal(t, http.StatusUnprocessableEntity, rec.Code)
	require.Equal(t, "render_failed", errorCode(t, rec))
}

// ─── mount gating ───────────────────────────────────────────────────────

// A handler built non-writable must not answer PUT at all. This is the
// handler-level half of routes.go's guard; routes_email_templates_test.go
// covers the Register-level half.
func TestPutIsUnmountedWhenNotWritable(t *testing.T) {
	rec := do(t, templateRouter(newStubTemplateStore(fixtureRow("k", emailtemplates.StatusPublished)),
		newStubRegistry(nil), nil, false),
		http.MethodPut, "/admin/email-templates/k", validPut)
	require.Equal(t, http.StatusNotFound, rec.Code)
}

// The reads stay available when the write cannot be attributed. Losing
// visibility of what is sending, because the change could not be recorded,
// would be a worse trade than the one the guard exists to make.
func TestReadsStayMountedWhenNotWritable(t *testing.T) {
	rec := do(t, templateRouter(newStubTemplateStore(), newStubRegistry(billingFallbacks()), nil, false),
		http.MethodGet, "/admin/email-templates", "")
	require.Equal(t, http.StatusOK, rec.Code)
}

// ─── pinned contract ────────────────────────────────────────────────────

// goldenFixture is the one shape both golden tests are cut from: a
// published row, a draft row and a registered-but-unseeded key, so the
// fixture exercises all three states and both sends_from answers in one
// payload.
func goldenFixture() (*stubTemplateStore, *stubTemplateRegistry) {
	published := fixtureRow("orderdoc_invoice", emailtemplates.StatusPublished)
	draft := fixtureRow("giftcard_delivery", emailtemplates.StatusDraft)
	draft.Version = 7
	draft.Subject = "Your gift card from {{.StoreName}}"
	draft.UpdatedBy = ""
	draft.UpdatedAt = time.Date(2026, 8, 20, 16, 45, 0, 0, time.UTC)
	// Every real key has an embedded default registered by a Go call site,
	// so the fixture registers all three: the golden then shows the three
	// states as production actually presents them, rather than an
	// unregistered draft that only a retired key could produce.
	fallbacks := billingFallbacks()
	fallbacks["orderdoc_invoice"] = emailtemplates.EmbeddedFallback{Subject: "Order {{.OrderNumber}}"}
	fallbacks["giftcard_delivery"] = emailtemplates.EmbeddedFallback{Subject: "Your gift card"}
	return newStubTemplateStore(published, draft), newStubRegistry(fallbacks)
}

// The golden fixture pins the exact contract shape as BYTES, catching a
// rename or an unauthorized addition that a struct-shaped assertion would
// happily accept — the same guard /admin/notifications and
// /admin/email-sends carry.
func TestEmailTemplatesListMatchesPinnedContract(t *testing.T) {
	store, reg := goldenFixture()
	rec := do(t, templateRouter(store, reg, nil, true), http.MethodGet, "/admin/email-templates", "")
	require.Equal(t, http.StatusOK, rec.Code)

	want, err := os.ReadFile("testdata/email_templates_response.json")
	require.NoError(t, err)
	require.JSONEq(t, string(want), rec.Body.String())
}

func TestEmailTemplateDetailMatchesPinnedContract(t *testing.T) {
	store, reg := goldenFixture()
	rec := do(t, templateRouter(store, reg, nil, true), http.MethodGet,
		"/admin/email-templates/orderdoc_invoice", "")
	require.Equal(t, http.StatusOK, rec.Code)

	want, err := os.ReadFile("testdata/email_template_detail_response.json")
	require.NoError(t, err)
	require.JSONEq(t, string(want), rec.Body.String())
}

// Asserted on the RAW BYTES, not an unmarshalled struct: a struct cannot
// distinguish an absent key from an empty one, and the whole point of
// omitting version/updated_at on an unauthored key is that they are ABSENT
// rather than zero.
func TestEmailTemplatesListOmitsBodiesAndZeroedMetadata(t *testing.T) {
	store, reg := goldenFixture()
	rec := do(t, templateRouter(store, reg, nil, true), http.MethodGet, "/admin/email-templates", "")
	body := rec.Body.String()

	require.NotContains(t, body, "html_body",
		"a list of full HTML bodies is a payload, not a list — they belong on the single-key read")
	require.NotContains(t, body, "text_body")
	require.NotContains(t, body, `"version":0`)
	require.NotContains(t, body, `"updated_at":""`)
}
