// Package admin — order_personalisation.go: the merchant's access to what
// the buyer supplied (#968).
//
// With image-to-3D deferred, this IS the figurine feature: downloading
// the photograph and reading the text is the whole of what the merchant
// gets. An order whose artwork cannot be retrieved is worse than no
// feature, because the money has already been taken.
package admin

import (
	"encoding/json"
	"log/slog"
	"net/http"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"gorm.io/gorm"

	"github.com/mark8ly/marketplace-api/internal/audit"
	"github.com/mark8ly/marketplace-api/internal/media"
	"github.com/mark8ly/marketplace-api/internal/order"
	"github.com/mark8ly/marketplace-api/pkg/apperrors"
)

// DownloadURLTTL is how long an artwork link lives.
//
// Short, because it is a signed URL to a private object that is often a
// photograph of a person: it should stop working long before it could be
// forwarded, logged or pasted somewhere durable.
const DownloadURLTTL = 10 * time.Minute

// AdminPersonalisationResponse is one answer as the merchant sees it.
//
// Storage keys are NOT on the wire. The merchant gets a download link
// from the dedicated endpoint, which is audited; handing out the key
// would route around that and leak the bucket layout.
type AdminPersonalisationResponse struct {
	ID         string  `json:"id"`
	FieldKey   string  `json:"field_key"`
	FieldLabel string  `json:"field_label"`
	Kind       string  `json:"kind"`
	TextValue  *string `json:"text_value,omitempty"`
	PriceDelta string  `json:"price_delta"`
	Position   int     `json:"position"`

	// image only
	HasArtwork       bool    `json:"has_artwork"`
	OriginalFilename *string `json:"original_filename,omitempty"`
	ContentType      *string `json:"content_type,omitempty"`
	SizeBytes        *int64  `json:"size_bytes,omitempty"`
	// Reference is the short code printed on the packing slip, so a
	// merchant holding a paper slip can find the right file.
	Reference string `json:"reference"`
}

// ArtworkReference is the short code that ties a downloaded file to a
// line on a packing slip.
//
// The first eight characters of the personalisation id. Not a hash and
// not sequential: it has to be findable by someone reading a printed
// sheet, and long enough not to collide inside one order.
func ArtworkReference(id string) string {
	if len(id) >= 8 {
		return id[:8]
	}
	return id
}

// ToAdminPersonalisations maps the order's rows to the wire shape.
func ToAdminPersonalisations(rows []order.ItemPersonalisation) map[string][]AdminPersonalisationResponse {
	out := make(map[string][]AdminPersonalisationResponse)
	for i := range rows {
		p := &rows[i]
		out[p.OrderItemID.String()] = append(out[p.OrderItemID.String()], AdminPersonalisationResponse{
			ID:               p.ID.String(),
			FieldKey:         p.FieldKey,
			FieldLabel:       p.FieldLabel,
			Kind:             p.Kind,
			TextValue:        p.TextValue,
			PriceDelta:       p.PriceDelta.String(),
			Position:         p.Position,
			HasArtwork:       p.HasArtwork(),
			OriginalFilename: p.OriginalFilename,
			ContentType:      p.ContentType,
			SizeBytes:        p.SizeBytes,
			Reference:        ArtworkReference(p.ID.String()),
		})
	}
	return out
}

// OrderPersonalisationHandler serves artwork reads for one store.
type OrderPersonalisationHandler struct {
	db     *gorm.DB
	logger *slog.Logger
	// artwork signs reads against the PRIVATE bucket. Nil when no private
	// bucket is configured, which means no artwork can exist either.
	artwork media.SignedReadURLGenerator
	audit   *audit.Emitter
}

// NewOrderPersonalisationHandler constructs the handler.
func NewOrderPersonalisationHandler(
	db *gorm.DB, artwork media.SignedReadURLGenerator, logger *slog.Logger,
) *OrderPersonalisationHandler {
	return &OrderPersonalisationHandler{db: db, artwork: artwork, logger: logger}
}

// WithAudit attaches the audit emitter. Nil-safe.
func (h *OrderPersonalisationHandler) WithAudit(e *audit.Emitter) *OrderPersonalisationHandler {
	h.audit = e
	return h
}

type artworkLink struct {
	PersonalisationID string    `json:"personalisation_id"`
	Reference         string    `json:"reference"`
	Filename          string    `json:"filename"`
	URL               string    `json:"url"`
	ExpiresAt         time.Time `json:"expires_at"`
	// Crop is the rectangle the buyer chose, in ORIGINAL pixels.
	//
	// Returned ALONGSIDE the original rather than applied to it: this
	// service has no image library, and a server-side crop would be the
	// first place it needed one. The merchant's own tooling applies it at
	// full resolution, which is the point — the preview is lossy and must
	// never be the print source.
	Crop json.RawMessage `json:"crop,omitempty"`
}

// Download handles GET
// /admin/stores/:storeId/orders/:id/personalisations/:personalisationId/download.
//
// Returns a signed URL for the PRISTINE ORIGINAL. Never the preview —
// that is a browser-cropped, re-encoded thumbnail, and a merchant who
// prints from it ships a blurry figurine.
func (h *OrderPersonalisationHandler) Download(c *gin.Context) {
	storeID := c.Param("storeId")
	orderID := c.Param("id")

	if h.artwork == nil {
		c.AbortWithStatusJSON(http.StatusNotImplemented, gin.H{
			"error":   "not_implemented",
			"message": "artwork downloads require a private GCS bucket",
		})
		return
	}

	sid, oid, pid, ok := parseScopeIDs(c, storeID, orderID, c.Param("personalisationId"), h.logger)
	if !ok {
		return
	}

	row, err := order.GetPersonalisationForOrder(c.Request.Context(), h.db, sid, oid, pid)
	if err != nil {
		RespondErr(c, apperrors.NotFound("personalisation"), h.logger)
		return
	}
	if !row.HasArtwork() {
		RespondErr(c, apperrors.NotFound("artwork"), h.logger)
		return
	}

	link, err := h.signOne(c, row)
	if err != nil {
		RespondErr(c, err, h.logger)
		return
	}

	h.recordAccess(c, storeID, orderID, []string{row.ID.String()})
	c.JSON(http.StatusOK, link)
}

// DownloadAll handles GET
// /admin/stores/:storeId/orders/:id/personalisations/download.
//
// One call for the whole order, because a merchant batching a morning's
// figurines should not click through every line of every order. Returns
// links rather than a zip: zipping means streaming every object through
// this service, and the merchant's browser can fetch them directly.
func (h *OrderPersonalisationHandler) DownloadAll(c *gin.Context) {
	storeID := c.Param("storeId")
	orderID := c.Param("id")

	if h.artwork == nil {
		c.AbortWithStatusJSON(http.StatusNotImplemented, gin.H{
			"error":   "not_implemented",
			"message": "artwork downloads require a private GCS bucket",
		})
		return
	}

	sid, oid, _, ok := parseScopeIDs(c, storeID, orderID, uuid.Nil.String(), h.logger)
	if !ok {
		return
	}

	// Scoped by store first: an order id from another merchant must not
	// reach their artwork.
	var count int64
	if err := h.db.WithContext(c.Request.Context()).
		Table("orders").Where("id = ? AND store_id = ?", oid, sid).
		Count(&count).Error; err != nil || count == 0 {
		RespondErr(c, apperrors.NotFound("order"), h.logger)
		return
	}

	rows, err := order.ListPersonalisationsForOrder(c.Request.Context(), h.db, oid)
	if err != nil {
		RespondErr(c, err, h.logger)
		return
	}

	links := make([]artworkLink, 0, len(rows))
	ids := make([]string, 0, len(rows))
	for i := range rows {
		if !rows[i].HasArtwork() {
			continue
		}
		link, sErr := h.signOne(c, &rows[i])
		if sErr != nil {
			RespondErr(c, sErr, h.logger)
			return
		}
		links = append(links, *link)
		ids = append(ids, rows[i].ID.String())
	}

	if len(ids) > 0 {
		h.recordAccess(c, storeID, orderID, ids)
	}
	c.JSON(http.StatusOK, gin.H{"artwork": links})
}

func (h *OrderPersonalisationHandler) signOne(c *gin.Context, row *order.ItemPersonalisation) (*artworkLink, error) {
	url, expiresAt, err := h.artwork.SignedReadURL(
		c.Request.Context(), *row.StorageKeyOriginal, DownloadURLTTL)
	if err != nil {
		return nil, err
	}
	filename := "artwork"
	if row.OriginalFilename != nil && *row.OriginalFilename != "" {
		filename = *row.OriginalFilename
	}
	out := &artworkLink{
		PersonalisationID: row.ID.String(),
		Reference:         ArtworkReference(row.ID.String()),
		Filename:          filename,
		URL:               url,
		ExpiresAt:         expiresAt,
	}
	if row.Crop != nil {
		out.Crop = json.RawMessage(*row.Crop)
	}
	return out, nil
}

// recordAccess writes an audit row.
//
// Retrieving a buyer's photograph is an access event worth keeping: it is
// personal data, often a picture of a child, and "who looked at it and
// when" is the question a complaint starts with.
func (h *OrderPersonalisationHandler) recordAccess(c *gin.Context, _ string, orderID string, ids []string) {
	if h.audit == nil {
		return
	}
	// Emit reads tenant, store and actor from the gin context, which is
	// where HeaderTrustAuth and the :storeId param already put them.
	h.audit.Emit(c, audit.Event{
		Action:       "order.artwork.downloaded",
		ResourceType: "order",
		ResourceID:   orderID,
		Metadata: map[string]any{
			"personalisation_ids": ids,
			"count":               len(ids),
		},
	})
}

func parseScopeIDs(c *gin.Context, storeID, orderID, personalisationID string, logger *slog.Logger) (uuid.UUID, uuid.UUID, uuid.UUID, bool) {
	sid, sErr := uuid.Parse(storeID)
	oid, oErr := uuid.Parse(orderID)
	pid, pErr := uuid.Parse(personalisationID)
	if sErr != nil || oErr != nil || pErr != nil {
		RespondErr(c, apperrors.ValidationFailed("id", "malformed identifier"), logger)
		return uuid.Nil, uuid.Nil, uuid.Nil, false
	}
	return sid, oid, pid, true
}
