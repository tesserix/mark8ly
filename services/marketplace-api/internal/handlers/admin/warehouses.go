// Package admin — warehouses.go: per-store warehouse CRUD, mounted at
// /admin/stores/:storeId/warehouses (#177 PR 5b).
//
// # Why this surface exists
//
// Until now a warehouse was a side effect. Saving a shipping carrier config
// upserted one behind the form, keyed on (store_id, name), so a merchant who
// typed a slightly different name got a SECOND, stockless warehouse instead
// of an edit — allocation then reported nothing available and the order never
// shipped. #505 and #508 made that visible; this makes the warehouse a thing
// the merchant manages directly, which is the actual fix.
//
// # One verb, not two
//
// The spec splits removal in half: a warehouse with any allocation history is
// archived (order_allocations.warehouse_id is ON DELETE RESTRICT, and the
// allocation row is the record of which warehouse shipped a line), and only a
// warehouse with no history at all can be deleted. That distinction is real
// but it is not the merchant's to make, so DELETE decides: it deletes when it
// can, archives when it must, and reports which happened.
package admin

import (
	"errors"
	"log/slog"
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"

	"github.com/mark8ly/marketplace-api/internal/warehouse"
	"github.com/mark8ly/marketplace-api/pkg/apperrors"
)

// WarehousesHandler serves the per-store warehouse endpoints.
type WarehousesHandler struct {
	db     *gorm.DB
	repo   *warehouse.Repository
	logger *slog.Logger
}

// NewWarehousesHandler constructs a WarehousesHandler.
func NewWarehousesHandler(db *gorm.DB, logger *slog.Logger) *WarehousesHandler {
	return &WarehousesHandler{db: db, repo: warehouse.NewRepository(), logger: logger}
}

// ─────────────────────────────────────────────────────────────────────────
// Wire types
// ─────────────────────────────────────────────────────────────────────────

// WarehouseResponse is one warehouse on the wire.
type WarehouseResponse struct {
	ID            string  `json:"id"`
	Name          string  `json:"name"`
	Line1         string  `json:"line1"`
	Line2         string  `json:"line2,omitempty"`
	City          string  `json:"city"`
	Region        string  `json:"region"`
	PostalCode    string  `json:"postal_code"`
	CountryCode   string  `json:"country_code"`
	Phone         string  `json:"phone"`
	Email         string  `json:"email,omitempty"`
	ContactPerson string  `json:"contact_person,omitempty"`
	IsDefault     bool    `json:"is_default"`
	Priority      int     `json:"priority"`
	ArchivedAt    *string `json:"archived_at,omitempty"`
}

// WarehouseWriteRequest is the create and update body. Deliberately one
// type: a create and an edit collect exactly the same fields, and two
// near-identical structs is how one of them ends up missing a rule.
type WarehouseWriteRequest struct {
	Name          string `json:"name"`
	Line1         string `json:"line1"`
	Line2         string `json:"line2"`
	City          string `json:"city"`
	Region        string `json:"region"`
	PostalCode    string `json:"postal_code"`
	CountryCode   string `json:"country_code"`
	Phone         string `json:"phone"`
	Email         string `json:"email"`
	ContactPerson string `json:"contact_person"`
}

// ReorderWarehousesRequest carries the FULL ordered set of live warehouse
// ids, not a delta — see Reorder.
type ReorderWarehousesRequest struct {
	Order []string `json:"order"`
}

func toWarehouseResponse(w warehouse.Warehouse) WarehouseResponse {
	out := WarehouseResponse{
		ID: w.ID, Name: w.Name, Line1: w.Line1, Line2: w.Line2,
		City: w.City, Region: w.Region, PostalCode: w.PostalCode,
		CountryCode: w.CountryCode, Phone: w.Phone, Email: w.Email,
		ContactPerson: w.ContactPerson, IsDefault: w.IsDefault, Priority: w.Priority,
	}
	if w.ArchivedAt != nil {
		s := w.ArchivedAt.UTC().Format("2006-01-02T15:04:05Z07:00")
		out.ArchivedAt = &s
	}
	return out
}

// ─────────────────────────────────────────────────────────────────────────
// Handlers
// ─────────────────────────────────────────────────────────────────────────

// List handles GET /admin/stores/:storeId/warehouses.
//
// Archived warehouses are excluded unless ?include_archived=true. They are
// excluded from the allocator's candidates too (checkout_stock.go) — a list
// that showed them while the allocator ignored them would be an oversell
// waiting to happen.
func (h *WarehousesHandler) List(c *gin.Context) {
	storeID := c.Param("storeId")
	includeArchived := c.Query("include_archived") == "true"

	rows, err := h.repo.List(c.Request.Context(), h.db, storeID, includeArchived)
	if err != nil {
		RespondErr(c, err, h.logger)
		return
	}
	out := make([]WarehouseResponse, 0, len(rows))
	for _, w := range rows {
		out = append(out, toWarehouseResponse(w))
	}
	c.JSON(http.StatusOK, gin.H{"data": out})
}

// Create handles POST /admin/stores/:storeId/warehouses.
func (h *WarehousesHandler) Create(c *gin.Context) {
	storeID := c.Param("storeId")
	tenantID := c.GetString("tenant_id")

	var req WarehouseWriteRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		RespondErr(c, apperrors.ValidationFailed("body", err.Error()), h.logger)
		return
	}
	if err := validateWarehouseWrite(req); err != nil {
		RespondErr(c, err, h.logger)
		return
	}

	w := applyWarehouseWrite(warehouse.Warehouse{TenantID: tenantID, StoreID: storeID}, req)
	created, err := h.repo.Create(c.Request.Context(), h.db, w)
	if err != nil {
		RespondErr(c, mapWarehouseErr(err, req.Name), h.logger)
		return
	}
	c.JSON(http.StatusCreated, gin.H{"data": toWarehouseResponse(created)})
}

// Update handles PATCH /admin/stores/:storeId/warehouses/:id.
//
// Keyed on the id, so a RENAME edits the row rather than forking a second
// one. That is the whole point of the slice — the carrier form's name-keyed
// upsert is what stranded stock on an orphan warehouse.
func (h *WarehousesHandler) Update(c *gin.Context) {
	storeID := c.Param("storeId")
	tenantID := c.GetString("tenant_id")

	var req WarehouseWriteRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		RespondErr(c, apperrors.ValidationFailed("body", err.Error()), h.logger)
		return
	}
	if err := validateWarehouseWrite(req); err != nil {
		RespondErr(c, err, h.logger)
		return
	}

	w := applyWarehouseWrite(warehouse.Warehouse{
		ID: c.Param("id"), TenantID: tenantID, StoreID: storeID,
	}, req)
	updated, err := h.repo.UpdateByID(c.Request.Context(), h.db, w)
	if err != nil {
		RespondErr(c, mapWarehouseErr(err, req.Name), h.logger)
		return
	}
	c.JSON(http.StatusOK, gin.H{"data": toWarehouseResponse(updated)})
}

// Delete handles DELETE /admin/stores/:storeId/warehouses/:id.
//
// Deletes when the warehouse has no allocation history and holds no stock;
// archives otherwise. The merchant asked for one thing — "remove this" — and
// gets one answer, with `outcome` saying which path ran so the UI can tell
// them the row is still on past orders.
//
// `units_remaining` is not decoration: archiving does NOT move stock, and
// those units stop being sellable the moment the allocator skips the row.
// The caller needs the number to warn before, or explain after.
func (h *WarehousesHandler) Delete(c *gin.Context) {
	ctx := c.Request.Context()
	storeID, id := c.Param("storeId"), c.Param("id")

	// Prove ownership first: Delete and Archive both take an id alone, and
	// an id is a guessable handle to another store's row.
	if _, err := h.repo.LiveForStore(ctx, h.db, id, storeID); err != nil {
		RespondErr(c, mapWarehouseErr(err, ""), h.logger)
		return
	}

	units, err := h.repo.UnitsHeld(ctx, h.db, id)
	if err != nil {
		RespondErr(c, err, h.logger)
		return
	}

	err = h.repo.Delete(ctx, h.db, id)
	switch {
	case err == nil:
		c.JSON(http.StatusOK, gin.H{"data": gin.H{
			"id": id, "outcome": "deleted", "units_remaining": 0,
		}})
		return
	case errors.Is(err, warehouse.ErrHasStock),
		errors.Is(err, warehouse.ErrHasUnshippedParcel),
		errors.Is(err, warehouse.ErrHasHistory):
		// Every refusal Delete can report is a reason to ARCHIVE, not a
		// reason to tell the merchant no. Refusing outright would leave a
		// stocked warehouse permanently unremovable.
		if archiveErr := h.repo.Archive(ctx, h.db, id); archiveErr != nil {
			RespondErr(c, mapWarehouseErr(archiveErr, ""), h.logger)
			return
		}
		c.JSON(http.StatusOK, gin.H{"data": gin.H{
			"id": id, "outcome": "archived", "reason": warehouseRemovalReason(err),
			"units_remaining": units,
		}})
		return
	default:
		RespondErr(c, mapWarehouseErr(err, ""), h.logger)
	}
}

// Reorder handles PUT /admin/stores/:storeId/warehouses/reorder.
//
// The body must be the store's COMPLETE live set. A delta applied to a list
// that changed underneath — a warehouse archived in another tab — reorders
// the wrong rows, and the caller would never know. Comparing against the
// live set turns that into a 400 instead of a silently wrong fill order.
func (h *WarehousesHandler) Reorder(c *gin.Context) {
	ctx := c.Request.Context()
	storeID := c.Param("storeId")

	var req ReorderWarehousesRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		RespondErr(c, apperrors.ValidationFailed("body", err.Error()), h.logger)
		return
	}

	live, err := h.repo.List(ctx, h.db, storeID, false)
	if err != nil {
		RespondErr(c, err, h.logger)
		return
	}
	if err := requireCompleteSet(req.Order, live); err != nil {
		RespondErr(c, err, h.logger)
		return
	}

	updates := make([]warehouse.PriorityUpdate, 0, len(req.Order))
	for i, id := range req.Order {
		updates = append(updates, warehouse.PriorityUpdate{ID: id, Priority: i})
	}
	if err := h.repo.SetPriorities(ctx, h.db, storeID, updates); err != nil {
		RespondErr(c, mapWarehouseErr(err, ""), h.logger)
		return
	}

	rows, err := h.repo.List(ctx, h.db, storeID, false)
	if err != nil {
		RespondErr(c, err, h.logger)
		return
	}
	out := make([]WarehouseResponse, 0, len(rows))
	for _, w := range rows {
		out = append(out, toWarehouseResponse(w))
	}
	c.JSON(http.StatusOK, gin.H{"data": out})
}

// SetDefault handles PUT /admin/stores/:storeId/warehouses/:id/default.
func (h *WarehousesHandler) SetDefault(c *gin.Context) {
	ctx := c.Request.Context()
	storeID, id := c.Param("storeId"), c.Param("id")

	if err := h.repo.SetDefault(ctx, h.db, storeID, id); err != nil {
		RespondErr(c, mapWarehouseErr(err, ""), h.logger)
		return
	}
	w, err := h.repo.LiveForStore(ctx, h.db, id, storeID)
	if err != nil {
		RespondErr(c, mapWarehouseErr(err, ""), h.logger)
		return
	}
	c.JSON(http.StatusOK, gin.H{"data": toWarehouseResponse(w)})
}

// ─────────────────────────────────────────────────────────────────────────
// Helpers
// ─────────────────────────────────────────────────────────────────────────

// validateWarehouseWrite enforces the fields a warehouse must carry to be
// usable for shipping.
//
// Phone is required and that is not bureaucracy: a warehouse saved without
// one made every ShipEngine quote fail with a bare "valid from zip code"
// error and no indication of the cause (#508, fixed in 5ed07a2b). The rule
// used to live in the carrier form's own validator; the address moves here,
// so the rule moves with it rather than being enforced in two places that
// drift.
func validateWarehouseWrite(req WarehouseWriteRequest) error {
	for _, f := range []struct {
		name, value string
	}{
		{"name", req.Name},
		{"line1", req.Line1},
		{"city", req.City},
		{"postal_code", req.PostalCode},
		{"country_code", req.CountryCode},
		{"phone", req.Phone},
	} {
		if trimmed(f.value) == "" {
			return apperrors.ValidationFailed(f.name, f.name+" is required")
		}
	}
	if len(trimmed(req.CountryCode)) != 2 {
		return apperrors.ValidationFailed("country_code", "country_code must be a 2-letter ISO code")
	}
	return nil
}

func applyWarehouseWrite(w warehouse.Warehouse, req WarehouseWriteRequest) warehouse.Warehouse {
	w.Name = trimmed(req.Name)
	w.Line1 = trimmed(req.Line1)
	w.Line2 = trimmed(req.Line2)
	w.City = trimmed(req.City)
	w.Region = trimmed(req.Region)
	w.PostalCode = trimmed(req.PostalCode)
	w.CountryCode = trimmed(req.CountryCode)
	w.Phone = trimmed(req.Phone)
	w.Email = trimmed(req.Email)
	w.ContactPerson = trimmed(req.ContactPerson)
	return w
}

// requireCompleteSet checks that ids is exactly the live set, once each.
func requireCompleteSet(ids []string, live []warehouse.Warehouse) error {
	if len(ids) != len(live) {
		return apperrors.ValidationFailed("order",
			"order must list every live warehouse exactly once")
	}
	want := make(map[string]struct{}, len(live))
	for _, w := range live {
		want[w.ID] = struct{}{}
	}
	seen := make(map[string]struct{}, len(ids))
	for _, id := range ids {
		if _, ok := want[id]; !ok {
			return apperrors.ValidationFailed("order", "unknown warehouse in order: "+id)
		}
		if _, dup := seen[id]; dup {
			return apperrors.ValidationFailed("order", "duplicate warehouse in order: "+id)
		}
		seen[id] = struct{}{}
	}
	return nil
}

// warehouseRemovalReason names why a delete became an archive.
func warehouseRemovalReason(err error) string {
	switch {
	case errors.Is(err, warehouse.ErrHasStock):
		return "holds_stock"
	case errors.Is(err, warehouse.ErrHasUnshippedParcel):
		return "unshipped_parcel"
	default:
		return "allocation_history"
	}
}

// mapWarehouseErr turns repository sentinels into wire errors. Anything
// unrecognised falls through untouched so RespondErr logs and 500s it,
// rather than being flattened into a misleading 404.
func mapWarehouseErr(err error, name string) error {
	switch {
	case errors.Is(err, warehouse.ErrNotFound):
		return apperrors.NotFound("warehouse")
	case errors.Is(err, warehouse.ErrNameTaken):
		return apperrors.WarehouseNameTaken(name)
	default:
		return err
	}
}

// trimmed is strings.TrimSpace, named for readability at the call sites
// above where every field goes through it.
func trimmed(s string) string { return strings.TrimSpace(s) }
