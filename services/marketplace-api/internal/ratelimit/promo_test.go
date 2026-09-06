package ratelimit

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
)

// routerWithTenant builds a router that stamps tenantID into the context the
// way tenantMW does upstream, then applies PromoPerTenant.
func routerWithTenant(tenantID string) *gin.Engine {
	gin.SetMode(gin.TestMode)
	r := gin.New()
	r.Use(func(c *gin.Context) {
		if tenantID != "" {
			c.Set("tenant_id", tenantID)
		}
		c.Next()
	})
	r.Use(PromoPerTenant())
	r.POST("/apply-promo", func(c *gin.Context) { c.JSON(http.StatusOK, gin.H{"ok": true}) })
	return r
}

func postN(t *testing.T, r *gin.Engine, n int) []int {
	t.Helper()
	codes := make([]int, 0, n)
	for i := 0; i < n; i++ {
		w := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodPost, "/apply-promo", nil)
		req.RemoteAddr = "1.2.3.4:12345"
		r.ServeHTTP(w, req)
		codes = append(codes, w.Code)
	}
	return codes
}

func TestPromoPerTenant_BlocksOverTheBurst(t *testing.T) {
	r := routerWithTenant("tenant-a")
	codes := postN(t, r, 11)

	for i, c := range codes[:10] {
		if c != http.StatusOK {
			t.Fatalf("request %d within burst: expected 200, got %d", i, c)
		}
	}
	if codes[10] != http.StatusTooManyRequests {
		t.Fatalf("11th request: expected 429, got %d", codes[10])
	}
}

func TestPromoPerTenant_OneTenantCannotExhaustAnother(t *testing.T) {
	// THE ROW THIS EXISTS FOR. The limiter it replaced keyed on an email
	// address that mark8ly#773 stopped sending, so every request would have
	// carried the same empty key. Had buildKeyedLimiter bucketed those
	// together rather than skipping them, one merchant's retries would lock
	// out every other merchant.
	shared := PromoPerTenant()
	build := func(tenantID string) *gin.Engine {
		gin.SetMode(gin.TestMode)
		r := gin.New()
		r.Use(func(c *gin.Context) { c.Set("tenant_id", tenantID); c.Next() })
		r.Use(shared)
		r.POST("/apply-promo", func(c *gin.Context) { c.JSON(http.StatusOK, gin.H{"ok": true}) })
		return r
	}

	if got := postN(t, build("tenant-a"), 11); got[10] != http.StatusTooManyRequests {
		t.Fatalf("tenant-a should be exhausted: got %d", got[10])
	}
	// Same middleware instance, different tenant: unaffected.
	if got := postN(t, build("tenant-b"), 1); got[0] != http.StatusOK {
		t.Fatalf("tenant-b must not inherit tenant-a's bucket: got %d", got[0])
	}
}

func TestPromoPerTenant_NoTenantIsSkippedNotShared(t *testing.T) {
	// An unattributable request is let through rather than counted into one
	// global bucket. Bucketing them together would let a single unkeyed caller
	// starve every other unkeyed caller, which is worse than not limiting.
	r := routerWithTenant("")
	for i, c := range postN(t, r, 15) {
		if c != http.StatusOK {
			t.Fatalf("request %d with no tenant_id: expected 200 (skipped), got %d", i, c)
		}
	}
}
