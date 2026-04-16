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
	"gorm.io/gorm"

	"github.com/mark8ly/marketplace-api/internal/media"
	"github.com/mark8ly/marketplace-api/pkg/apperrors"
)

// userProfile is the GORM model for user_profiles. Keep in sync with
// migrations/000036_user_profiles.up.sql.
type userProfile struct {
	UserID      string    `gorm:"column:user_id;primaryKey"`
	Email       string    `gorm:"column:email"`
	DisplayName string    `gorm:"column:display_name"`
	Phone       string    `gorm:"column:phone"`
	AvatarURL   string    `gorm:"column:avatar_url"`
	CreatedAt   time.Time `gorm:"column:created_at"`
	UpdatedAt   time.Time `gorm:"column:updated_at"`
}

func (userProfile) TableName() string { return "user_profiles" }

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

func toProfileDTO(p userProfile, mfaEnabled bool) profileDTO {
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
	db         *gorm.DB
	uploader   media.Uploader
	authBFFURL string
	httpClient *http.Client
	logger     *slog.Logger
}

// NewAccountHandler constructs an AccountHandler. db is required.
// uploader is optional (nil disables avatar uploads).
func NewAccountHandler(db *gorm.DB, authBFFURL string, logger *slog.Logger) *AccountHandler {
	return &AccountHandler{
		db:         db,
		authBFFURL: strings.TrimRight(authBFFURL, "/"),
		httpClient: &http.Client{Timeout: 10 * time.Second},
		logger:     logger,
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
	email := c.GetString("email")
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
	email := c.GetString("email")
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
		if err := h.db.WithContext(c.Request.Context()).
			Model(&userProfile{}).
			Where("user_id = ?", userID).
			Updates(updates).Error; err != nil {
			h.logger.Error("update user profile", "user_id", userID, "err", err)
			RespondErr(c, fmt.Errorf("update profile: %w", err), h.logger)
			return
		}
		// re-load so response reflects the freshly persisted row
		if err := h.db.WithContext(c.Request.Context()).
			Where("user_id = ?", userID).
			First(&profile).Error; err != nil {
			RespondErr(c, fmt.Errorf("reload profile: %w", err), h.logger)
			return
		}
	}

	mfa := h.fetchMFAEnabled(c.Request.Context(), userID, email)
	c.JSON(http.StatusOK, gin.H{"data": toProfileDTO(profile, mfa)})
}

// loadOrSeed returns the user_profile row, inserting a skeleton row on
// first read so the rest of the admin UI can count on a stable shape.
func (h *AccountHandler) loadOrSeed(c *gin.Context, userID, email string) (userProfile, error) {
	var p userProfile
	err := h.db.WithContext(c.Request.Context()).
		Where("user_id = ?", userID).
		First(&p).Error
	if err == nil {
		// Keep email mirrored if it drifted (e.g. GIP change).
		if email != "" && p.Email != email {
			_ = h.db.WithContext(c.Request.Context()).
				Model(&userProfile{}).
				Where("user_id = ?", userID).
				Updates(map[string]any{"email": email, "updated_at": time.Now()}).Error
			p.Email = email
		}
		return p, nil
	}
	if !errors.Is(err, gorm.ErrRecordNotFound) {
		return userProfile{}, fmt.Errorf("load profile: %w", err)
	}
	now := time.Now()
	p = userProfile{
		UserID:    userID,
		Email:     email,
		CreatedAt: now,
		UpdatedAt: now,
	}
	if err := h.db.WithContext(c.Request.Context()).Create(&p).Error; err != nil {
		return userProfile{}, fmt.Errorf("seed profile: %w", err)
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

// DeleteAccount handles DELETE /admin/account — owner-only with confirmation.
func (h *AccountHandler) DeleteAccount(c *gin.Context) {
	userID := c.GetString("user_id")
	if userID == "" {
		RespondErr(c, apperrors.ValidationFailed("user_id", "missing user context"), h.logger)
		return
	}

	// Accept the confirmation either in the body (what the UI sends) or
	// the X-Confirm-Delete header (what the old API contract required).
	var body struct {
		Confirmation string `json:"confirmation"`
	}
	_ = c.ShouldBindJSON(&body)
	confirm := body.Confirmation
	if confirm == "" {
		confirm = c.GetHeader("X-Confirm-Delete")
	}
	if confirm != "delete my store" {
		RespondErr(c, apperrors.ValidationFailed("confirmation", "confirmation text does not match"), h.logger)
		return
	}

	// Real cascade-delete is a dedicated phase (tenant cleanup,
	// OpenFGA tuples, GIP user removal). Until then this just wipes the
	// local user_profile row so the UI stops showing stale data.
	if err := h.db.WithContext(c.Request.Context()).
		Where("user_id = ?", userID).
		Delete(&userProfile{}).Error; err != nil {
		RespondErr(c, fmt.Errorf("delete profile: %w", err), h.logger)
		return
	}

	c.JSON(http.StatusOK, gin.H{"data": gin.H{"deleted": true, "user_id": userID}})
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
	// to the opaque GIP sub.
	if email := c.GetString("email"); email != "" {
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
