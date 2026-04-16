package storefront

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"

	"github.com/mark8ly/marketplace-api/internal/customer"
	"github.com/mark8ly/marketplace-api/internal/stores"
	"github.com/mark8ly/marketplace-api/pkg/apperrors"
)

// RequireStorefrontKey returns a middleware that rejects requests missing
// or mismatching X-Storefront-Key. When secret is empty the middleware is
// a no-op — used for local dev and tests.
func RequireStorefrontKey(secret string) gin.HandlerFunc {
	return func(c *gin.Context) {
		if secret == "" {
			c.Next()
			return
		}
		if c.GetHeader("X-Storefront-Key") != secret {
			respondNotFound(c)
			return
		}
		c.Next()
	}
}

// SlugLookup is the narrow read contract StoreContext needs. Any type
// that satisfies Get(ctx, slug) can be supplied — production wiring
// passes a *stores.SlugCache; tests inject fakes.
type SlugLookup interface {
	Get(ctx context.Context, slug string) (*stores.Store, error)
}

// StoreContext resolves the :storeSlug path param to a store row via the
// SlugCache. Sets the resolved store on the gin context under key "store".
// Returns 404 on miss / suspended / archived — no existence leak — and
// 500 on unexpected cache errors.
func StoreContext(cache SlugLookup) gin.HandlerFunc {
	return func(c *gin.Context) {
		slug := c.Param("storeSlug")
		if slug == "" {
			respondNotFound(c)
			return
		}
		store, err := cache.Get(c.Request.Context(), slug)
		if err != nil {
			if !errors.Is(err, stores.ErrNotFound) {
				c.AbortWithStatusJSON(http.StatusInternalServerError, map[string]any{
					"error":   "internal",
					"message": "internal server error",
				})
				return
			}
			respondNotFound(c)
			return
		}
		if store == nil || store.Status != stores.StatusActive {
			respondNotFound(c)
			return
		}
		c.Set("store", store)
		c.Next()
	}
}

func respondNotFound(c *gin.Context) {
	c.AbortWithStatusJSON(http.StatusNotFound, map[string]any{
		"error":   string(apperrors.CodeNotFound),
		"message": "not found",
	})
}

// customerContextKey values set by OptionalCustomerAuth.
const (
	CustomerProfileIDKey = "customer_profile_id"
	CustomerEmailKey     = "customer_email"
	CustomerGipUIDKey    = "customer_gip_uid"
	CustomerProfileKey   = "customer_profile"
)

// sessionClaims represents the decoded auth-bff session cookie payload.
type sessionClaims struct {
	GipUID    string `json:"gip_uid"`
	Email     string `json:"email"`
	FirstName string `json:"first_name"`
	LastName  string `json:"last_name"`
	Exp       int64  `json:"exp"`
}

// OptionalCustomerAuth reads the auth-bff session cookie, validates its
// HMAC signature, and if valid, upserts a customer_profiles row via
// Service.EnsureProfile and sets customer context on gin.
//
// If the cookie is missing, invalid, or expired, the request continues
// as a guest (no customer context). This middleware never aborts.
func OptionalCustomerAuth(secret string, customerSvc *customer.Service, logger *slog.Logger) gin.HandlerFunc {
	return func(c *gin.Context) {
		if secret == "" {
			c.Next()
			return
		}

		cookieVal, err := c.Cookie("mp_customer_session")
		if err != nil || cookieVal == "" {
			c.Next()
			return
		}

		claims, err := validateSessionCookie(cookieVal, secret)
		if err != nil {
			logger.Debug("invalid customer session cookie",
				"error", err,
				"remote_addr", c.ClientIP(),
			)
			c.Next()
			return
		}

		if claims.Exp > 0 && time.Now().Unix() > claims.Exp {
			logger.Debug("expired customer session cookie",
				"email", claims.Email,
			)
			c.Next()
			return
		}

		storeVal, exists := c.Get("store")
		if !exists {
			c.Next()
			return
		}
		store, ok := storeVal.(*stores.Store)
		if !ok || store == nil {
			c.Next()
			return
		}

		storeID, err := uuid.Parse(store.ID)
		if err != nil {
			c.Next()
			return
		}
		tenantID, err := uuid.Parse(store.TenantID)
		if err != nil {
			c.Next()
			return
		}

		profile, err := customerSvc.EnsureProfile(c.Request.Context(), customer.EnsureProfileInput{
			StoreID:   storeID,
			TenantID:  tenantID,
			GipUID:    claims.GipUID,
			Email:     claims.Email,
			FirstName: claims.FirstName,
			LastName:  claims.LastName,
		}, c)
		if err != nil {
			logger.Error("failed to ensure customer profile",
				"error", err,
				"email", claims.Email,
				"store_id", store.ID,
			)
			c.Next()
			return
		}

		c.Set(CustomerProfileIDKey, profile.ID.String())
		c.Set(CustomerEmailKey, profile.Email)
		c.Set(CustomerGipUIDKey, claims.GipUID)
		c.Set(CustomerProfileKey, profile)

		c.Next()
	}
}

// validateSessionCookie validates the HMAC-SHA256 signature of the cookie.
func validateSessionCookie(cookie, secret string) (*sessionClaims, error) {
	parts := strings.SplitN(cookie, ".", 2)
	if len(parts) != 2 {
		return nil, errors.New("malformed cookie: missing separator")
	}

	payloadB64 := parts[0]
	sigB64 := parts[1]

	mac := hmac.New(sha256.New, []byte(secret))
	mac.Write([]byte(payloadB64))
	expectedSig := mac.Sum(nil)

	// The storefront encodes the signature as hex (not base64url).
	// Try hex first, fall back to base64url for forward compatibility.
	actualSig, err := hex.DecodeString(sigB64)
	if err != nil {
		// Fallback: try base64url encoding
		actualSig, err = base64.RawURLEncoding.DecodeString(sigB64)
		if err != nil {
			return nil, errors.New("malformed cookie: invalid signature encoding")
		}
	}
	if !hmac.Equal(expectedSig, actualSig) {
		return nil, errors.New("invalid cookie signature")
	}

	// Payload is standard base64 (not base64url) from the storefront.
	payloadBytes, err := base64.StdEncoding.DecodeString(payloadB64)
	if err != nil {
		// Fallback: try base64url
		payloadBytes, err = base64.RawURLEncoding.DecodeString(payloadB64)
	}
	if err != nil {
		return nil, errors.New("malformed cookie: invalid payload encoding")
	}

	var claims sessionClaims
	if err := json.Unmarshal(payloadBytes, &claims); err != nil {
		return nil, fmt.Errorf("malformed cookie: %w", err)
	}

	if claims.Email == "" {
		return nil, errors.New("malformed cookie: missing email")
	}

	return &claims, nil
}

// RequireCustomerAuth returns 401 if no customer context was set by
// OptionalCustomerAuth. Use on /account/* routes.
func RequireCustomerAuth() gin.HandlerFunc {
	return func(c *gin.Context) {
		profileID, exists := c.Get(CustomerProfileIDKey)
		if !exists || profileID == "" {
			c.AbortWithStatusJSON(http.StatusUnauthorized, map[string]any{
				"error":   "unauthorized",
				"message": "Authentication required. Please sign in.",
			})
			return
		}

		profileVal, _ := c.Get(CustomerProfileKey)
		if profile, ok := profileVal.(*customer.CustomerProfile); ok && profile != nil {
			if profile.Status == customer.StatusBlocked {
				c.AbortWithStatusJSON(http.StatusForbidden, map[string]any{
					"error":   "forbidden",
					"message": "Your account has been suspended. Please contact the store.",
				})
				return
			}
		}

		c.Next()
	}
}
