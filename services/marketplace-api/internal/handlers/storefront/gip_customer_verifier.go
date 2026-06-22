// Package storefront — gip_customer_verifier.go: production CustomerVerifier
// backed by the Firebase Admin Auth client. Mirrors auth.GIPVerifier (used
// by the admin mobile group) but projects the claims the storefront needs —
// the GIP UID and the verified email — instead of the tenant claim.
package storefront

import (
	"context"
	"errors"

	firebaseAuth "firebase.google.com/go/v4/auth"
	"github.com/gin-gonic/gin"
)

// GIPCustomerVerifier verifies storefront customer GIP ID tokens.
type GIPCustomerVerifier struct {
	client *firebaseAuth.Client
}

// NewGIPCustomerVerifier builds a verifier from a Firebase Auth client.
func NewGIPCustomerVerifier(client *firebaseAuth.Client) *GIPCustomerVerifier {
	return &GIPCustomerVerifier{client: client}
}

// VerifyCustomerToken validates the token signature and returns the GIP UID
// + email. The UID is otto's customer identity (skips the OTP step) and the
// email is forwarded so staff see who they're talking to.
func (v *GIPCustomerVerifier) VerifyCustomerToken(idToken string) (gipUID, email string, err error) {
	token, err := v.client.VerifyIDToken(context.Background(), idToken)
	if err != nil {
		return "", "", err
	}
	if token.UID == "" {
		return "", "", errors.New("token missing uid")
	}
	email, _ = token.Claims["email"].(string)
	return token.UID, email, nil
}

// RegisterMobileStorefrontSupport mounts ONLY the support-chat routes under
// /mobile/storefront/stores/:storeSlug/support with the store-context +
// customer-auth middleware. It exists so the support feature can ship
// before the full mobile storefront route group (RegisterMobileStorefront)
// is wired. When that group is mounted, drop this call to avoid a duplicate
// registration (support is also wired inside RegisterMobileStorefront).
func RegisterMobileStorefrontSupport(router *gin.RouterGroup, h *MobileSupportHandler, slug SlugLookup, verifier CustomerVerifier) {
	if h == nil || verifier == nil {
		return
	}
	grp := router.Group("/mobile/storefront/stores/:storeSlug/support",
		StoreContext(slug),
		MobileCustomerAuth(verifier),
	)
	h.Register(grp)
}
