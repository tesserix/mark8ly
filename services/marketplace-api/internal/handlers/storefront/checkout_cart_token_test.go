package storefront

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
)

// cartTokenForCheckout decides which cart a checkout's stock holds are
// committed against and which cart its personalisation uploads are claimed
// from. Get it wrong and every order commits against holds it cannot match
// — the #1006-class bug — so the precedence is pinned here.
func TestCartTokenForCheckout(t *testing.T) {
	gin.SetMode(gin.TestMode)
	cookieTok := uuid.NewString()
	bodyTok := uuid.NewString()

	ctx := func(cookie string) *gin.Context {
		w := httptest.NewRecorder()
		c, _ := gin.CreateTestContext(w)
		c.Request = httptest.NewRequest(http.MethodPost, "/checkout", nil)
		if cookie != "" {
			c.Request.AddCookie(&http.Cookie{Name: CartTokenCookie, Value: cookie})
		}
		return c
	}

	t.Run("web: the cookie wins, and the body is ignored when both are present", func(t *testing.T) {
		require.Equal(t, cookieTok, cartTokenForCheckout(ctx(cookieTok), nil))
		require.Equal(t, cookieTok, cartTokenForCheckout(ctx(cookieTok), &bodyTok))
	})

	t.Run("mobile: no cookie jar, so the body token is the cart (#969)", func(t *testing.T) {
		require.Equal(t, bodyTok, cartTokenForCheckout(ctx(""), &bodyTok))
	})

	t.Run("a malformed token is not a cart", func(t *testing.T) {
		bad := "not-a-uuid"
		got := cartTokenForCheckout(ctx("also-not-a-uuid"), &bad)
		_, err := uuid.Parse(got)
		require.NoError(t, err)
		require.NotEqual(t, bad, got)
	})

	t.Run("neither: a fresh token, so enforcement never depends on the caller", func(t *testing.T) {
		got := cartTokenForCheckout(ctx(""), nil)
		_, err := uuid.Parse(got)
		require.NoError(t, err)
	})
}
