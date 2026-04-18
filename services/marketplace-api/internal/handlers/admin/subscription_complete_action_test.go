package admin_test

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"

	"github.com/mark8ly/marketplace-api/internal/handlers/admin"
	"github.com/mark8ly/marketplace-api/internal/subscription"
)

// fakeSubLoader is a test double for admin.SubscriptionLoaderForTest.
type fakeSubLoader struct {
	sub *subscription.StoreSubscription
	err error
}

func (f *fakeSubLoader) LoadForTenant(_ context.Context, _, _ uuid.UUID) (*subscription.StoreSubscription, error) {
	return f.sub, f.err
}

const completeActionStoreID = "aaaaaaaa-aaaa-aaaa-aaaa-aaaaaaaaaaaa"
const completeActionTenantID = "bbbbbbbb-bbbb-bbbb-bbbb-bbbbbbbbbbbb"

// newCompleteActionRouter builds a minimal gin engine wired like RegisterAdmin
// would, injecting tenant_id onto the context.
func newCompleteActionRouter(loader admin.SubscriptionLoaderForTest) *gin.Engine {
	gin.SetMode(gin.TestMode)
	r := gin.New()
	h := admin.NewCompleteActionHandlerWithLoader(loader, nil)
	r.Use(func(c *gin.Context) {
		c.Set("tenant_id", completeActionTenantID)
		c.Next()
	})
	r.GET("/admin/stores/:storeId/subscription/complete-action", h.Redirect)
	return r
}

func getCompleteAction(t *testing.T, r *gin.Engine, storeID string) *httptest.ResponseRecorder {
	t.Helper()
	req := httptest.NewRequest(http.MethodGet,
		"/admin/stores/"+storeID+"/subscription/complete-action", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	return w
}

func TestCompleteAction_NoPendingAction_Returns404(t *testing.T) {
	sub := &subscription.StoreSubscription{HostedInvoiceURL: nil}
	r := newCompleteActionRouter(&fakeSubLoader{sub: sub})
	w := getCompleteAction(t, r, completeActionStoreID)

	assert.Equal(t, http.StatusNotFound, w.Code)
	assert.Contains(t, w.Body.String(), "no_pending_action")
}

func TestCompleteAction_EmptyURL_Returns404(t *testing.T) {
	empty := ""
	sub := &subscription.StoreSubscription{HostedInvoiceURL: &empty}
	r := newCompleteActionRouter(&fakeSubLoader{sub: sub})
	w := getCompleteAction(t, r, completeActionStoreID)

	assert.Equal(t, http.StatusNotFound, w.Code)
	assert.Contains(t, w.Body.String(), "no_pending_action")
}

func TestCompleteAction_Redirects302ToStoredURL(t *testing.T) {
	stripeURL := "https://invoice.stripe.com/i/acct_test/test_abc"
	sub := &subscription.StoreSubscription{HostedInvoiceURL: &stripeURL}
	r := newCompleteActionRouter(&fakeSubLoader{sub: sub})
	w := getCompleteAction(t, r, completeActionStoreID)

	assert.Equal(t, http.StatusFound, w.Code)
	assert.Equal(t, stripeURL, w.Header().Get("Location"))
}

func TestCompleteAction_SubscriptionNotFound_Returns404(t *testing.T) {
	r := newCompleteActionRouter(&fakeSubLoader{err: gorm.ErrRecordNotFound})
	w := getCompleteAction(t, r, completeActionStoreID)

	assert.Equal(t, http.StatusNotFound, w.Code)
	assert.Contains(t, w.Body.String(), "subscription_not_found")
}

func TestCompleteAction_InvalidStoreID_Returns400(t *testing.T) {
	r := newCompleteActionRouter(&fakeSubLoader{})
	w := getCompleteAction(t, r, "not-a-uuid")

	require.Equal(t, http.StatusBadRequest, w.Code)
	assert.Contains(t, w.Body.String(), "validation_failed")
}
