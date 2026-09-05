// Package storefront — customer_join.go: the explicit store join.
//
// A Mark8ly customer has ONE platform login and a distinct membership per
// store. The membership record is the customer_profiles row keyed on
// (store_id, email). This file holds the only storefront path that
// creates one, and it runs solely when the customer asked for it.
//
// Everything else — the session middleware, browsing, an authenticated
// API call, checkout — is read-only with respect to membership. Before
// this existed, the session middleware upserted the row on ANY
// authenticated request, so signing in at store2 with a store1 password
// silently made you a customer of store2. See
// docs/superpowers/specs/2026-09-05-customer-store-membership-design.md.
package storefront

import (
	"errors"
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"

	"github.com/mark8ly/marketplace-api/internal/customer"
	"github.com/mark8ly/marketplace-api/internal/stores"
)

// joinRequest carries the optional display name the customer supplied on
// the join screen. The identity (email + uid) is NEVER taken from the
// body — it comes from the verified session/bearer credential on the
// context, so a caller cannot join a store on someone else's behalf.
type joinRequest struct {
	FirstName *string `json:"first_name" binding:"omitempty,max=200"`
	LastName  *string `json:"last_name" binding:"omitempty,max=200"`
}

// Membership handles GET /storefront/stores/:storeSlug/account/membership.
//
// Reports whether the verified identity on this request has joined this
// store. The storefront calls it with a freshly minted (but not yet
// issued) session cookie so it can decide whether to hand that cookie to
// the browser at all — a session must never be minted at a store the
// customer has not joined.
func (h *CustomerAccountHandler) Membership(c *gin.Context) {
	email := c.GetString(CustomerIdentityEmailKey)
	profile, ok := c.Get(CustomerProfileKey)
	if !ok {
		c.JSON(http.StatusOK, gin.H{"data": gin.H{
			"member":  false,
			"blocked": false,
			"email":   email,
		}})
		return
	}
	p, _ := profile.(*customer.CustomerProfile)
	if p == nil {
		c.JSON(http.StatusOK, gin.H{"data": gin.H{
			"member":  false,
			"blocked": false,
			"email":   email,
		}})
		return
	}
	c.JSON(http.StatusOK, gin.H{"data": gin.H{
		"member":  true,
		"blocked": p.Status == customer.StatusBlocked,
		"email":   p.Email,
	}})
}

// Join handles POST /storefront/stores/:storeSlug/account/join.
//
// Creates the customer_profiles row for the verified identity and this
// store, and nothing else: no new identity, no new credential, no change
// to any store the customer already belongs to. Idempotent — a customer
// who is already a member gets 200 and their existing profile back.
func (h *CustomerAccountHandler) Join(c *gin.Context) {
	email := c.GetString(CustomerIdentityEmailKey)
	uid := c.GetString(CustomerIdentityUIDKey)
	if email == "" {
		c.JSON(http.StatusUnauthorized, gin.H{
			"error":   "unauthorized",
			"message": "Authentication required. Please sign in.",
		})
		return
	}

	store, storeID, tenantID, ok := joinStoreIDs(c)
	if !ok {
		return
	}

	var req joinRequest
	// Body is optional — a join with no name is perfectly valid.
	_ = c.ShouldBindJSON(&req)

	profile, err := h.customerSvc.JoinStore(c.Request.Context(), customer.JoinStoreInput{
		StoreID:   storeID,
		TenantID:  tenantID,
		GipUID:    uid,
		Email:     email,
		FirstName: derefString(req.FirstName),
		LastName:  derefString(req.LastName),
	}, c)
	if err != nil {
		if errors.Is(err, customer.ErrBlocked) {
			// The merchant blocked this customer. Joining must not
			// resurrect them, and the message must not read as a
			// retryable glitch.
			c.JSON(http.StatusForbidden, gin.H{
				"error":   "account_blocked",
				"message": "This store has suspended your account, so it can't be reopened here. Please contact the store if you think that's a mistake.",
			})
			return
		}
		h.logger.Error("failed to join store",
			"error", err,
			"email_masked", maskEmailForLog(email),
			"store_id", store.ID,
		)
		c.JSON(http.StatusInternalServerError, gin.H{
			"error":   "internal",
			"message": "We couldn't set up your account with this store. Please try again in a moment.",
		})
		return
	}

	c.JSON(http.StatusOK, gin.H{"data": mapProfileToResponse(profile)})
}

// joinStoreIDs pulls the resolved store off the context and parses its
// ids. Responds 404 and reports false when anything is missing — the same
// no-existence-leak shape the rest of the storefront uses.
func joinStoreIDs(c *gin.Context) (*stores.Store, uuid.UUID, uuid.UUID, bool) {
	storeVal, _ := c.Get("store")
	store, _ := storeVal.(*stores.Store)
	if store == nil {
		respondNotFound(c)
		return nil, uuid.Nil, uuid.Nil, false
	}
	storeID, err := uuid.Parse(store.ID)
	if err != nil {
		respondNotFound(c)
		return nil, uuid.Nil, uuid.Nil, false
	}
	tenantID, err := uuid.Parse(store.TenantID)
	if err != nil {
		respondNotFound(c)
		return nil, uuid.Nil, uuid.Nil, false
	}
	return store, storeID, tenantID, true
}
