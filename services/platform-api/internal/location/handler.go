package location

import (
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"

	apperrors "github.com/mark8ly/platform-api/pkg/errors"
)

// Handler is the HTTP layer for location endpoints.
//
// Handlers are intentionally thin: parse, delegate to service, format response.
// All business logic lives in Service. All errors flow through respondError
// for consistent error envelope shape.
type Handler struct {
	svc *Service
}

// NewHandler constructs a Handler.
func NewHandler(svc *Service) *Handler {
	return &Handler{svc: svc}
}

// Register mounts the location routes onto the given gin.RouterGroup.
// All routes are public (no auth) — reference data is non-sensitive.
func (h *Handler) Register(r *gin.RouterGroup) {
	loc := r.Group("/locations")
	{
		loc.GET("/countries", h.listCountries)
		loc.GET("/countries/:code", h.getCountry)
		loc.GET("/countries/:code/states", h.listStatesByCountry)
		loc.GET("/states/:id", h.getState)
		loc.GET("/currencies", h.listCurrencies)
		loc.GET("/currencies/:code", h.getCurrency)
		loc.GET("/timezones", h.listTimezones)
		// IANA timezone IDs contain '/' (e.g. "America/New_York"), so we use
		// Gin's catch-all to capture everything after the prefix. The handler
		// strips the leading slash that Gin includes.
		loc.GET("/timezones/*id", h.getTimezone)
	}
}

// listResponse wraps a slice of items in a `data` envelope.
// Single-item responses use the same envelope for consistency.
type listResponse[T any] struct {
	Data []T `json:"data"`
}

type itemResponse[T any] struct {
	Data T `json:"data"`
}

// nonNil ensures the JSON output is always `[]`, never `null`. The contract
// is that empty lists are `data: []` and clients can iterate without a
// null check.
func nonNil[T any](items []T) []T {
	if items == nil {
		return []T{}
	}
	return items
}

func (h *Handler) listCountries(c *gin.Context) {
	items, err := h.svc.ListCountries(c.Request.Context())
	if err != nil {
		respondError(c, err)
		return
	}
	c.JSON(http.StatusOK, listResponse[Country]{Data: nonNil(items)})
}

func (h *Handler) getCountry(c *gin.Context) {
	item, err := h.svc.GetCountry(c.Request.Context(), c.Param("code"))
	if err != nil {
		respondError(c, err)
		return
	}
	c.JSON(http.StatusOK, itemResponse[Country]{Data: *item})
}

func (h *Handler) listStatesByCountry(c *gin.Context) {
	items, err := h.svc.ListStatesByCountry(c.Request.Context(), c.Param("code"))
	if err != nil {
		respondError(c, err)
		return
	}
	c.JSON(http.StatusOK, listResponse[State]{Data: nonNil(items)})
}

func (h *Handler) getState(c *gin.Context) {
	item, err := h.svc.GetState(c.Request.Context(), c.Param("id"))
	if err != nil {
		respondError(c, err)
		return
	}
	c.JSON(http.StatusOK, itemResponse[State]{Data: *item})
}

func (h *Handler) listCurrencies(c *gin.Context) {
	items, err := h.svc.ListCurrencies(c.Request.Context())
	if err != nil {
		respondError(c, err)
		return
	}
	c.JSON(http.StatusOK, listResponse[Currency]{Data: nonNil(items)})
}

func (h *Handler) getCurrency(c *gin.Context) {
	item, err := h.svc.GetCurrency(c.Request.Context(), c.Param("code"))
	if err != nil {
		respondError(c, err)
		return
	}
	c.JSON(http.StatusOK, itemResponse[Currency]{Data: *item})
}

func (h *Handler) listTimezones(c *gin.Context) {
	items, err := h.svc.ListTimezones(c.Request.Context())
	if err != nil {
		respondError(c, err)
		return
	}
	c.JSON(http.StatusOK, listResponse[Timezone]{Data: nonNil(items)})
}

func (h *Handler) getTimezone(c *gin.Context) {
	// Gin's catch-all (`*id`) returns the captured path with a leading "/".
	// Strip it so the service sees the canonical IANA ID.
	id := strings.TrimPrefix(c.Param("id"), "/")
	item, err := h.svc.GetTimezone(c.Request.Context(), id)
	if err != nil {
		respondError(c, err)
		return
	}
	c.JSON(http.StatusOK, itemResponse[Timezone]{Data: *item})
}

// respondError converts an error into the standard JSON error envelope.
// AppError → use its code/message/status. Anything else → 500 + opaque message.
func respondError(c *gin.Context, err error) {
	if ae, ok := apperrors.As(err); ok {
		c.JSON(ae.Status, gin.H{
			"error":   ae.Code,
			"message": ae.Message,
		})
		return
	}
	c.JSON(http.StatusInternalServerError, gin.H{
		"error":   "internal_error",
		"message": "an unexpected error occurred",
	})
}
