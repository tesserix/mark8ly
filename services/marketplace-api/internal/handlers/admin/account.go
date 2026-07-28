// Package admin — account.go: HTTP handler for the admin account
// endpoints (S1). Profile read/update persists to the user_profiles
// table. MFA and sessions proxy to auth-bff.
package admin

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"path"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"

	"github.com/mark8ly/marketplace-api/internal/media"
	"github.com/mark8ly/marketplace-api/internal/userprofile"
	"github.com/mark8ly/marketplace-api/pkg/apperrors"
)

// profileDTO is the wire shape the admin UI consumes. Field names mirror
// AccountProfile in apps/admin/lib/api/settings-tier2-api.ts.
type profileDTO struct {
	UserID     string    `json:"user_id"`
	Email      string    `json:"email"`
	Name       string    `json:"name"`
	Phone      string    `json:"phone"`
	AvatarURL  string    `json:"avatar_url"`
	MFAEnabled bool      `json:"mfa_enabled"`
	CreatedAt  time.Time `json:"created_at"`
}

func toProfileDTO(p userprofile.Profile, mfaEnabled bool) profileDTO {
	return profileDTO{
		UserID:     p.UserID,
		Email:      p.Email,
		Name:       p.DisplayName,
		Phone:      p.Phone,
		AvatarURL:  p.AvatarURL,
		MFAEnabled: mfaEnabled,
		CreatedAt:  p.CreatedAt,
	}
}

// AccountHandler handles /admin/account endpoints.
// Profile persists to user_profiles. MFA and sessions proxy to auth-bff.
type AccountHandler struct {
	profiles       userprofile.Store
	uploader       media.Uploader
	authBFFURL     string
	internalSecret string
	httpClient     *http.Client
	logger         *slog.Logger
}

// NewAccountHandler constructs an AccountHandler. profiles is required.
// uploader is optional (nil disables avatar uploads). internalSecret
// is the shared MARKETPLACE_INTERNAL_AUTH_SECRET used to sign
// outbound calls to auth-bff's /internal endpoints; empty value
// disables the "reset my profile" cascade beyond the local DB wipe.
func NewAccountHandler(profiles userprofile.Store, authBFFURL, internalSecret string, logger *slog.Logger) *AccountHandler {
	return &AccountHandler{
		profiles:       profiles,
		authBFFURL:     strings.TrimRight(authBFFURL, "/"),
		internalSecret: internalSecret,
		httpClient:     &http.Client{Timeout: 10 * time.Second},
		logger:         logger,
	}
}

// SetUploader wires the media uploader for the avatar upload-url endpoint.
func (h *AccountHandler) SetUploader(u media.Uploader) { h.uploader = u }

// GetProfile handles GET /admin/account.
// Upsert-on-read: if the row doesn't exist yet, create one from the
// session headers. Response is wrapped as {data: ...} so the client
// can stay consistent with the rest of the admin API.
func (h *AccountHandler) GetProfile(c *gin.Context) {
	userID := c.GetString("user_id")
	email := c.GetString("user_email")
	if userID == "" {
		RespondErr(c, apperrors.ValidationFailed("user_id", "missing user context"), h.logger)
		return
	}

	profile, err := h.loadOrSeed(c, userID, email)
	if err != nil {
		RespondErr(c, err, h.logger)
		return
	}
	mfa := h.fetchMFAEnabled(c.Request.Context(), userID, email)
	c.JSON(http.StatusOK, gin.H{"data": toProfileDTO(profile, mfa)})
}

// UpdateProfileRequest is the request body for PATCH /admin/account.
// All fields are pointers so the client can send a partial update; nil
// means "leave this field alone".
type UpdateProfileRequest struct {
	DisplayName *string `json:"display_name"`
	Phone       *string `json:"phone"`
	AvatarURL   *string `json:"avatar_url"`
}

// UpdateProfile handles PATCH /admin/account.
func (h *AccountHandler) UpdateProfile(c *gin.Context) {
	var req UpdateProfileRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		RespondErr(c, apperrors.ValidationFailed("body", "invalid request body"), h.logger)
		return
	}

	userID := c.GetString("user_id")
	email := c.GetString("user_email")
	if userID == "" {
		RespondErr(c, apperrors.ValidationFailed("user_id", "missing user context"), h.logger)
		return
	}

	profile, err := h.loadOrSeed(c, userID, email)
	if err != nil {
		RespondErr(c, err, h.logger)
		return
	}

	updates := map[string]any{"updated_at": time.Now()}
	if req.DisplayName != nil {
		updates["display_name"] = strings.TrimSpace(*req.DisplayName)
	}
	if req.Phone != nil {
		updates["phone"] = strings.TrimSpace(*req.Phone)
	}
	if req.AvatarURL != nil {
		updates["avatar_url"] = strings.TrimSpace(*req.AvatarURL)
	}

	if len(updates) > 1 {
		if err := h.profiles.Update(c.Request.Context(), userID, updates); err != nil {
			h.logger.Error("update user profile", "user_id", userID, "err", err)
			RespondErr(c, fmt.Errorf("update profile: %w", err), h.logger)
			return
		}
		// re-load so response reflects the freshly persisted row
		reloaded, err := h.profiles.Get(c.Request.Context(), userID)
		if err != nil {
			RespondErr(c, fmt.Errorf("reload profile: %w", err), h.logger)
			return
		}
		profile = reloaded
	}

	mfa := h.fetchMFAEnabled(c.Request.Context(), userID, email)
	c.JSON(http.StatusOK, gin.H{"data": toProfileDTO(profile, mfa)})
}

// loadOrSeed returns the user_profile row, inserting a skeleton row on
// first read so the rest of the admin UI can count on a stable shape.
func (h *AccountHandler) loadOrSeed(c *gin.Context, userID, email string) (userprofile.Profile, error) {
	ctx := c.Request.Context()

	p, err := h.profiles.Get(ctx, userID)
	if err == nil {
		// Keep email mirrored if it drifted (e.g. GIP change).
		if email != "" && p.Email != email {
			_ = h.profiles.Update(ctx, userID, map[string]any{
				"email":      email,
				"updated_at": time.Now(),
			})
			p.Email = email
		}
		return p, nil
	}
	if !errors.Is(err, userprofile.ErrNotFound) {
		return userprofile.Profile{}, fmt.Errorf("load profile: %w", err)
	}

	now := time.Now()
	p = userprofile.Profile{
		UserID:    userID,
		Email:     email,
		CreatedAt: now,
		UpdatedAt: now,
	}
	if err := h.profiles.Create(ctx, p); err != nil {
		return userprofile.Profile{}, fmt.Errorf("seed profile: %w", err)
	}
	return p, nil
}


// ─── Avatar upload ───────────────────────────────────────────────────

var avatarAllowedContentTypes = map[string]struct{}{
	"image/png":  {},
	"image/jpeg": {},
	"image/webp": {},
}

// AvatarUploadURLRequest is the POST body for avatar/upload-url.
type AvatarUploadURLRequest struct {
	Filename    string `json:"filename" binding:"required"`
	ContentType string `json:"content_type" binding:"required"`
}

// AvatarUploadURLResponse mirrors BrandingUploadURLResponse. The client
// PUTs the file to upload_url, then PATCH /admin/account with
// {avatar_url: public_url} to persist.
type AvatarUploadURLResponse struct {
	UploadURL  string    `json:"upload_url"`
	PublicURL  string    `json:"public_url"`
	StorageKey string    `json:"storage_key"`
	ExpiresAt  time.Time `json:"expires_at"`
}

// AvatarUploadURL handles POST /admin/account/avatar/upload-url.
// Returns a V4 signed PUT URL plus the public URL the client should
// persist via PATCH /admin/account. No DB writes happen here.
func (h *AccountHandler) AvatarUploadURL(c *gin.Context) {
	userID := c.GetString("user_id")
	if userID == "" {
		RespondErr(c, apperrors.ValidationFailed("user_id", "missing user context"), h.logger)
		return
	}

	var req AvatarUploadURLRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		RespondErr(c, apperrors.ValidationFailed("body", err.Error()), h.logger)
		return
	}
	if _, ok := avatarAllowedContentTypes[req.ContentType]; !ok {
		RespondErr(c, apperrors.ValidationFailed("content_type", "must be image/png, image/jpeg, or image/webp"), h.logger)
		return
	}

	if h.uploader == nil {
		c.AbortWithStatusJSON(http.StatusNotImplemented, gin.H{
			"error":   "not_implemented",
			"message": "uploads are disabled on this deployment",
		})
		return
	}
	signer, ok := h.uploader.(media.SignedURLGenerator)
	if !ok {
		c.AbortWithStatusJSON(http.StatusNotImplemented, gin.H{
			"error":   "not_implemented",
			"message": "signed upload URLs require a real GCS bucket",
		})
		return
	}
	bn, ok := h.uploader.(interface{ BucketName() string })
	if !ok {
		c.AbortWithStatusJSON(http.StatusNotImplemented, gin.H{
			"error":   "not_implemented",
			"message": "uploader does not expose bucket name",
		})
		return
	}

	key := buildAvatarStorageKey(userID, req.Filename)
	uploadURL, expiresAt, err := signer.SignedUploadURL(c.Request.Context(), key, req.ContentType, 15*time.Minute)
	if err != nil {
		RespondErr(c, err, h.logger)
		return
	}

	publicURL := fmt.Sprintf("https://storage.googleapis.com/%s/%s", bn.BucketName(), key)
	c.JSON(http.StatusOK, AvatarUploadURLResponse{
		UploadURL:  uploadURL,
		PublicURL:  publicURL,
		StorageKey: key,
		ExpiresAt:  expiresAt,
	})
}

// buildAvatarStorageKey returns `users/<userId>/avatar/<uuid>.<ext>`.
// Random UUID prevents CDN-cache hits after a reupload.
func buildAvatarStorageKey(userID, filename string) string {
	ext := strings.ToLower(path.Ext(filename))
	safe := make([]byte, 0, len(ext))
	for _, r := range ext {
		if r == '.' || (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9') {
			safe = append(safe, byte(r))
		}
	}
	if len(safe) == 0 || safe[0] != '.' {
		safe = append([]byte("."), safe...)
	}
	return fmt.Sprintf("users/%s/avatar/%s%s", userID, uuid.NewString(), string(safe))
}

// ─── MFA (proxy to auth-bff) ────────────────────────────────────────

// EnableMFA handles POST /admin/account/mfa/enable — proxies to auth-bff.
func (h *AccountHandler) EnableMFA(c *gin.Context) {
	h.proxyToAuthBFF(c, "POST", "/api/v1/mfa/enable")
}

// VerifyMFA handles POST /admin/account/mfa/verify — proxies to auth-bff.
// The body is { code: "123456" } and auth-bff flips enabled=true on a
// successful validation.
func (h *AccountHandler) VerifyMFA(c *gin.Context) {
	h.proxyToAuthBFF(c, "POST", "/api/v1/mfa/verify")
}

// DisableMFA handles POST /admin/account/mfa/disable — proxies to auth-bff.
func (h *AccountHandler) DisableMFA(c *gin.Context) {
	h.proxyToAuthBFF(c, "POST", "/api/v1/mfa/disable")
}

// ListSessions handles GET /admin/account/sessions — proxies to auth-bff.
func (h *AccountHandler) ListSessions(c *gin.Context) {
	h.proxyToAuthBFF(c, "GET", "/api/v1/sessions")
}

// RevokeSession handles DELETE /admin/account/sessions/:id — proxies to auth-bff.
func (h *AccountHandler) RevokeSession(c *gin.Context) {
	sessionID := c.Param("id")
	h.proxyToAuthBFF(c, "DELETE", fmt.Sprintf("/api/v1/sessions/%s", sessionID))
}

// ResetProfileConfirmation is the exact text the user must type in the
// Danger Zone input to trigger DELETE /admin/account. Scoped to a
// "reset" rather than a full tenant delete so the name accurately
// reflects the server-side behaviour: wipe the user's personal data
// and force sign-out. Store data is preserved.
const ResetProfileConfirmation = "reset my profile"

// DeleteAccount handles DELETE /admin/account — resets the caller's
// personal data: deletes the user_profile row, revokes every active
// session, and wipes MFA enrolment. Tenant + store data stays intact
// so the user can recover by signing back in. A future phase may add
// a true tenant cascade; until then this keeps the blast radius
// contained to the authenticated user.
func (h *AccountHandler) DeleteAccount(c *gin.Context) {
	userID := c.GetString("user_id")
	if userID == "" {
		RespondErr(c, apperrors.ValidationFailed("user_id", "missing user context"), h.logger)
		return
	}

	var body struct {
		Confirmation string `json:"confirmation"`
	}
	_ = c.ShouldBindJSON(&body)
	confirm := body.Confirmation
	if confirm == "" {
		confirm = c.GetHeader("X-Confirm-Delete")
	}
	if confirm != ResetProfileConfirmation {
		RespondErr(c, apperrors.ValidationFailed(
			"confirmation",
			fmt.Sprintf("confirmation text must be %q", ResetProfileConfirmation),
		), h.logger)
		return
	}

	// Best-effort: ask auth-bff to revoke every session and wipe MFA
	// for this user. We run this before the local profile delete so a
	// failure here aborts the whole operation — don't wipe profile
	// data if we can't also sign the user out.
	if err := h.cascadeAuthBFFReset(c.Request.Context(), userID); err != nil {
		h.logger.Error("delete account: auth-bff cascade", "err", err, "user_id", userID)
		RespondErr(c, fmt.Errorf("reset sessions: %w", err), h.logger)
		return
	}

	if err := h.profiles.Delete(c.Request.Context(), userID); err != nil {
		RespondErr(c, fmt.Errorf("delete profile: %w", err), h.logger)
		return
	}

	c.JSON(http.StatusOK, gin.H{"data": gin.H{"reset": true, "user_id": userID}})
}

// cascadeAuthBFFReset calls auth-bff's DELETE /internal/users/:id to
// revoke sessions + wipe MFA. Returns nil when the endpoint succeeds
// or when auth-bff / the internal secret is not wired (graceful
// degradation for dev environments). Any 4xx/5xx from auth-bff
// surfaces as an error so the caller can abort.
func (h *AccountHandler) cascadeAuthBFFReset(parent context.Context, userID string) error {
	if h.authBFFURL == "" || h.internalSecret == "" {
		return nil
	}
	ctx, cancel := context.WithTimeout(parent, 5*time.Second)
	defer cancel()
	req, err := http.NewRequestWithContext(
		ctx, http.MethodDelete,
		fmt.Sprintf("%s/internal/users/%s", h.authBFFURL, userID),
		nil,
	)
	if err != nil {
		return err
	}
	req.Header.Set("X-Internal-Auth", h.internalSecret)
	resp, err := h.httpClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 400 {
		return fmt.Errorf("auth-bff internal delete returned %d", resp.StatusCode)
	}
	return nil
}

// proxyToAuthBFF forwards the request to the auth-bff service.
func (h *AccountHandler) proxyToAuthBFF(c *gin.Context, method, path string) {
	if h.authBFFURL == "" {
		c.JSON(http.StatusServiceUnavailable, gin.H{
			"error":   "service_unavailable",
			"message": "auth-bff service not configured",
		})
		return
	}

	targetURL := h.authBFFURL + path

	req, err := http.NewRequestWithContext(c.Request.Context(), method, targetURL, c.Request.Body)
	if err != nil {
		h.logger.Error("failed to create proxy request", "url", targetURL, "err", err)
		c.JSON(http.StatusInternalServerError, gin.H{
			"error":   "internal",
			"message": "failed to proxy request",
		})
		return
	}

	req.Header.Set("Content-Type", c.GetHeader("Content-Type"))
	if userID := c.GetString("user_id"); userID != "" {
		req.Header.Set("X-User-ID", userID)
	}
	if tenantID := c.GetString("tenant_id"); tenantID != "" {
		req.Header.Set("X-Tenant-ID", tenantID)
	}
	// Forward email so auth-bff's MFA enrolment can label the QR with
	// the account name the user expects to see in their authenticator
	// app (e.g. "Mark8ly: alice@example.com") instead of falling back
	// to the opaque GIP sub. HeaderTrustAuth stores the email under
	// "user_email" (not "email"), so read that key or fall back to the
	// raw request header when it's set but the middleware didn't see
	// fit to promote it (shouldn't happen in prod).
	email := c.GetString("user_email")
	if email == "" {
		email = c.GetHeader("X-User-Email")
	}
	if email != "" {
		req.Header.Set("X-User-Email", email)
	}

	resp, err := h.httpClient.Do(req)
	if err != nil {
		h.logger.Error("auth-bff proxy failed", "url", targetURL, "err", err)
		c.JSON(http.StatusBadGateway, gin.H{
			"error":   "bad_gateway",
			"message": "auth-bff service unavailable",
		})
		return
	}
	defer resp.Body.Close()

	body, _ := io.ReadAll(resp.Body)
	c.Data(resp.StatusCode, resp.Header.Get("Content-Type"), body)
}

// parseStoreID is a helper to parse the storeId path parameter. Unused by
// account handler (operates outside store scope) but kept for consistency.
func parseStoreID(c *gin.Context) (uuid.UUID, error) {
	return uuid.Parse(c.Param("storeId"))
}

// fetchMFAEnabled asks auth-bff for the user's current MFA state.
// Best-effort — if auth-bff is unreachable or returns anything other
// than a clean 2xx we default to false so the UI doesn't lie about
// the user being protected. A 2-second timeout keeps the profile
// endpoint fast under partial outage.
func (h *AccountHandler) fetchMFAEnabled(parent context.Context, userID, email string) bool {
	if h.authBFFURL == "" {
		return false
	}
	ctx, cancel := context.WithTimeout(parent, 2*time.Second)
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, "GET", h.authBFFURL+"/api/v1/mfa/status", nil)
	if err != nil {
		return false
	}
	req.Header.Set("X-User-ID", userID)
	if email != "" {
		req.Header.Set("X-User-Email", email)
	}
	resp, err := h.httpClient.Do(req)
	if err != nil || resp == nil {
		return false
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return false
	}
	var body struct {
		Data struct {
			Enabled bool `json:"enabled"`
		} `json:"data"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
		return false
	}
	return body.Data.Enabled
}
