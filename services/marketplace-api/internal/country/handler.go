package country

import (
	"net/http"

	"github.com/gin-gonic/gin"
)

// Handler serves public reference data for supported countries.
type Handler struct {
	repo Repository
}

// NewHandler constructs a Handler.
func NewHandler(repo Repository) *Handler {
	return &Handler{repo: repo}
}

// countryResponse is the public DTO — intentionally a subset of
// SupportedCountry. Provider lists and tax details are admin-only.
type countryResponse struct {
	CountryCode  string `json:"country_code"`
	Name         string `json:"name"`
	CurrencyCode string `json:"currency_code"`
	Region       string `json:"region"`
}

// ListSupported handles GET /api/v1/public/supported-countries.
// No auth — public reference data for onboarding and storefront.
func (h *Handler) ListSupported(c *gin.Context) {
	rows, err := h.repo.ListActive(c.Request.Context())
	if err != nil {
		c.AbortWithStatusJSON(http.StatusInternalServerError, gin.H{
			"error": "internal", "message": "internal server error",
		})
		return
	}
	out := make([]countryResponse, 0, len(rows))
	for _, r := range rows {
		out = append(out, countryResponse{
			CountryCode:  r.CountryCode,
			Name:         r.Name,
			CurrencyCode: r.CurrencyCode,
			Region:       r.Region,
		})
	}
	c.JSON(http.StatusOK, gin.H{"data": out})
}
