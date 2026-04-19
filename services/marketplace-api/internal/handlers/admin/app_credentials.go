package admin

import (
	"context"
	"errors"
	"io"
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"gorm.io/gorm"

	"github.com/mark8ly/marketplace-api/internal/billing/appcreds"
	"github.com/mark8ly/marketplace-api/internal/subscription"
)

// subscriptionLookup is the minimal slice of subscription.Repository
// this handler actually uses. Kept narrow so fakeSubRepo in the unit
// tests doesn't have to track the full Repository surface.
type subscriptionLookup interface {
	GetByStoreID(ctx context.Context, db *gorm.DB, tenantID, storeID uuid.UUID) (*subscription.StoreSubscription, error)
}

// AppCredentialsHandler mounts:
//
//	POST /admin/stores/:storeId/app-credentials/apple
//	POST /admin/stores/:storeId/app-credentials/google
//
// Both endpoints are Pro-gated AND require has_white_label_app_add_on=true
// (spec §14.2 / §18.9). The first gate fails with 403 add_on_not_active
// for non-Pro or non-add-on stores — intentionally the same error code
// to avoid leaking plan info via error message diffs.
type AppCredentialsHandler struct {
	db       *gorm.DB
	subRepo  subscriptionLookup
	credsSvc *appcreds.Service
}

// NewAppCredentialsHandler wires the three dependencies. Construct once at
// boot and pass into Deps. subRepo is narrowed to subscriptionLookup so
// tests only need to stub the one method this handler actually calls.
func NewAppCredentialsHandler(db *gorm.DB, subRepo subscriptionLookup, credsSvc *appcreds.Service) *AppCredentialsHandler {
	return &AppCredentialsHandler{db: db, subRepo: subRepo, credsSvc: credsSvc}
}

// appAddOnGate returns nil if the request carries a store on the Pro plan
// with has_white_label_app_add_on=true. Otherwise it writes a 403 body +
// returns a non-nil error so the caller can early-return.
//
// Error codes kept stable so the admin UI can show the right message:
//
//	"add_on_not_active" — covers both not-on-Pro and on-Pro-without-addon.
//
// Deliberately does NOT distinguish "non-Pro" vs "Pro but no add-on":
// from the user's POV they both mean "purchase the add-on first."
func (h *AppCredentialsHandler) appAddOnGate(c *gin.Context) (tenantID, storeID uuid.UUID, ok bool) {
	tenantID, err := uuid.Parse(c.GetString("tenant_id"))
	if err != nil {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "unauthorized"})
		return uuid.Nil, uuid.Nil, false
	}
	storeID, err = uuid.Parse(c.Param("storeId"))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid_store_id"})
		return uuid.Nil, uuid.Nil, false
	}

	sub, err := h.subRepo.GetByStoreID(c.Request.Context(), h.db, tenantID, storeID)
	if err != nil || sub == nil {
		c.JSON(http.StatusForbidden, gin.H{"error": "add_on_not_active"})
		return uuid.Nil, uuid.Nil, false
	}
	if sub.Plan != subscription.PlanPro || !sub.HasWhiteLabelAppAddOn {
		c.JSON(http.StatusForbidden, gin.H{"error": "add_on_not_active"})
		return uuid.Nil, uuid.Nil, false
	}
	return tenantID, storeID, true
}

// multipartAppleCreds extracts the three parts of the Apple ASC credential
// bundle from a multipart/form-data body. All three are required — the
// handler rejects 400 if any field is missing or the P8 bytes don't
// validate as a PKCS#8 ECDSA P-256 PEM block.
type multipartAppleCreds struct {
	P8       []byte
	IssuerID string
	KeyID    string
}

func readAppleMultipart(c *gin.Context) (multipartAppleCreds, error) {
	const maxP8 = 64 * 1024
	f, _, err := c.Request.FormFile("p8")
	if err != nil {
		return multipartAppleCreds{}, errors.New("missing p8 file")
	}
	defer f.Close()
	p8, err := io.ReadAll(io.LimitReader(f, maxP8))
	if err != nil {
		return multipartAppleCreds{}, errors.New("cannot read p8")
	}

	issuerID := strings.TrimSpace(c.PostForm("issuer_id"))
	keyID := strings.TrimSpace(c.PostForm("key_id"))
	if issuerID == "" {
		return multipartAppleCreds{}, errors.New("missing issuer_id")
	}
	if keyID == "" {
		return multipartAppleCreds{}, errors.New("missing key_id")
	}
	return multipartAppleCreds{P8: p8, IssuerID: issuerID, KeyID: keyID}, nil
}

// PostApple stores the Apple ASC .p8 + issuer_id + key_id triple. All
// three are written atomically from the caller's POV — if any Store call
// fails, the handler returns 500 and caller must retry the whole POST.
// (Partial state is possible on infra failure; the lifecycle advancer at
// day-90 purges ALL four cred types, so a half-written state self-heals.)
//
//	POST /admin/stores/:storeId/app-credentials/apple
//	multipart/form-data:
//	  - p8 (file) : PEM-wrapped PKCS#8 ECDSA P-256 private key
//	  - issuer_id (string)
//	  - key_id (string)
//
// Returns 204 on success; 403 on gate failure; 400 on payload validation
// failure; 500 on Secret Manager write failure.
func (h *AppCredentialsHandler) PostApple(c *gin.Context) {
	tenantID, storeID, ok := h.appAddOnGate(c)
	if !ok {
		return
	}

	creds, err := readAppleMultipart(c)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid_payload", "detail": err.Error()})
		return
	}
	if err := appcreds.ValidateP8(creds.P8); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid_p8_format"})
		return
	}

	actor := "user:" + c.GetString("user_id")
	writes := []struct {
		ct appcreds.CredType
		v  []byte
	}{
		{appcreds.CredTypeAppleP8, creds.P8},
		{appcreds.CredTypeAppleIssuerID, []byte(creds.IssuerID)},
		{appcreds.CredTypeAppleKeyID, []byte(creds.KeyID)},
	}
	for _, w := range writes {
		if err := h.credsSvc.Store(c.Request.Context(), appcreds.StoreInput{
			TenantID: tenantID, StoreID: storeID,
			CredType: w.ct, Payload: w.v, Actor: actor,
		}); err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "store_failed"})
			return
		}
	}
	c.Status(http.StatusNoContent)
}

// PostGoogle stores the Google Play Android Publisher service-account
// JSON blob as a single secret. Validates shape (service_account type,
// required fields) before write.
//
//	POST /admin/stores/:storeId/app-credentials/google
//	multipart/form-data:
//	  - service_account_json (file)
func (h *AppCredentialsHandler) PostGoogle(c *gin.Context) {
	tenantID, storeID, ok := h.appAddOnGate(c)
	if !ok {
		return
	}

	const maxJSON = 64 * 1024
	f, _, err := c.Request.FormFile("service_account_json")
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid_payload", "detail": "missing service_account_json"})
		return
	}
	defer f.Close()
	payload, err := io.ReadAll(io.LimitReader(f, maxJSON))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid_payload"})
		return
	}

	if err := appcreds.ValidateGooglePlayJSON(payload); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid_service_account_json"})
		return
	}

	actor := "user:" + c.GetString("user_id")
	if err := h.credsSvc.Store(c.Request.Context(), appcreds.StoreInput{
		TenantID: tenantID, StoreID: storeID,
		CredType: appcreds.CredTypeGooglePlayJSON, Payload: payload, Actor: actor,
	}); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "store_failed"})
		return
	}
	c.Status(http.StatusNoContent)
}
