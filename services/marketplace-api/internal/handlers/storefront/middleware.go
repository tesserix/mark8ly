package storefront

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"crypto/subtle"
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
		if !constantTimeEqual(c.GetHeader("X-Storefront-Key"), secret) {
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
//
// Two tiers, and the difference is the whole point of the membership
// model. The IDENTITY keys mean "this request carries a credential this
// storefront verified" — a Mark8ly login is platform-wide, so that says
// nothing about whether the customer belongs to THIS store. The PROFILE
// keys mean "this identity has joined this store", and they are set only
// when a customer_profiles row already exists. No middleware may create
// that row; see CustomerProfileService.
const (
	CustomerProfileIDKey = "customer_profile_id"
	CustomerEmailKey     = "customer_email"
	CustomerGipUIDKey    = "customer_gip_uid"
	CustomerProfileKey   = "customer_profile"

	// CustomerIdentityEmailKey / CustomerIdentityUIDKey carry the
	// verified identity even when it has no membership here, so
	// RequireCustomerAuth can tell "not signed in" apart from "signed in
	// but not a member of this store" and the join endpoint has an
	// authenticated subject to create the membership for.
	CustomerIdentityEmailKey = "customer_identity_email"
	CustomerIdentityUIDKey   = "customer_identity_uid"
)

// CustomerProfileService is the customer-profile surface the storefront
// handlers depend on. It models the one *customer.Service main.go builds,
// so both the read-only session middleware and the write-side join
// handler take the same value.
//
// LookupProfile is read-only. JoinStore CREATES a membership.
// OptionalCustomerAuth and mobileCustomerProfileMW must never call
// JoinStore: a customer must not acquire a membership of a store by
// browsing it, which is exactly the bug this interface's split exists to
// prevent (docs/superpowers/specs/2026-09-05-customer-store-membership-design.md).
// TestSessionPathNeverCreatesMembership fails loudly if that is undone.
type CustomerProfileService interface {
	LookupProfile(ctx context.Context, storeID uuid.UUID, email string) (*customer.CustomerProfile, error)
	JoinStore(ctx context.Context, in customer.JoinStoreInput, c *gin.Context) (*customer.CustomerProfile, error)
}

// sessionClaims represents the decoded auth-bff session cookie payload.
type sessionClaims struct {
	UID       string `json:"uid"`
	GipUID    string `json:"gip_uid"`
	Email     string `json:"email"`
	FirstName string `json:"first_name"`
	LastName  string `json:"last_name"`
	StoreSlug string `json:"store_slug"`
	StoreID   string `json:"store_id"`
	TenantID  string `json:"tenant_id"`
	Exp       int64  `json:"exp"`
}

// OptionalCustomerAuth reads the auth-bff session cookie, validates its
// HMAC signature and store scope, and sets customer context on gin.
//
// It is READ-ONLY with respect to membership. A valid cookie sets the
// identity keys; the profile keys follow only if this identity has
// already joined this store. An authenticated customer with no
// membership here resolves to "not a member" — never to a freshly minted
// row. Creation lives behind the explicit join
// (CustomerAccountHandler.Join).
//
// If the cookie is missing, invalid, or expired, the request continues
// as a guest (no customer context). This middleware never aborts.
func OptionalCustomerAuth(secret string, customerSvc CustomerProfileService, logger *slog.Logger) gin.HandlerFunc {
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
		if !claims.MatchesStore(store) {
			logger.Debug("customer session scoped to a different store",
				"email", claims.Email,
				"session_store_slug", claims.StoreSlug,
				"request_store_slug", store.Slug,
			)
			c.Next()
			return
		}

		storeID, err := uuid.Parse(store.ID)
		if err != nil {
			c.Next()
			return
		}

		// The credential is verified and scoped to this store, so the
		// identity is trustworthy from here on — independently of whether
		// it has a membership.
		c.Set(CustomerIdentityEmailKey, claims.Email)
		c.Set(CustomerIdentityUIDKey, claims.UIDOrGipUID())

		profile, err := customerSvc.LookupProfile(c.Request.Context(), storeID, claims.Email)
		if err != nil {
			if !errors.Is(err, customer.ErrNotFound) {
				logger.Error("failed to look up customer membership",
					"error", err,
					"store_id", store.ID,
				)
			}
			// Not a member of this store (or the lookup failed): continue
			// with identity only. RequireCustomerAuth turns that into an
			// actionable 403 on the routes that need a membership.
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

// RequireCustomerIdentity aborts with 401 unless the request carries a
// verified customer identity. It deliberately does NOT require a
// membership — it is the guard for the join endpoint, which exists
// precisely for authenticated customers who have not joined yet.
func RequireCustomerIdentity() gin.HandlerFunc {
	return func(c *gin.Context) {
		if c.GetString(CustomerIdentityEmailKey) == "" {
			c.AbortWithStatusJSON(http.StatusUnauthorized, map[string]any{
				"error":   "unauthorized",
				"message": "Authentication required. Please sign in.",
			})
			return
		}
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

	if claims.Email == "" || claims.UIDOrGipUID() == "" {
		return nil, errors.New("malformed cookie: missing identity")
	}

	return &claims, nil
}

func (s sessionClaims) UIDOrGipUID() string {
	if s.UID != "" {
		return s.UID
	}
	return s.GipUID
}

func (s sessionClaims) MatchesStore(store *stores.Store) bool {
	if store == nil {
		return false
	}
	return s.StoreSlug == store.Slug &&
		s.StoreID == store.ID &&
		s.TenantID == store.TenantID
}

func constantTimeEqual(got, want string) bool {
	if got == "" || want == "" {
		return false
	}
	gotSum := sha256.Sum256([]byte(got))
	wantSum := sha256.Sum256([]byte(want))
	return subtle.ConstantTimeCompare(gotSum[:], wantSum[:]) == 1
}

// RequireCustomerAuth returns 401 if no customer context was set by
// OptionalCustomerAuth. Use on /account/* routes.
func RequireCustomerAuth() gin.HandlerFunc {
	return func(c *gin.Context) {
		profileID, exists := c.Get(CustomerProfileIDKey)
		if !exists || profileID == "" {
			// Signed in, but not a customer of THIS store. Say so
			// truthfully and offer the fix: a generic 401 here would tell
			// a customer with a perfectly good password to "sign in"
			// again, which can never succeed.
			if c.GetString(CustomerIdentityEmailKey) != "" {
				c.AbortWithStatusJSON(http.StatusForbidden, map[string]any{
					"error":         "membership_required",
					"message":       "Your Mark8ly login works here, but you don't have an account with this store yet. Join the store to continue.",
					"join_required": true,
				})
				return
			}
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
