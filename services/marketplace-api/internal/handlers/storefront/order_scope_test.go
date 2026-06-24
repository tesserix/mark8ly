package storefront

import (
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"

	"github.com/mark8ly/marketplace-api/internal/order"
)

func TestOrderMatchesCaller(t *testing.T) {
	cid := uuid.New()
	o := &order.Order{CustomerID: &cid, CustomerEmail: "Jane@Example.com"}

	if !orderMatchesCaller(o, cid.String(), "") {
		t.Error("matching profile id should match")
	}
	if orderMatchesCaller(o, uuid.New().String(), "") {
		t.Error("a different profile id must NOT match")
	}
	if !orderMatchesCaller(o, "", "jane@example.com") {
		t.Error("email should match case-insensitively")
	}
	if orderMatchesCaller(o, "", "someone.else@example.com") {
		t.Error("a different email must NOT match")
	}
	if orderMatchesCaller(o, "", "") {
		t.Error("no identity must NOT match (fail closed)")
	}

	// Order with no owning customer id can only be matched by email.
	noCust := &order.Order{CustomerEmail: "guest@example.com"}
	if orderMatchesCaller(noCust, uuid.New().String(), "") {
		t.Error("nil CustomerID must not match a profile id")
	}
	if !orderMatchesCaller(noCust, "", "guest@example.com") {
		t.Error("guest order should match by email")
	}
}

func TestResolveCallerCustomer(t *testing.T) {
	gin.SetMode(gin.TestMode)

	// Trusted backend caller (Otto MCP) via X-Customer-Email header.
	c, _ := gin.CreateTestContext(httptest.NewRecorder())
	c.Request = httptest.NewRequest("GET", "/", nil)
	c.Request.Header.Set("X-Customer-Email", "  a@b.com  ")
	if pid, email, ok := resolveCallerCustomer(c); !ok || email != "a@b.com" || pid != "" {
		t.Errorf("header path: got pid=%q email=%q ok=%v", pid, email, ok)
	}

	// No session, no header → not identified (fail closed; callers store-scope).
	c2, _ := gin.CreateTestContext(httptest.NewRecorder())
	c2.Request = httptest.NewRequest("GET", "/", nil)
	if _, _, ok := resolveCallerCustomer(c2); ok {
		t.Error("no identity should resolve ok=false")
	}
}
