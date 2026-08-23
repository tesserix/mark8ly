package platformadmin

import (
	"context"
	"errors"
	"log/slog"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/gin-gonic/gin"

	"github.com/mark8ly/marketplace-api/internal/tenantdirectory"
)

// TenantDirectory is the subset of tenantdirectory.Client this handler needs.
// Declared here so the handler can be tested with a stub and so the transport
// package stays swappable.
type TenantDirectory interface {
	List(ctx context.Context, p tenantdirectory.ListParams) (*tenantdirectory.ListResult, error)
	Get(ctx context.Context, id string) (*tenantdirectory.TenantDetail, error)
}

// EntitiesTenantsHandler serves the platform console's tenant directory
// (#277). It owns the wire shape and nothing else — search, filtering and
// pagination all happen in platform-api, which owns the data.
type EntitiesTenantsHandler struct {
	dir    TenantDirectory
	logger *slog.Logger
}

// NewEntitiesTenantsHandler constructs the handler. logger may be nil.
func NewEntitiesTenantsHandler(dir TenantDirectory, logger *slog.Logger) *EntitiesTenantsHandler {
	return &EntitiesTenantsHandler{dir: dir, logger: logger}
}

// Register mounts both routes on the supplied group.
func (h *EntitiesTenantsHandler) Register(g *gin.RouterGroup) {
	g.GET("/admin/entities/tenants", h.list)
	g.GET("/admin/entities/tenants/:id", h.detail)
}

// tenantRow is the directory wire shape.
type tenantRow struct {
	ID         string `json:"id"`
	Name       string `json:"name"`
	OwnerEmail string `json:"owner_email"`
	Status     string `json:"status"`
	CreatedAt  string `json:"created_at"`
}

type tenantListResponse struct {
	Data       []tenantRow `json:"data"`
	Pagination pagination  `json:"pagination"`
}

type tenantStore struct {
	ID     string `json:"id"`
	Slug   string `json:"slug"`
	Name   string `json:"name"`
	Status string `json:"status"`
}

type tenantDetailRow struct {
	tenantRow
	StoreCount int           `json:"store_count"`
	Stores     []tenantStore `json:"stores"`
}

func (h *EntitiesTenantsHandler) list(c *gin.Context) {
	p := parseTenantParams(c)

	res, err := h.dir.List(c.Request.Context(), p)
	if err != nil {
		h.respondErr(c, err)
		return
	}

	// Allocate before appending: a nil slice marshals to {}, which defeats a
	// caller's `?? []` and crashes their page precisely when there is no data.
	rows := make([]tenantRow, 0, len(res.Tenants))
	for _, t := range res.Tenants {
		rows = append(rows, toTenantRow(t))
	}

	c.JSON(http.StatusOK, tenantListResponse{
		Data: rows,
		Pagination: pagination{
			Page:  max(res.Page, 1),
			Limit: res.Limit,
			Total: res.Total,
		},
	})
}

func (h *EntitiesTenantsHandler) detail(c *gin.Context) {
	t, err := h.dir.Get(c.Request.Context(), c.Param("id"))
	if err != nil {
		h.respondErr(c, err)
		return
	}

	stores := make([]tenantStore, 0, len(t.Stores))
	for _, s := range t.Stores {
		stores = append(stores, tenantStore{ID: s.ID, Slug: s.Slug, Name: s.Name, Status: s.Status})
	}

	c.JSON(http.StatusOK, gin.H{"data": tenantDetailRow{
		tenantRow:  toTenantRow(t.Tenant),
		StoreCount: len(stores),
		Stores:     stores,
	}})
}

func toTenantRow(t tenantdirectory.Tenant) tenantRow {
	return tenantRow{
		// Bare id. The platform API namespaces as <slug>:<id> on arrival;
		// prefixing here yields "mark8ly:mark8ly:...".
		ID:         t.ID,
		Name:       t.Name,
		OwnerEmail: t.OwnerEmail,
		Status:     t.Status,
		CreatedAt:  t.CreatedAt.UTC().Format(time.RFC3339),
	}
}

// respondErr maps client errors to the surface's stable codes.
//
// ErrUnavailable becomes 503, never an empty 200: an empty directory and an
// unreachable one are different answers, and a console operator shown "no
// tenants" would believe the first.
//
// A non-404 4xx from platform-api (e.g. 401/403 from a wrong shared secret)
// wraps neither sentinel, so it falls through to the default branch below and
// becomes 500. That is deliberate: such a response means mark8ly's own
// configuration is broken, not that the dependency is transiently down.
// Reporting 503 would suggest retrying, which never helps a wrong secret.
func (h *EntitiesTenantsHandler) respondErr(c *gin.Context, err error) {
	switch {
	case errors.Is(err, tenantdirectory.ErrNotFound):
		c.JSON(http.StatusNotFound, gin.H{
			"error": "not_found", "message": "tenant not found",
		})
	case errors.Is(err, tenantdirectory.ErrUnavailable):
		if h.logger != nil {
			h.logger.Error("tenant directory upstream unavailable", "err", err)
		}
		c.JSON(http.StatusServiceUnavailable, gin.H{
			"error": "upstream_unavailable", "message": "tenant directory is unavailable",
		})
	default:
		if h.logger != nil {
			h.logger.Error("tenant directory", "err", err)
		}
		c.JSON(http.StatusInternalServerError, gin.H{
			"error": "internal_error", "message": "could not read the tenant directory",
		})
	}
}

// parseTenantParams never errors. A missing parameter takes platform-api's
// default; an oversized limit is clamped there.
func parseTenantParams(c *gin.Context) tenantdirectory.ListParams {
	p := tenantdirectory.ListParams{
		Q:      strings.TrimSpace(c.Query("q")),
		Status: strings.TrimSpace(c.Query("status")),
	}
	if v := strings.TrimSpace(c.Query("page")); v != "" {
		if n, err := strconv.Atoi(v); err == nil && n > 0 {
			p.Page = n
		}
	}
	if v := strings.TrimSpace(c.Query("limit")); v != "" {
		if n, err := strconv.Atoi(v); err == nil && n > 0 {
			p.Limit = n
		}
	}
	if t, ok := parseTime(c.Query("created_from")); ok {
		p.CreatedFrom = t
	}
	if t, ok := parseTime(c.Query("created_to")); ok {
		p.CreatedTo = t
	}
	return p
}
