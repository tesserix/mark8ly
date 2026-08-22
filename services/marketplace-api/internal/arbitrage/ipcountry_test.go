package arbitrage

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
)

func newGinCtx(headers map[string]string, remoteAddr string) *gin.Context {
	gin.SetMode(gin.TestMode)
	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	for k, v := range headers {
		req.Header.Set(k, v)
	}
	if remoteAddr != "" {
		req.RemoteAddr = remoteAddr
	}
	c, _ := gin.CreateTestContext(w)
	c.Request = req
	return c
}

func TestCFIPCountryFromGin_TrustsHeader(t *testing.T) {
	c := newGinCtx(map[string]string{
		"CF-IPCountry":     "US",
		"CF-Connecting-IP": "203.0.113.42",
	}, "10.0.0.1:54321")

	country, ip := CFIPCountryFromGin(c)
	if country != "US" {
		t.Errorf("country=%q, want US", country)
	}
	if ip != "203.0.113.42" {
		t.Errorf("rawIP=%q, want 203.0.113.42", ip)
	}
}

func TestCFIPCountryFromGin_MissingHeader(t *testing.T) {
	c := newGinCtx(nil, "10.0.0.1:12345")

	country, ip := CFIPCountryFromGin(c)
	if country != "??" {
		t.Errorf("country=%q, want ??", country)
	}
	if ip != "10.0.0.1" {
		t.Errorf("rawIP=%q, want 10.0.0.1", ip)
	}
}

func TestCFIPCountryFromGin_NormalizesCase(t *testing.T) {
	c := newGinCtx(map[string]string{
		"CF-IPCountry":     "us",
		"CF-Connecting-IP": "1.2.3.4",
	}, "")

	country, _ := CFIPCountryFromGin(c)
	if country != "US" {
		t.Errorf("country=%q, want US", country)
	}
}

func TestCFIPCountryFromGin_RejectsXX(t *testing.T) {
	c := newGinCtx(map[string]string{
		"CF-IPCountry": "XX",
	}, "1.2.3.4:0")

	country, _ := CFIPCountryFromGin(c)
	if country != "??" {
		t.Errorf("country=%q, want ?? for XX (anonymous network)", country)
	}
}

func TestCFIPCountryFromGin_RejectsT1(t *testing.T) {
	c := newGinCtx(map[string]string{
		"CF-IPCountry": "T1",
	}, "1.2.3.4:0")

	country, _ := CFIPCountryFromGin(c)
	if country != "??" {
		t.Errorf("country=%q, want ?? for T1 (Tor exit)", country)
	}
}
