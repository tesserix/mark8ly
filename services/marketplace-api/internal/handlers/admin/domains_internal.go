package admin

// InternalDomainsHandler exposes domain re-verify + cert-refresh actions
// to super-admin callers (tesserix-home) over the in-cluster /internal
// namespace. Differs from DomainsHandler.Verify / .RefreshStatus in two
// ways:
//
//   1. URL takes only the domain ID (no :storeId) — the super-admin UI
//      doesn't carry a tenant-scoped session, so we resolve store_id
//      from the row before calling the existing domain.Service methods.
//   2. No tenant-relation auth check — istio AuthorizationPolicy on
//      mark8ly-marketplace-api-admin gates this to ns/tesserix/sa/company.
//
// Routes:
//   POST /internal/domains/:id/verify
//   POST /internal/domains/:id/refresh-status

import (
	"errors"
	"log/slog"
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"gorm.io/gorm"

	"github.com/mark8ly/marketplace-api/internal/domain"
)

type InternalDomainsHandler struct {
	svc    *domain.Service
	db     *gorm.DB
	logger *slog.Logger
}

func NewInternalDomainsHandler(svc *domain.Service, db *gorm.DB, logger *slog.Logger) *InternalDomainsHandler {
	return &InternalDomainsHandler{svc: svc, db: db, logger: logger}
}

// Register mounts the routes on the supplied /internal group. Caller is
// responsible for namespacing (e.g. r.Group("/internal")).
func (h *InternalDomainsHandler) Register(g *gin.RouterGroup) {
	g.POST("/domains/:id/verify", h.Verify)
	g.POST("/domains/:id/refresh-status", h.RefreshStatus)
}

// resolveStoreID looks up the row's store_id so we can call the existing
// service methods (which are scoped (storeID, id) for tenant isolation in
// the merchant-facing API). The internal endpoint sidesteps tenant-relation
// checks but still uses the same service path to avoid drift.
func (h *InternalDomainsHandler) resolveStoreID(c *gin.Context, id uuid.UUID) (uuid.UUID, bool) {
	var row struct {
		StoreID uuid.UUID `gorm:"column:store_id"`
	}
	err := h.db.WithContext(c.Request.Context()).
		Table("custom_domains").
		Select("store_id").
		Where("id = ?", id).
		Take(&row).Error
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			c.JSON(http.StatusNotFound, gin.H{"error": "domain_not_found"})
		} else {
			h.logger.Error("internal domains: lookup store_id", "id", id, "err", err)
			c.JSON(http.StatusInternalServerError, gin.H{"error": "lookup_failed"})
		}
		return uuid.Nil, false
	}
	return row.StoreID, true
}

func (h *InternalDomainsHandler) Verify(c *gin.Context) {
	id, err := uuid.Parse(c.Param("id"))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid_id"})
		return
	}
	storeID, ok := h.resolveStoreID(c, id)
	if !ok {
		return
	}
	d, err := h.svc.Verify(c.Request.Context(), storeID, id)
	if err != nil {
		RespondErr(c, err, h.logger)
		return
	}
	c.JSON(http.StatusOK, gin.H{"data": gin.H{
		"id":         d.ID.String(),
		"domain":     d.Domain,
		"status":     string(d.Status),
		"ssl_status": string(d.SSLStatus),
	}})
}

func (h *InternalDomainsHandler) RefreshStatus(c *gin.Context) {
	id, err := uuid.Parse(c.Param("id"))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid_id"})
		return
	}
	storeID, ok := h.resolveStoreID(c, id)
	if !ok {
		return
	}
	d, err := h.svc.RefreshCertStatus(c.Request.Context(), storeID, id)
	if err != nil {
		RespondErr(c, err, h.logger)
		return
	}
	c.JSON(http.StatusOK, gin.H{"data": gin.H{
		"id":          d.ID.String(),
		"domain":      d.Domain,
		"cert_status": string(d.CertStatus),
		"ssl_status":  string(d.SSLStatus),
	}})
}
