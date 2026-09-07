package platformadmin_test

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"sort"
	"strings"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"

	"github.com/mark8ly/platform-api/internal/emailtemplates"
	"github.com/mark8ly/platform-api/internal/platformadmin"
	"github.com/mark8ly/platformauth"
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
		UpdatedAt: time.Date(2026, 9, 7, 12, 0, 0, 0, time.UTC),
		UpdatedBy: in.UpdatedBy,
	}
	s.rows[in.Key] = saved
	return saved, nil
}

// stubTemplateRegistry is a hand-rolled EmailTemplateRegistry double.
//
// marketplace-api's equivalent test double wraps a real
// *emailtemplates.Loader, because that package's Loader supports
// registering an arbitrary fallback key at test time (Loader.Register).
// platform-api's emailtemplates.Registry instead wraps
// *notification.Loader, whose embedded fallback set is the FIXED six
// auth/onboarding keys compiled into the binary — there is no
// Register(key, fallback) to inject a synthetic key into. So this double
// implements platformadmin.EmailTemplateRegistry directly against a
// caller-supplied map, the smallest thing that stands in for it.
type stubTemplateRegistry struct {
	fallbacks   map[string]emailtemplates.EmbeddedFallback
	invalidated []string
	renderErr   error
	renderFn    func(ctx context.Context, key string, vars any) (emailtemplates.Rendered, error)
}

func newStubRegistry(fallbacks map[string]emailtemplates.EmbeddedFallback) *stubTemplateRegistry {
	return &stubTemplateRegistry{fallbacks: fallbacks}
}

func (s *stubTemplateRegistry) RegisteredKeys() []string {
	keys := make([]string, 0, len(s.fallbacks))
	for k := range s.fallbacks {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return keys
}

func (s *stubTemplateRegistry) Fallback(key string) (emailtemplates.EmbeddedFallback, bool) {
	fb, ok := s.fallbacks[key]
	return fb, ok
}

func (s *stubTemplateRegistry) Invalidate(key string) {
	s.invalidated = append(s.invalidated, key)
}

func (s *stubTemplateRegistry) Render(ctx context.Context, key string, vars any) (emailtemplates.Rendered, error) {
	if s.renderErr != nil {
		return emailtemplates.Rendered{}, s.renderErr
	}
	if s.renderFn != nil {
		return s.renderFn(ctx, key, vars)
	}
	fb, ok := s.fallbacks[key]
	if !ok {
		return emailtemplates.Rendered{}, errors.New("emailtemplates: unknown key")
	}
	return emailtemplates.Rendered{Subject: fb.Subject, HTMLBody: fb.HTMLBody, TextBody: fb.TextBody}, nil
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
	db *gorm.DB,
) *gin.Engine {
	gin.SetMode(gin.TestMode)
	r := gin.New()
	g := r.Group("", func(c *gin.Context) {
		c.Set(platformauth.CtxOperatorID, testOperatorID)
		c.Set(platformauth.CtxCapability, "platform.email_templates.write")
	})
	platformadmin.NewEmailTemplatesHandler(store, registry, sender, writable, db, nil).Register(g)
	return r
}

// emailTemplatesTestRouter builds a full platformadmin.Register router —
// signature-checked, like the rest of routes_test.go — with the
// email-templates handler wired via a stub store/registry. Used by
// TestNegativeControl's email-templates subtest, which needs the route
// actually mounted so an unsigned 401 is distinguishable from an
// unmounted 404.
func emailTemplatesTestRouter(t *testing.T, secret string, nonces platformauth.NonceStore) *gin.Engine {
	t.Helper()
	gin.SetMode(gin.TestMode)

	r := gin.New()
	platformadmin.Register(r.Group(platformadmin.MountPrefix), platformadmin.Deps{
		Secret:                secret,
		NonceStore:            nonces,
		EmailTemplates:        newStubTemplateStore(),
		EmailTemplateRegistry: newStubRegistry(nil),
	})
	return r
}

// do sends an Idempotency-Key by default, because the PUT requires one
// (mirroring marketplace-api's mark8ly#730 fix) and every other assertion
// in this file is about something else. Use doWithoutIdempotencyKey for
// the case that tests the requirement.
func do(t *testing.T, r *gin.Engine, method, path, body string) *httptest.ResponseRecorder {
	t.Helper()
	return doHeaders(t, r, method, path, body, true)
}

func doWithoutIdempotencyKey(t *testing.T, r *gin.Engine, method, path, body string) *httptest.ResponseRecorder {
	t.Helper()
	return doHeaders(t, r, method, path, body, false)
}

func doHeaders(t *testing.T, r *gin.Engine, method, path, body string, withIdempotencyKey bool) *httptest.ResponseRecorder {
	t.Helper()
	var req *http.Request
	if body == "" {
		req = httptest.NewRequest(method, path, nil)
	} else {
		req = httptest.NewRequest(method, path, strings.NewReader(body))
		req.Header.Set("Content-Type", "application/json")
	}
	if withIdempotencyKey {
		req.Header.Set("Idempotency-Key", "test-key-"+method+"-"+path)
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
		Subject:   "{{.Code}} is your Mark8ly sign-in code",
		HTMLBody:  "<p>{{.Code}}</p>",
		TextBody:  "{{.Code}}",
		Variables: []emailtemplates.Variable{{Name: "Code", Type: "string", Required: true}},
		Status:    status,
		Version:   3,
		UpdatedAt: time.Date(2026, 8, 1, 9, 30, 0, 0, time.UTC),
		UpdatedBy: "op_previous",
	}
}

func welcomeFallback() map[string]emailtemplates.EmbeddedFallback {
	return map[string]emailtemplates.EmbeddedFallback{
		"welcome": {
			Subject:  "Welcome to Mark8ly, {{.BusinessName}}",
			HTMLBody: "<p>embedded html</p>",
			TextBody: "embedded text",
		},
		"login_otp": {
			Subject:  "{{.Code}} is your Mark8ly sign-in code",
			HTMLBody: "<p>embedded html</p>",
			TextBody: "embedded text",
		},
	}
}

// ─── list ───────────────────────────────────────────────────────────────

// TestListMergesRegisteredButUnseededKeys mirrors marketplace-api's own
// test of the same name (mark8ly#717's bug): a registered key with no
// database row must still be listed as unauthored/sends-from-embedded,
// not silently dropped.
func TestListMergesRegisteredButUnseededKeys(t *testing.T) {
	store := newStubTemplateStore(fixtureRow("login_otp", emailtemplates.StatusPublished))
	reg := newStubRegistry(welcomeFallback())

	rows := decodeRows(t, do(t, templateRouter(store, reg, nil, true, nil), http.MethodGet, "/admin/email-templates", ""))

	byKey := map[string]map[string]any{}
	for _, r := range rows {
		byKey[r["key"].(string)] = r
	}
	require.Len(t, rows, 2)

	unseeded := byKey["welcome"]
	require.NotNil(t, unseeded, "a registered key with no row must still be listed")
	require.Equal(t, "unauthored", unseeded["state"])
	require.Equal(t, "embedded", unseeded["sends_from"])
	require.Equal(t, true, unseeded["has_embedded_default"])
	require.Equal(t, "Welcome to Mark8ly, {{.BusinessName}}", unseeded["subject"])
	require.NotContains(t, unseeded, "version")
	require.NotContains(t, unseeded, "updated_at")

	published := byKey["login_otp"]
	require.Equal(t, "published", published["state"])
	require.Equal(t, "row", published["sends_from"])
	require.Equal(t, float64(3), published["version"])
	require.Equal(t, "2026-08-01T09:30:00Z", published["updated_at"])
	require.Equal(t, "op_previous", published["updated_by"])
}

func TestListReturnsEmptyArrayNotNull(t *testing.T) {
	rec := do(t, templateRouter(newStubTemplateStore(), newStubRegistry(nil), nil, true, nil),
		http.MethodGet, "/admin/email-templates", "")
	require.Equal(t, http.StatusOK, rec.Code)
	require.JSONEq(t, `{"data":[]}`, rec.Body.String())
}

func TestListReportsStoreFailure(t *testing.T) {
	store := newStubTemplateStore()
	store.listErr = errors.New("boom")
	rec := do(t, templateRouter(store, newStubRegistry(nil), nil, true, nil),
		http.MethodGet, "/admin/email-templates", "")
	require.Equal(t, http.StatusInternalServerError, rec.Code)
}

// ─── get ────────────────────────────────────────────────────────────────

func TestGetReturnsStoredBodies(t *testing.T) {
	store := newStubTemplateStore(fixtureRow("login_otp", emailtemplates.StatusPublished))
	data := decodeData(t, do(t, templateRouter(store, newStubRegistry(nil), nil, true, nil),
		http.MethodGet, "/admin/email-templates/login_otp", ""))

	require.Equal(t, "{{.Code}}", data["text_body"])
	require.Equal(t, "<p>{{.Code}}</p>", data["html_body"])
}

func TestGetReturnsEmbeddedBodiesForUnauthoredKey(t *testing.T) {
	reg := newStubRegistry(welcomeFallback())
	data := decodeData(t, do(t, templateRouter(newStubTemplateStore(), reg, nil, true, nil),
		http.MethodGet, "/admin/email-templates/welcome", ""))

	require.Equal(t, "unauthored", data["state"])
	require.Equal(t, "embedded text", data["text_body"])
	require.Equal(t, []any{}, data["variables"],
		"an embedded default declares no variable schema on this endpoint, matching marketplace-api")
}

func TestGetRejectsUnknownKey(t *testing.T) {
	rec := do(t, templateRouter(newStubTemplateStore(), newStubRegistry(nil), nil, true, nil),
		http.MethodGet, "/admin/email-templates/not_a_real_key", "")
	require.Equal(t, http.StatusNotFound, rec.Code)
}

// ─── upsert ─────────────────────────────────────────────────────────────

const validBody = `{"subject":"Hi {{.Name}}","html_body":"<p>{{.Name}}</p>","text_body":"{{.Name}}","status":"published"}`

func TestUpsertStampsSignedOperatorAndBumpsVersion(t *testing.T) {
	store := newStubTemplateStore(fixtureRow("login_otp", emailtemplates.StatusPublished))
	reg := newStubRegistry(welcomeFallback())
	r := templateRouter(store, reg, nil, true, nil)

	rec := do(t, r, http.MethodPut, "/admin/email-templates/login_otp", validBody)
	require.Equal(t, http.StatusOK, rec.Code, rec.Body.String())

	require.Equal(t, testOperatorID, store.gotUpsert.UpdatedBy)
	data := decodeData(t, rec)
	require.Equal(t, float64(4), data["version"])
}

func TestUpsertRequiresIdempotencyKey(t *testing.T) {
	store := newStubTemplateStore(fixtureRow("login_otp", emailtemplates.StatusPublished))
	r := templateRouter(store, newStubRegistry(nil), nil, true, nil)

	rec := doWithoutIdempotencyKey(t, r, http.MethodPut, "/admin/email-templates/login_otp", validBody)
	require.Equal(t, http.StatusBadRequest, rec.Code)
	require.Contains(t, rec.Body.String(), "idempotency_key_required")
}

func TestUpsertInvalidatesTheRegistry(t *testing.T) {
	store := newStubTemplateStore(fixtureRow("login_otp", emailtemplates.StatusPublished))
	reg := newStubRegistry(nil)
	r := templateRouter(store, reg, nil, true, nil)

	rec := do(t, r, http.MethodPut, "/admin/email-templates/login_otp", validBody)
	require.Equal(t, http.StatusOK, rec.Code, rec.Body.String())
	require.Equal(t, []string{"login_otp"}, reg.invalidated)
}

func TestUpsertRejectsUnbalancedBraces(t *testing.T) {
	store := newStubTemplateStore(fixtureRow("login_otp", emailtemplates.StatusPublished))
	r := templateRouter(store, newStubRegistry(nil), nil, true, nil)

	body := `{"subject":"Hi {{.Name}","html_body":"<p>ok</p>","text_body":"ok","status":"published"}`
	rec := do(t, r, http.MethodPut, "/admin/email-templates/login_otp", body)
	require.Equal(t, http.StatusBadRequest, rec.Code)
	require.Contains(t, rec.Body.String(), "mismatched template braces")
}

func TestUpsertRejectsUnknownStatus(t *testing.T) {
	store := newStubTemplateStore(fixtureRow("login_otp", emailtemplates.StatusPublished))
	r := templateRouter(store, newStubRegistry(nil), nil, true, nil)

	body := `{"subject":"ok","html_body":"ok","text_body":"ok","status":"live"}`
	rec := do(t, r, http.MethodPut, "/admin/email-templates/login_otp", body)
	require.Equal(t, http.StatusBadRequest, rec.Code)
	require.Contains(t, rec.Body.String(), "invalid_status")
}

func TestUpsertCreatesTheOverrideForAnUnseededKey(t *testing.T) {
	store := newStubTemplateStore()
	reg := newStubRegistry(welcomeFallback())
	r := templateRouter(store, reg, nil, true, nil)

	rec := do(t, r, http.MethodPut, "/admin/email-templates/welcome", validBody)
	require.Equal(t, http.StatusOK, rec.Code, rec.Body.String())
}

func TestUpsertRefusesAnUnknownKey(t *testing.T) {
	store := newStubTemplateStore()
	r := templateRouter(store, newStubRegistry(nil), nil, true, nil)

	rec := do(t, r, http.MethodPut, "/admin/email-templates/not_a_real_key", validBody)
	require.Equal(t, http.StatusNotFound, rec.Code)
}

// TestWriteRouteRequiresDBThroughRegister proves the mount guard in
// routes.go's Register — not just NewEmailTemplatesHandler's writable
// flag directly, which TestPutIsUnmountedWhenNotWritable below covers.
// EmailTemplates and EmailTemplateRegistry are both non-nil (a real
// deployment would have them) but DB is nil, so the write route must not
// exist: routes.go computes `writable` as `deps.DB != nil`, and this is
// the test that would fail if that expression were ever weakened (e.g.
// hardcoded to true) — see the mutation check recorded for this task.
func TestWriteRouteRequiresDBThroughRegister(t *testing.T) {
	gin.SetMode(gin.TestMode)
	r := gin.New()
	platformadmin.Register(r.Group(platformadmin.MountPrefix), platformadmin.Deps{
		Secret:                testSecret,
		NonceStore:            newMemNonces(),
		EmailTemplates:        newStubTemplateStore(fixtureRow("login_otp", emailtemplates.StatusPublished)),
		EmailTemplateRegistry: newStubRegistry(nil),
		DB:                    nil,
	})

	req := signedRequest(t, testSecret, http.MethodPut, platformadmin.MountPrefix+"/admin/email-templates/login_otp")
	req.Body = http.NoBody
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	require.Equal(t, http.StatusNotFound, w.Code,
		"the write route must not mount without a database — it is what records the change against an operator")
}

func TestPutIsUnmountedWhenNotWritable(t *testing.T) {
	store := newStubTemplateStore(fixtureRow("login_otp", emailtemplates.StatusPublished))
	r := templateRouter(store, newStubRegistry(nil), nil, false, nil)

	rec := do(t, r, http.MethodPut, "/admin/email-templates/login_otp", validBody)
	require.Equal(t, http.StatusNotFound, rec.Code,
		"the PUT route must not exist at all when the handler was built non-writable")
}

func TestReadsStayMountedWhenNotWritable(t *testing.T) {
	store := newStubTemplateStore(fixtureRow("login_otp", emailtemplates.StatusPublished))
	r := templateRouter(store, newStubRegistry(nil), nil, false, nil)

	rec := do(t, r, http.MethodGet, "/admin/email-templates", "")
	require.Equal(t, http.StatusOK, rec.Code)
}

// Idempotency replay against a real database (mirroring marketplace-api's
// mark8ly#730 fix) is covered by TestUpsertReplaysIdempotentRetryFromDB in
// email_templates_integration_test.go (//go:build integration) — it needs
// pkg/testdb, and this file's unit tests must not touch a database at all
// (the shared test DB is not safe for concurrent/ad-hoc runs). What THIS
// file proves without a database is TestUpsertRequiresIdempotencyKey
// above: the header is required before the handler ever reaches the
// reservation machinery.

// ─── test-send ──────────────────────────────────────────────────────────

func TestTestSendRendersThroughTheRegistry(t *testing.T) {
	reg := newStubRegistry(welcomeFallback())
	sender := &stubTestSender{}
	r := templateRouter(newStubTemplateStore(), reg, sender, true, nil)

	body := `{"to":"operator@example.com","vars":{"BusinessName":"Acme"}}`
	rec := do(t, r, http.MethodPost, "/admin/email-templates/welcome/test-send", body)
	require.Equal(t, http.StatusOK, rec.Code, rec.Body.String())
	require.Equal(t, "operator@example.com", sender.gotTo)
	require.Equal(t, "Welcome to Mark8ly, {{.BusinessName}}", sender.gotRendered.Subject)
}

func TestTestSendRequiresARecipient(t *testing.T) {
	r := templateRouter(newStubTemplateStore(), newStubRegistry(welcomeFallback()), &stubTestSender{}, true, nil)

	rec := do(t, r, http.MethodPost, "/admin/email-templates/welcome/test-send", `{"vars":{}}`)
	require.Equal(t, http.StatusBadRequest, rec.Code)
}

func TestTestSendMapsUpstreamFailures(t *testing.T) {
	sender := &stubTestSender{err: errors.New("provider rejected")}
	r := templateRouter(newStubTemplateStore(), newStubRegistry(welcomeFallback()), sender, true, nil)

	rec := do(t, r, http.MethodPost, "/admin/email-templates/welcome/test-send", `{"to":"op@example.com"}`)
	require.Equal(t, http.StatusBadGateway, rec.Code)
	require.Contains(t, rec.Body.String(), "send_failed")
}

func TestTestSendOnUnknownKeyReportsRenderFailure(t *testing.T) {
	r := templateRouter(newStubTemplateStore(), newStubRegistry(nil), &stubTestSender{}, true, nil)

	rec := do(t, r, http.MethodPost, "/admin/email-templates/not_a_real_key/test-send", `{"to":"op@example.com"}`)
	require.Equal(t, http.StatusUnprocessableEntity, rec.Code)
	require.Contains(t, rec.Body.String(), "render_failed")
}

func TestTestSendWithNoSenderAnswers503(t *testing.T) {
	r := templateRouter(newStubTemplateStore(), newStubRegistry(welcomeFallback()), nil, true, nil)

	rec := do(t, r, http.MethodPost, "/admin/email-templates/welcome/test-send", `{"to":"op@example.com"}`)
	require.Equal(t, http.StatusServiceUnavailable, rec.Code)
	require.Contains(t, rec.Body.String(), "not_configured")
}
