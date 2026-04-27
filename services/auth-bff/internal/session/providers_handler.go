package session

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"time"

	"github.com/gin-gonic/gin"
)

// LinkedProvider is a single auth method bound to a GIP user.
type LinkedProvider struct {
	ProviderID string `json:"provider_id"`     // "password" or "google.com" or "facebook.com" etc.
	Email      string `json:"email,omitempty"` // empty for password (mirrored to top-level email anyway)
}

type providersResponse struct {
	Providers []LinkedProvider `json:"providers"`
}

// gipLookupResponse mirrors the Identity Toolkit accounts:lookup response.
type gipLookupResponse struct {
	Users []struct {
		Email        string `json:"email"`
		PasswordHash string `json:"passwordHash"`
		ProviderUserInfo []struct {
			ProviderID string `json:"providerId"`
			Email      string `json:"email"`
		} `json:"providerUserInfo"`
	} `json:"users"`
}

// getMyProviders returns the linked providers for the current admin user.
// Reads the m8_session cookie, extracts the GIP UID, calls Identity
// Toolkit accounts:lookup against MP-Internal pool. Used by the
// /settings/security page in the admin app.
func (h *Handler) getMyProviders(c *gin.Context) {
	s, err := h.mgr.Read(c.Request)
	if err != nil || s == nil {
		c.JSON(http.StatusUnauthorized, gin.H{
			"error":   "no_session",
			"message": "no active admin session",
		})
		return
	}
	if h.gipAPIKey == "" || h.gipInternalTenantID == "" {
		c.JSON(http.StatusServiceUnavailable, gin.H{
			"error":   "gip_not_configured",
			"message": "providers lookup is not configured",
		})
		return
	}

	body := map[string]any{
		"localId":  []string{s.UID},
		"tenantId": h.gipInternalTenantID,
	}
	raw, err := json.Marshal(body)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"error":   "internal_error",
			"message": "failed to build lookup request",
		})
		return
	}

	apiURL := fmt.Sprintf(
		"https://identitytoolkit.googleapis.com/v1/accounts:lookup?key=%s",
		url.QueryEscape(h.gipAPIKey),
	)
	req, err := http.NewRequestWithContext(c.Request.Context(), "POST", apiURL, bytes.NewReader(raw))
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"error":   "internal_error",
			"message": "failed to build lookup request",
		})
		return
	}
	req.Header.Set("Content-Type", "application/json")

	client := &http.Client{Timeout: 5 * time.Second}
	res, err := client.Do(req)
	if err != nil {
		if h.logger != nil {
			h.logger.Warn("providers lookup: gip unreachable", "err", err)
		}
		c.JSON(http.StatusServiceUnavailable, gin.H{
			"error":   "gip_unreachable",
			"message": "could not reach identity provider",
		})
		return
	}
	defer res.Body.Close()
	respBody, err := io.ReadAll(res.Body)
	if err != nil {
		c.JSON(http.StatusBadGateway, gin.H{
			"error":   "gip_read_failed",
			"message": "could not read identity provider response",
		})
		return
	}
	if res.StatusCode != http.StatusOK {
		if h.logger != nil {
			h.logger.Warn("providers lookup: gip returned non-200",
				"status", res.StatusCode, "body", string(respBody))
		}
		c.JSON(http.StatusBadGateway, gin.H{
			"error":   "gip_error",
			"message": "identity provider returned an error",
		})
		return
	}

	var lookup gipLookupResponse
	if err := json.Unmarshal(respBody, &lookup); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"error":   "parse_failed",
			"message": "could not parse identity provider response",
		})
		return
	}
	if len(lookup.Users) == 0 {
		c.JSON(http.StatusNotFound, gin.H{
			"error":   "user_not_found",
			"message": "no provider records for the current user",
		})
		return
	}

	user := lookup.Users[0]
	providers := []LinkedProvider{}
	if user.PasswordHash != "" {
		providers = append(providers, LinkedProvider{ProviderID: "password", Email: user.Email})
	}
	for _, p := range user.ProviderUserInfo {
		providers = append(providers, LinkedProvider{ProviderID: p.ProviderID, Email: p.Email})
	}

	c.JSON(http.StatusOK, gin.H{"data": providersResponse{Providers: providers}})
}
