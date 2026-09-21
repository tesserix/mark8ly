package admin

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
)

// The gate exists because cancellation never reaches Stripe: there is no
// Subscriptions.Cancel call anywhere in this service, so a real subscriber
// would lose access at the next finalize tick and keep being billed. Until
// that is fixed, the routes that can create live money must not be reachable.

func TestRequireBillingWrites_BlocksWhenDisabled(t *testing.T) {
	gin.SetMode(gin.TestMode)
	r := gin.New()
	reached := false
	r.POST("/x", RequireBillingWrites(false), func(c *gin.Context) {
		reached = true
		c.Status(http.StatusOK)
	})

	w := httptest.NewRecorder()
	r.ServeHTTP(w, httptest.NewRequest(http.MethodPost, "/x", nil))

	if reached {
		t.Fatal("handler ran; the gate must abort before it")
	}
	if w.Code != http.StatusServiceUnavailable {
		t.Fatalf("status = %d, want %d", w.Code, http.StatusServiceUnavailable)
	}

	var body map[string]string
	if err := json.Unmarshal(w.Body.Bytes(), &body); err != nil {
		t.Fatalf("response is not JSON: %v (%s)", err, w.Body.String())
	}
	if body["error"] != "billing_writes_disabled" {
		t.Errorf("error = %q, want %q", body["error"], "billing_writes_disabled")
	}
	if body["message"] == "" {
		t.Error("message is empty; the merchant needs to be told what happened")
	}
}

func TestRequireBillingWrites_PassesWhenEnabled(t *testing.T) {
	gin.SetMode(gin.TestMode)
	r := gin.New()
	reached := false
	r.POST("/x", RequireBillingWrites(true), func(c *gin.Context) {
		reached = true
		c.Status(http.StatusOK)
	})

	w := httptest.NewRecorder()
	r.ServeHTTP(w, httptest.NewRequest(http.MethodPost, "/x", nil))

	if !reached {
		t.Fatal("handler did not run; an enabled gate must be transparent")
	}
	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", w.Code, http.StatusOK)
	}
}
