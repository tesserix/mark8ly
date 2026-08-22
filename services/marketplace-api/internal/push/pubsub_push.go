package push

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"log/slog"
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"google.golang.org/api/idtoken"

	"github.com/mark8ly/marketplace-api/internal/pushevents"
)

// OIDCVerifier validates a Google-signed ID token against an expected audience
// and returns the token's email claim. Swappable so tests don't need a live
// Google key set.
type OIDCVerifier func(ctx context.Context, token, audience string) (email string, err error)

func googleOIDCVerifier(ctx context.Context, token, audience string) (string, error) {
	payload, err := idtoken.Validate(ctx, token, audience)
	if err != nil {
		return "", err
	}
	email, _ := payload.Claims["email"].(string)
	return email, nil
}

// PubsubPushConfig configures the OIDC-gated merchant-push delivery endpoint.
type PubsubPushConfig struct {
	Sender *Sender
	Repo   *Repository
	Logger *slog.Logger
	// Audience is the OIDC aud claim the push subscription stamps (the public
	// endpoint URL). Empty skips the aud check — local/dev only, never prod.
	Audience string
	// ServiceAccount is the email the push subscription authenticates as.
	// Empty skips the email check.
	ServiceAccount string
	// Verify overrides the token validator (tests). Nil uses Google's.
	Verify OIDCVerifier
}

// NewPubsubPushHandler returns the Gin handler for the public, OIDC-authenticated
// Pub/Sub push subscription. Unlike the older cluster-internal webhook, this is
// safe to expose publicly: it rejects any request not bearing a valid OIDC token
// from our push service account before doing any work. The message already
// carries built content (title/body/deep_link), so the handler just fans it out
// to the store's admin devices via the Expo Push API.
func NewPubsubPushHandler(cfg PubsubPushConfig) gin.HandlerFunc {
	verify := cfg.Verify
	if verify == nil {
		verify = googleOIDCVerifier
	}
	return func(c *gin.Context) {
		// 1) Authenticate the Pub/Sub push via its OIDC bearer token.
		authz := c.GetHeader("Authorization")
		token := strings.TrimPrefix(authz, "Bearer ")
		if token == "" || token == authz {
			c.JSON(http.StatusUnauthorized, gin.H{"error": "missing bearer token"})
			return
		}
		email, err := verify(c.Request.Context(), token, cfg.Audience)
		if err != nil {
			cfg.Logger.Warn("push oidc verify failed", "error", err)
			c.JSON(http.StatusUnauthorized, gin.H{"error": "invalid token"})
			return
		}
		if cfg.ServiceAccount != "" && email != cfg.ServiceAccount {
			cfg.Logger.Warn("push oidc principal mismatch", "email", email)
			c.JSON(http.StatusForbidden, gin.H{"error": "unauthorized principal"})
			return
		}

		// 2) Decode the Pub/Sub push envelope → merchant-push event.
		var msg struct {
			Message struct {
				Data string `json:"data"`
			} `json:"message"`
		}
		if err := c.ShouldBindJSON(&msg); err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": "invalid pubsub message"})
			return
		}
		decoded, err := base64.StdEncoding.DecodeString(msg.Message.Data)
		if err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": "invalid base64 data"})
			return
		}
		var ev pushevents.Event
		if err := json.Unmarshal(decoded, &ev); err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": "invalid event payload"})
			return
		}
		storeID, err := uuid.Parse(ev.StoreID)
		if err != nil {
			// Bad producer data — ack so Pub/Sub stops redelivering.
			c.JSON(http.StatusOK, gin.H{"status": "ignored", "reason": "invalid store_id"})
			return
		}

		// 3) Fan out to the store's admin devices.
		tokens, err := cfg.Repo.ListByStore(storeID)
		if err != nil {
			cfg.Logger.Error("push token lookup failed", "error", err, "store_id", storeID)
			c.JSON(http.StatusInternalServerError, gin.H{"error": "token lookup failed"})
			return
		}
		if len(tokens) == 0 {
			c.JSON(http.StatusOK, gin.H{"status": "no_tokens"})
			return
		}
		strs := make([]string, len(tokens))
		for i, t := range tokens {
			strs[i] = t.TokenStr
		}
		result, err := cfg.Sender.Send(strs, ev.Title, ev.Body, ev.DeepLink)
		if err != nil {
			cfg.Logger.Error("push send failed", "error", err, "store_id", storeID)
			c.JSON(http.StatusInternalServerError, gin.H{"error": "push send failed"})
			return
		}
		for _, stale := range result.StaleTokens {
			if delErr := cfg.Repo.DeleteByToken(stale); delErr != nil {
				cfg.Logger.Error("stale push token delete failed", "error", delErr)
			}
		}
		cfg.Logger.Info("merchant push delivered",
			"store_id", storeID, "type", ev.Type,
			"sent", len(strs), "stale_removed", len(result.StaleTokens))
		c.JSON(http.StatusOK, gin.H{"status": "sent", "count": len(strs)})
	}
}
