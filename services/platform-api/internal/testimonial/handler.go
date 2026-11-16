package testimonial

import (
	"context"
	"errors"
	"net/http"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"gorm.io/gorm"
)

type authorizer interface {
	CheckObject(context.Context, string, string, string, string) (bool, error)
}

// Handler serves merchant submissions and consented public testimonials.
type Handler struct {
	db  *gorm.DB
	fga authorizer
}

// NewHandler binds testimonials to the platform database and store authorizer.
func NewHandler(db *gorm.DB, fga authorizer) *Handler { return &Handler{db: db, fga: fga} }

// RegisterInternal must be mounted behind strict internal authentication.
func (h *Handler) RegisterInternal(g *gin.RouterGroup) {
	g.GET("/stores/:id/testimonial", h.authorize, h.get)
	g.PUT("/stores/:id/testimonial", h.authorize, h.save)
}

// RegisterPublic mounts the bounded, consented homepage feed.
func (h *Handler) RegisterPublic(g *gin.RouterGroup) {
	g.GET("/testimonials", h.list)
}

// Review is the editable submission returned only to its store's managers.
type Review struct {
	AuthorName string `json:"author_name"`
	Quote      string `json:"quote"`
	Rating     int    `json:"rating"`
	Published  bool   `json:"published"`
}

// PublicReview excludes internal identity and consent metadata.
type PublicReview struct {
	StoreID    string `json:"store_id"`
	StoreName  string `json:"store_name"`
	AuthorName string `json:"author_name"`
	Quote      string `json:"quote"`
	Rating     int    `json:"rating"`
}

func fail(c *gin.Context, status int, code, message string) {
	c.AbortWithStatusJSON(status, gin.H{"error": code, "message": message})
}

// X-User-ID is supplied by the authenticated admin BFF, behind strict internal auth.
func (h *Handler) authorize(c *gin.Context) {
	c.Header("Cache-Control", "no-store")
	if _, err := uuid.Parse(c.Param("id")); err != nil || c.GetHeader("X-User-ID") == "" {
		fail(c, http.StatusNotFound, "not_found", "Store not found.")
		return
	}
	if h.fga == nil {
		fail(c, http.StatusServiceUnavailable, "unavailable", "Authorization is unavailable.")
		return
	}
	ctx, cancel := context.WithTimeout(c.Request.Context(), 3*time.Second)
	defer cancel()
	allowed, err := h.fga.CheckObject(ctx, c.GetHeader("X-User-ID"), "can_edit_store_settings", "store", c.Param("id"))
	if err != nil {
		fail(c, http.StatusServiceUnavailable, "unavailable", "Authorization is unavailable.")
		return
	}
	if !allowed {
		fail(c, http.StatusNotFound, "not_found", "Store not found.")
		return
	}
	c.Next()
}

func (h *Handler) get(c *gin.Context) {
	ctx, cancel := context.WithTimeout(c.Request.Context(), 3*time.Second)
	defer cancel()
	var row Review
	err := h.db.WithContext(ctx).Table("merchant_testimonials").
		Select("author_name, quote, rating, published").Where("store_id = ?", c.Param("id")).Take(&row).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		c.JSON(http.StatusOK, gin.H{"data": nil})
		return
	}
	if err != nil {
		fail(c, 503, "unavailable", "Could not load your testimonial.")
		return
	}
	c.Header("Cache-Control", "no-store")
	c.JSON(http.StatusOK, gin.H{"data": row})
}

func (h *Handler) save(c *gin.Context) {
	var req struct {
		AuthorName string `json:"author_name"`
		Quote      string `json:"quote"`
		Rating     int    `json:"rating"`
		Publish    bool   `json:"publish"`
		Consent    bool   `json:"consent"`
	}
	c.Request.Body = http.MaxBytesReader(c.Writer, c.Request.Body, 8192)
	if err := c.ShouldBindJSON(&req); err != nil {
		fail(c, 400, "invalid_body", "Enter a name, review and rating.")
		return
	}
	req.AuthorName, req.Quote = strings.TrimSpace(req.AuthorName), strings.TrimSpace(req.Quote)
	if n := utf8.RuneCountInString(req.AuthorName); n < 1 || n > 100 {
		fail(c, 400, "invalid_name", "Your name must be between 1 and 100 characters.")
		return
	}
	if n := utf8.RuneCountInString(req.Quote); n < 1 || n > 1000 {
		fail(c, 400, "invalid_quote", "Your review must be between 1 and 1000 characters.")
		return
	}
	if req.Rating < 1 || req.Rating > 5 || (req.Publish && !req.Consent) {
		fail(c, 400, "invalid_review", "Choose 1–5 stars and agree before publishing.")
		return
	}
	ctx, cancel := context.WithTimeout(c.Request.Context(), 3*time.Second)
	defer cancel()
	result := h.db.WithContext(ctx).Exec(`
		INSERT INTO merchant_testimonials (store_id, author_name, quote, rating, published, consented_at, submitted_by)
		SELECT id, ?, ?, ?, ?, CASE WHEN ? THEN now() ELSE NULL END, ?
		FROM stores WHERE id = ? AND status = 'active'
		ON CONFLICT (store_id) DO UPDATE SET author_name = EXCLUDED.author_name,
		quote = EXCLUDED.quote, rating = EXCLUDED.rating, published = EXCLUDED.published,
		consented_at = EXCLUDED.consented_at, submitted_by = EXCLUDED.submitted_by, updated_at = now()`,
		req.AuthorName, req.Quote, req.Rating, req.Publish, req.Publish, c.GetHeader("X-User-ID"), c.Param("id"))
	if result.Error != nil {
		fail(c, 503, "unavailable", "Could not save your testimonial.")
		return
	}
	if result.RowsAffected == 0 {
		fail(c, 404, "not_found", "Store not found.")
		return
	}
	c.JSON(http.StatusOK, gin.H{"data": Review{req.AuthorName, req.Quote, req.Rating, req.Publish}})
}

func (h *Handler) list(c *gin.Context) {
	ctx, cancel := context.WithTimeout(c.Request.Context(), 3*time.Second)
	defer cancel()
	rows := []PublicReview{}
	err := h.db.WithContext(ctx).Raw(`SELECT r.store_id, s.name AS store_name, r.author_name, r.quote, r.rating
		FROM merchant_testimonials r JOIN stores s ON s.id = r.store_id JOIN tenants t ON t.id = s.tenant_id
		WHERE r.published AND r.consented_at IS NOT NULL AND s.status = 'active' AND t.status = 'active'
		ORDER BY r.updated_at DESC, r.store_id LIMIT 12`).Scan(&rows).Error
	if err != nil {
		fail(c, 503, "unavailable", "Testimonials are unavailable.")
		return
	}
	c.Header("Cache-Control", "no-store")
	c.JSON(http.StatusOK, gin.H{"data": rows})
}
