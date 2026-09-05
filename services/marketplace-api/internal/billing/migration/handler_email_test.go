package migration_test

import (
	"context"
	"errors"
	"net/http"
	"sync"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/mark8ly/marketplace-api/internal/billing/migration"
	"github.com/mark8ly/marketplace-api/internal/email"
)

// ---------------------------------------------------------------------------
// Fakes
// ---------------------------------------------------------------------------

// sentMail is one captured Send call.
type sentMail struct {
	template email.TemplateID
	to       string
	data     map[string]any
}

// fakeMailer records what it was asked to send, and can be made to fail like
// a provider outage.
type fakeMailer struct {
	mu   sync.Mutex
	sent []sentMail
	err  error
}

func (f *fakeMailer) Send(_ context.Context, t email.TemplateID, to string, data map[string]any) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.sent = append(f.sent, sentMail{template: t, to: to, data: data})
	return f.err
}

func (f *fakeMailer) calls() []sentMail {
	f.mu.Lock()
	defer f.mu.Unlock()
	return append([]sentMail(nil), f.sent...)
}

// countingCounter records label values instead of touching Prometheus.
type countingCounter struct {
	mu     sync.Mutex
	labels [][]string
}

func (c *countingCounter) record(vals ...string) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.labels = append(c.labels, vals)
}

func (c *countingCounter) all() [][]string {
	c.mu.Lock()
	defer c.mu.Unlock()
	return append([][]string(nil), c.labels...)
}

type incFunc func()

func (f incFunc) Inc() { f() }

type fakeSent struct{ c *countingCounter }

func (f fakeSent) WithTemplate(template string) migration.CounterIncrementer {
	return incFunc(func() { f.c.record(template) })
}

type fakeSkip struct{ c *countingCounter }

func (f fakeSkip) WithTemplateReason(template, reason string) migration.CounterIncrementer {
	return incFunc(func() { f.c.record(template, reason) })
}

// ---------------------------------------------------------------------------
// Helpers
// ---------------------------------------------------------------------------

// lookupReturning builds a RecipientLookup that always answers the same way.
func lookupReturning(addr, name string) migration.RecipientLookup {
	return func(context.Context, uuid.UUID) (string, string) { return addr, name }
}

type emailRouterOpts struct {
	mailer *fakeMailer
	lookup migration.RecipientLookup
	sent   *countingCounter
	skip   *countingCounter
}

func newReviewRouterWithEmail(store *fakeReviewStore, o emailRouterOpts) *gin.Engine {
	gin.SetMode(gin.TestMode)
	r := gin.New()
	h := migration.NewHandler(store, nil, nil)
	if o.mailer != nil || o.lookup != nil {
		var sent migration.SentCounter
		var skip migration.SkipCounter
		if o.sent != nil {
			sent = fakeSent{o.sent}
		}
		if o.skip != nil {
			skip = fakeSkip{o.skip}
		}
		h = h.WithEmail(o.mailer, o.lookup, sent, skip)
	}
	r.Use(func(c *gin.Context) {
		c.Set("user_id", validUserID)
		c.Next()
	})
	r.POST("/internal/csm/migration-fast-path/:id/review", h.Review)
	return r
}

const csmNotes = "Verified Shopify export CSV — domain age confirmed."

func decide(t *testing.T, r *gin.Engine, decision string) *http.Response {
	t.Helper()
	w := doPost(t, r, "/internal/csm/migration-fast-path/"+validReviewID+"/review", map[string]any{
		"decision": decision,
		"notes":    csmNotes,
	})
	return w.Result()
}

// ---------------------------------------------------------------------------
// Delivery
// ---------------------------------------------------------------------------

func TestReview_Approve_SendsApprovalEmail(t *testing.T) {
	mailer := &fakeMailer{}
	sent := &countingCounter{}
	r := newReviewRouterWithEmail(&fakeReviewStore{}, emailRouterOpts{
		mailer: mailer,
		lookup: lookupReturning("merchant@example.com", "Nadia's Ceramics"),
		sent:   sent,
	})

	resp := decide(t, r, "approve")
	assert.Equal(t, http.StatusOK, resp.StatusCode)

	calls := mailer.calls()
	require.Len(t, calls, 1)
	assert.Equal(t, email.TemplateMigrationFastPathApproved, calls[0].template)
	assert.Equal(t, "merchant@example.com", calls[0].to)
	assert.Equal(t, "Nadia's Ceramics", calls[0].data["store_name"])

	assert.Equal(t, [][]string{{string(email.TemplateMigrationFastPathApproved)}}, sent.all())
}

func TestReview_Reject_SendsRejectionEmail(t *testing.T) {
	mailer := &fakeMailer{}
	r := newReviewRouterWithEmail(&fakeReviewStore{}, emailRouterOpts{
		mailer: mailer,
		lookup: lookupReturning("merchant@example.com", "Nadia's Ceramics"),
	})

	resp := decide(t, r, "reject")
	assert.Equal(t, http.StatusOK, resp.StatusCode)

	calls := mailer.calls()
	require.Len(t, calls, 1)
	assert.Equal(t, email.TemplateMigrationFastPathRejected, calls[0].template)
}

// The CSM's notes are written for internal review, not for the merchant.
// Sending them would publish internal commentary about a merchant on the
// strength of a field nobody authored for an external reader.
func TestReview_EmailNeverCarriesTheCSMNotes(t *testing.T) {
	mailer := &fakeMailer{}
	r := newReviewRouterWithEmail(&fakeReviewStore{}, emailRouterOpts{
		mailer: mailer,
		lookup: lookupReturning("merchant@example.com", "Nadia's Ceramics"),
	})

	decide(t, r, "reject")

	calls := mailer.calls()
	require.Len(t, calls, 1)
	for k, v := range calls[0].data {
		if s, ok := v.(string); ok {
			assert.NotContains(t, s, csmNotes, "template data %q leaked the CSM notes", k)
		}
	}
	_, hasNotes := calls[0].data["notes"]
	assert.False(t, hasNotes, "template data must not carry a notes key")
}

// ---------------------------------------------------------------------------
// The decision must survive every email failure
// ---------------------------------------------------------------------------

func TestReview_NoMailerConfigured_StillSucceeds(t *testing.T) {
	// newReviewRouter builds a handler that never had WithEmail called.
	r := newReviewRouter(&fakeReviewStore{}, true)
	resp := decide(t, r, "approve")
	assert.Equal(t, http.StatusOK, resp.StatusCode)
}

func TestReview_NoBillingEmailOnFile_SkipsWithoutSending(t *testing.T) {
	mailer := &fakeMailer{}
	skip := &countingCounter{}
	r := newReviewRouterWithEmail(&fakeReviewStore{}, emailRouterOpts{
		mailer: mailer,
		lookup: lookupReturning("", "your store"),
		skip:   skip,
	})

	resp := decide(t, r, "approve")
	assert.Equal(t, http.StatusOK, resp.StatusCode)
	assert.Empty(t, mailer.calls(), "provider must not see an empty address")

	recorded := skip.all()
	require.Len(t, recorded, 1)
	assert.Equal(t, string(email.TemplateMigrationFastPathApproved), recorded[0][0])
	assert.Equal(t, email.ReasonNoAddress, recorded[0][1])
}

// Bootstrap mints placeholder billing+<uuid>@mark8ly.local addresses. Handing
// one to the provider would hard-bounce and cost sender reputation.
func TestReview_PlaceholderAddress_SkipsWithoutSending(t *testing.T) {
	mailer := &fakeMailer{}
	skip := &countingCounter{}
	r := newReviewRouterWithEmail(&fakeReviewStore{}, emailRouterOpts{
		mailer: mailer,
		lookup: lookupReturning("billing+"+validStoreID+"@mark8ly.local", "your store"),
		skip:   skip,
	})

	resp := decide(t, r, "approve")
	assert.Equal(t, http.StatusOK, resp.StatusCode)
	assert.Empty(t, mailer.calls())

	recorded := skip.all()
	require.Len(t, recorded, 1)
	assert.Equal(t, email.ReasonPlaceholderAddress, recorded[0][1])
}

func TestReview_ProviderFailure_StillSucceedsAndCountsSkip(t *testing.T) {
	mailer := &fakeMailer{err: errors.New("smtp: connection refused")}
	skip := &countingCounter{}
	sent := &countingCounter{}
	r := newReviewRouterWithEmail(&fakeReviewStore{}, emailRouterOpts{
		mailer: mailer,
		lookup: lookupReturning("merchant@example.com", "Nadia's Ceramics"),
		sent:   sent,
		skip:   skip,
	})

	resp := decide(t, r, "approve")
	assert.Equal(t, http.StatusOK, resp.StatusCode, "the review is already committed; email must not fail it")
	assert.Len(t, skip.all(), 1)
	assert.Empty(t, sent.all(), "a failed send is not a sent email")
}

// ---------------------------------------------------------------------------
// No decision, no email
// ---------------------------------------------------------------------------

func TestReview_RepositoryFailure_SendsNothing(t *testing.T) {
	mailer := &fakeMailer{}
	r := newReviewRouterWithEmail(
		&fakeReviewStore{approveErr: errors.New("db down")},
		emailRouterOpts{
			mailer: mailer,
			lookup: lookupReturning("merchant@example.com", "Nadia's Ceramics"),
		})

	resp := decide(t, r, "approve")
	assert.Equal(t, http.StatusInternalServerError, resp.StatusCode)
	assert.Empty(t, mailer.calls(), "nothing was decided, so there is nothing to announce")
}

func TestReview_NotFound_SendsNothing(t *testing.T) {
	mailer := &fakeMailer{}
	r := newReviewRouterWithEmail(
		&fakeReviewStore{approveErr: migration.ErrNotFound},
		emailRouterOpts{
			mailer: mailer,
			lookup: lookupReturning("merchant@example.com", "Nadia's Ceramics"),
		})

	resp := decide(t, r, "approve")
	assert.Equal(t, http.StatusNotFound, resp.StatusCode)
	assert.Empty(t, mailer.calls())
}
