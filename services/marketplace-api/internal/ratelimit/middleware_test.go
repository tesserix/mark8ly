package ratelimit

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
)

func TestPerIP_AllowsUnderLimit(t *testing.T) {
	gin.SetMode(gin.TestMode)
	r := gin.New()
	r.Use(PerIP(10, 10)) // 10 req/s, burst 10
	r.POST("/test", func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{"ok": true})
	})

	for i := 0; i < 10; i++ {
		w := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodPost, "/test", nil)
		req.RemoteAddr = "1.2.3.4:12345"
		r.ServeHTTP(w, req)
		if w.Code != http.StatusOK {
			t.Fatalf("request %d: expected 200, got %d", i, w.Code)
		}
	}
}

func TestPerIP_BlocksOverLimit(t *testing.T) {
	gin.SetMode(gin.TestMode)
	r := gin.New()
	// Very low limit: 1 req/s, burst 1
	r.Use(PerIP(1, 1))
	r.POST("/test", func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{"ok": true})
	})

	// First request should pass.
	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/test", nil)
	req.RemoteAddr = "1.2.3.4:12345"
	r.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("first request: expected 200, got %d", w.Code)
	}

	// Second immediate request should be rate-limited.
	w2 := httptest.NewRecorder()
	req2 := httptest.NewRequest(http.MethodPost, "/test", nil)
	req2.RemoteAddr = "1.2.3.4:12345"
	r.ServeHTTP(w2, req2)
	if w2.Code != http.StatusTooManyRequests {
		t.Fatalf("second request: expected 429, got %d", w2.Code)
	}
}

func TestPerIP_DifferentIPsIndependent(t *testing.T) {
	gin.SetMode(gin.TestMode)
	r := gin.New()
	r.Use(PerIP(1, 1))
	r.POST("/test", func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{"ok": true})
	})

	// IP A
	w1 := httptest.NewRecorder()
	req1 := httptest.NewRequest(http.MethodPost, "/test", nil)
	req1.RemoteAddr = "1.1.1.1:12345"
	r.ServeHTTP(w1, req1)
	if w1.Code != http.StatusOK {
		t.Fatalf("ip-a: expected 200, got %d", w1.Code)
	}

	// IP B — should still be allowed
	w2 := httptest.NewRecorder()
	req2 := httptest.NewRequest(http.MethodPost, "/test", nil)
	req2.RemoteAddr = "2.2.2.2:12345"
	r.ServeHTTP(w2, req2)
	if w2.Code != http.StatusOK {
		t.Fatalf("ip-b: expected 200, got %d", w2.Code)
	}
}
