package breakglass

import (
	"bytes"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

func TestHMACIPHash_Deterministic(t *testing.T) {
	key := HMACKey("k")
	a := HMACIPHash(key, "203.0.113.7")
	b := HMACIPHash(key, "203.0.113.7")
	require.True(t, bytes.Equal(a, b))
	c := HMACIPHash(key, "198.51.100.1")
	require.False(t, bytes.Equal(a, c))
}

func TestHMACIPHash_DifferentKeys_DifferentHashes(t *testing.T) {
	a := HMACIPHash(HMACKey("k1"), "203.0.113.7")
	b := HMACIPHash(HMACKey("k2"), "203.0.113.7")
	require.False(t, bytes.Equal(a, b))
}

func TestClientIPFromRequest_PrefersXFF(t *testing.T) {
	gin.SetMode(gin.TestMode)
	req := httptest.NewRequest("POST", "/", nil)
	req.Header.Set("X-Forwarded-For", "198.51.100.7, 10.0.0.1")
	req.RemoteAddr = "127.0.0.1:443"
	c, _ := gin.CreateTestContext(httptest.NewRecorder())
	c.Request = req

	require.Equal(t, "198.51.100.7", ClientIPFromRequest(c))
}

func TestClientIPFromRequest_FallsBackToRemoteAddr(t *testing.T) {
	gin.SetMode(gin.TestMode)
	req := httptest.NewRequest("POST", "/", nil)
	req.RemoteAddr = "203.0.113.99:4444"
	c, _ := gin.CreateTestContext(httptest.NewRecorder())
	c.Request = req

	require.Equal(t, "203.0.113.99", ClientIPFromRequest(c))
}
