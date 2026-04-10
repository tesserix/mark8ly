package admin

import (
	"log/slog"
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/mark8ly/marketplace-api/internal/push"
)

type PushTokenHandler struct {
	repo   *push.Repository
	logger *slog.Logger
}

func NewPushTokenHandler(repo *push.Repository, logger *slog.Logger) *PushTokenHandler {
	return &PushTokenHandler{repo: repo, logger: logger}
}

type registerPushTokenRequest struct {
	Token    string `json:"token" binding:"required"`
	Platform string `json:"platform" binding:"required,oneof=ios android"`
	DeviceID string `json:"device_id" binding:"required,max=100"`
}

func (h *PushTokenHandler) Register(c *gin.Context) {
	var req registerPushTokenRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "bad_request", "message": err.Error()})
		return
	}

	userID, _ := uuid.Parse(c.GetString("user_id"))
	tenantID, _ := uuid.Parse(c.GetString("tenant_id"))
	storeID, _ := uuid.Parse(c.Param("storeId"))

	token := &push.Token{
		TenantID: tenantID,
		StoreID:  storeID,
		UserID:   userID,
		DeviceID: req.DeviceID,
		TokenStr: req.Token,
		Platform: req.Platform,
	}

	if err := h.repo.Upsert(token); err != nil {
		h.logger.Error("push token upsert failed", "error", err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "internal", "message": "failed to register push token"})
		return
	}

	c.JSON(http.StatusOK, gin.H{"id": token.ID, "message": "registered"})
}

func (h *PushTokenHandler) Delete(c *gin.Context) {
	userID, _ := uuid.Parse(c.GetString("user_id"))
	tokenID, err := uuid.Parse(c.Param("tokenId"))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "bad_request", "message": "invalid token ID"})
		return
	}

	if err := h.repo.Delete(userID, tokenID); err != nil {
		h.logger.Error("push token delete failed", "error", err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "internal", "message": "failed to delete push token"})
		return
	}

	c.JSON(http.StatusOK, gin.H{"message": "deleted"})
}
