package push

import (
	"encoding/base64"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
)

// pubsubMessage is the envelope Google Pub/Sub sends to push endpoints.
type pubsubMessage struct {
	Message struct {
		Data string `json:"data"`
	} `json:"message"`
}

// eventPayload is the decoded event data from Pub/Sub.
type eventPayload struct {
	Type    string `json:"type"`
	StoreID string `json:"store_id"`
	Data    struct {
		OrderNumber   string  `json:"order_number,omitempty"`
		OrderID       string  `json:"order_id,omitempty"`
		CustomerEmail string  `json:"customer_email,omitempty"`
		GrandTotal    float64 `json:"grand_total,omitempty"`
		ProductName   string  `json:"product_name,omitempty"`
		ProductID     string  `json:"product_id,omitempty"`
		Stock         int     `json:"stock,omitempty"`
	} `json:"data"`
}

// NewWebhookHandler creates a Gin handler for Pub/Sub push subscription messages.
// It routes events to push notifications and cleans up stale tokens.
func NewWebhookHandler(sender *Sender, tokenRepo *Repository, logger *slog.Logger) gin.HandlerFunc {
	return func(c *gin.Context) {
		var msg pubsubMessage
		if err := c.ShouldBindJSON(&msg); err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": "invalid pubsub message"})
			return
		}

		decoded, err := base64.StdEncoding.DecodeString(msg.Message.Data)
		if err != nil {
			logger.Error("failed to decode pubsub message data", "error", err)
			c.JSON(http.StatusBadRequest, gin.H{"error": "invalid base64 data"})
			return
		}

		var event eventPayload
		if err := json.Unmarshal(decoded, &event); err != nil {
			logger.Error("failed to unmarshal event payload", "error", err)
			c.JSON(http.StatusBadRequest, gin.H{"error": "invalid event payload"})
			return
		}

		storeID, err := uuid.Parse(event.StoreID)
		if err != nil {
			logger.Error("invalid store_id in event", "store_id", event.StoreID)
			c.JSON(http.StatusOK, gin.H{"status": "ignored", "reason": "invalid store_id"})
			return
		}

		// Look up push tokens for this store.
		tokens, err := tokenRepo.ListByStore(storeID)
		if err != nil {
			logger.Error("failed to list push tokens", "error", err, "store_id", storeID)
			c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to list tokens"})
			return
		}

		if len(tokens) == 0 {
			c.JSON(http.StatusOK, gin.H{"status": "no_tokens"})
			return
		}

		// Build notification content based on event type.
		var title, body, deepLink string
		switch event.Type {
		case "order.created":
			title = "New order #" + event.Data.OrderNumber
			body = event.Data.CustomerEmail + " — $" + formatAmount(event.Data.GrandTotal)
			deepLink = "/orders/" + event.Data.OrderID
		case "inventory.low_stock":
			title = "Low stock alert"
			body = event.Data.ProductName + " — " + formatStock(event.Data.Stock) + " remaining"
			deepLink = "/products/" + event.Data.ProductID
		case "order.cancelled":
			title = "Order cancelled"
			body = "#" + event.Data.OrderNumber + " cancelled by customer"
			deepLink = "/orders/" + event.Data.OrderID
		default:
			c.JSON(http.StatusOK, gin.H{"status": "ignored", "reason": "unhandled event type"})
			return
		}

		// Collect token strings.
		tokenStrs := make([]string, len(tokens))
		for i, t := range tokens {
			tokenStrs[i] = t.TokenStr
		}

		// Send push notifications.
		result, err := sender.Send(tokenStrs, title, body, deepLink)
		if err != nil {
			logger.Error("failed to send push notifications", "error", err, "store_id", storeID)
			c.JSON(http.StatusInternalServerError, gin.H{"error": "push send failed"})
			return
		}

		// Clean up stale tokens.
		for _, staleToken := range result.StaleTokens {
			if err := tokenRepo.DeleteByToken(staleToken); err != nil {
				logger.Error("failed to delete stale push token", "error", err, "token", staleToken[:20]+"...")
			}
		}

		logger.Info("push notifications sent",
			"store_id", storeID,
			"event_type", event.Type,
			"sent", len(tokenStrs),
			"stale_removed", len(result.StaleTokens),
		)

		c.JSON(http.StatusOK, gin.H{
			"status": "sent",
			"count":  len(tokenStrs),
			"stale":  len(result.StaleTokens),
		})
	}
}

func formatAmount(amount float64) string {
	return fmt.Sprintf("%.2f", amount)
}

func formatStock(stock int) string {
	return fmt.Sprintf("%d", stock)
}
