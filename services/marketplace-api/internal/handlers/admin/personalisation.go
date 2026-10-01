// Package admin — personalisation.go: the merchant's authoring surface
// for what a buyer may supply on a product (#962).
//
// Two layers of limit, deliberately split:
//
//   - The per-PLAN field count lives here, next to the subscription
//     lookup, exactly as the images-per-product cap does in media.go.
//     Enforced at creation only: a downgrade must not retroactively
//     break a product the merchant already sells.
//   - The absolute ceiling (MaxPersonalisationFieldsPerProduct) lives in
//     the service, because it holds even when the plan says Unlimited.
package admin

import (
	"fmt"
	"log/slog"
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"gorm.io/gorm"

	"github.com/mark8ly/marketplace-api/internal/plangate"
	"github.com/mark8ly/marketplace-api/internal/product"
	"github.com/mark8ly/marketplace-api/internal/subscription"
	"github.com/mark8ly/marketplace-api/pkg/apperrors"
)

// PersonalisationHandler bundles dependencies for the personalisation
// endpoints.
type PersonalisationHandler struct {
	svc    *product.Service
	logger *slog.Logger
	// resolver + subRepo + subDB are optional. When all three are wired,
	// Create enforces the per-plan field cap. Production must wire them;
	// tests may leave them nil, in which case only the service's absolute
	// ceiling applies.
	resolver *plangate.PlanResolver
	subRepo  subscription.Repository
	subDB    *gorm.DB
}

// NewPersonalisationHandler constructs a PersonalisationHandler.
func NewPersonalisationHandler(svc *product.Service, logger *slog.Logger) *PersonalisationHandler {
	return &PersonalisationHandler{svc: svc, logger: logger}
}

// SetPlanGate wires the plan resolver and subscription repository used to
// enforce the fields-per-product cap at Create time. Pass the same
// *gorm.DB the subscription repository was constructed against.
func (h *PersonalisationHandler) SetPlanGate(resolver *plangate.PlanResolver, subRepo subscription.Repository, db *gorm.DB) {
	h.resolver = resolver
	h.subRepo = subRepo
	h.subDB = db
}

// List handles GET /admin/stores/:storeId/products/:id/personalisation-fields.
func (h *PersonalisationHandler) List(c *gin.Context) {
	storeID := c.Param("storeId")
	fields, err := h.svc.ListPersonalisationFields(
		c.Request.Context(), c.Param("id"), storeID, c.GetString("tenant_id"))
	if err != nil {
		RespondErr(c, err, h.logger)
		return
	}
	c.JSON(http.StatusOK, gin.H{"fields": ToPersonalisationFieldResponses(fields)})
}

// Create handles POST /admin/stores/:storeId/products/:id/personalisation-fields.
func (h *PersonalisationHandler) Create(c *gin.Context) {
	storeID := c.Param("storeId")
	productID := c.Param("id")
	tenantID := c.GetString("tenant_id")

	var body CreatePersonalisationFieldBody
	if err := c.ShouldBindJSON(&body); err != nil {
		RespondErr(c, apperrors.ValidationFailed("body", err.Error()), h.logger)
		return
	}

	if err := h.enforceFieldCap(c, tenantID, storeID, productID); err != nil {
		return
	}

	field, err := h.svc.CreatePersonalisationField(c.Request.Context(), product.CreatePersonalisationFieldRequest{
		ProductID:        productID,
		StoreID:          storeID,
		TenantID:         tenantID,
		Key:              body.Key,
		Label:            body.Label,
		Kind:             body.Kind,
		Required:         body.Required,
		Position:         body.Position,
		HelpText:         body.HelpText,
		MaxLength:        body.MaxLength,
		MaxImages:        body.MaxImages,
		MinPx:            body.MinPx,
		MockupStorageKey: body.MockupStorageKey,
		PrintArea:        toServicePrintArea(body.PrintArea),
		PriceDelta:       body.PriceDelta,
		Options:          toServiceOptionSpecs(body.Options),
	})
	if err != nil {
		RespondErr(c, err, h.logger)
		return
	}
	c.JSON(http.StatusCreated, ToPersonalisationFieldResponse(field))
}

// Patch handles PATCH .../personalisation-fields/:fieldId.
func (h *PersonalisationHandler) Patch(c *gin.Context) {
	var body PatchPersonalisationFieldBody
	if err := c.ShouldBindJSON(&body); err != nil {
		RespondErr(c, apperrors.ValidationFailed("body", err.Error()), h.logger)
		return
	}
	storeID := c.Param("storeId")
	field, err := h.svc.UpdatePersonalisationField(c.Request.Context(), product.UpdatePersonalisationFieldRequest{
		FieldID:          c.Param("fieldId"),
		StoreID:          storeID,
		TenantID:         c.GetString("tenant_id"),
		Label:            body.Label,
		Required:         body.Required,
		Position:         body.Position,
		HelpText:         body.HelpText,
		MaxLength:        body.MaxLength,
		MaxImages:        body.MaxImages,
		MinPx:            body.MinPx,
		MockupStorageKey: body.MockupStorageKey,
		PrintArea:        toServicePrintArea(body.PrintArea),
		PriceDelta:       body.PriceDelta,
	})
	if err != nil {
		RespondErr(c, err, h.logger)
		return
	}
	c.JSON(http.StatusOK, ToPersonalisationFieldResponse(field))
}

// Delete handles DELETE .../personalisation-fields/:fieldId.
func (h *PersonalisationHandler) Delete(c *gin.Context) {
	storeID := c.Param("storeId")
	err := h.svc.DeletePersonalisationField(
		c.Request.Context(), c.Param("fieldId"), storeID, c.GetString("tenant_id"))
	if err != nil {
		RespondErr(c, err, h.logger)
		return
	}
	c.Status(http.StatusNoContent)
}

// AddOption handles POST .../personalisation-fields/:fieldId/options.
func (h *PersonalisationHandler) AddOption(c *gin.Context) {
	storeID := c.Param("storeId")
	var body PersonalisationOptionDTO
	if err := c.ShouldBindJSON(&body); err != nil {
		RespondErr(c, apperrors.ValidationFailed("body", err.Error()), h.logger)
		return
	}
	field, err := h.svc.AddPersonalisationOption(
		c.Request.Context(), c.Param("fieldId"), storeID, c.GetString("tenant_id"),
		product.PersonalisationOptionSpec{
			Value:      body.Value,
			Label:      body.Label,
			PriceDelta: body.PriceDelta,
			Position:   body.Position,
		})
	if err != nil {
		RespondErr(c, err, h.logger)
		return
	}
	c.JSON(http.StatusCreated, ToPersonalisationFieldResponse(field))
}

// PatchOption handles PATCH .../personalisation-fields/:fieldId/options/:optionId.
func (h *PersonalisationHandler) PatchOption(c *gin.Context) {
	storeID := c.Param("storeId")
	var body PatchPersonalisationOptionBody
	if err := c.ShouldBindJSON(&body); err != nil {
		RespondErr(c, apperrors.ValidationFailed("body", err.Error()), h.logger)
		return
	}
	spec := product.PersonalisationOptionSpec{}
	if body.Value != nil {
		spec.Value = *body.Value
	}
	if body.Label != nil {
		spec.Label = *body.Label
	}
	if body.Position != nil {
		spec.Position = *body.Position
	}
	// Presence, not zero-ness: a merchant setting a paid option back to
	// free sends 0, and that must reach the column.
	setDelta := body.PriceDelta != nil
	if setDelta {
		spec.PriceDelta = *body.PriceDelta
	}

	field, err := h.svc.UpdatePersonalisationOption(
		c.Request.Context(), c.Param("fieldId"), c.Param("optionId"),
		storeID, c.GetString("tenant_id"), spec, setDelta)
	if err != nil {
		RespondErr(c, err, h.logger)
		return
	}
	c.JSON(http.StatusOK, ToPersonalisationFieldResponse(field))
}

// DeleteOption handles DELETE .../personalisation-fields/:fieldId/options/:optionId.
func (h *PersonalisationHandler) DeleteOption(c *gin.Context) {
	storeID := c.Param("storeId")
	field, err := h.svc.DeletePersonalisationOption(
		c.Request.Context(), c.Param("fieldId"), c.Param("optionId"),
		storeID, c.GetString("tenant_id"))
	if err != nil {
		RespondErr(c, err, h.logger)
		return
	}
	c.JSON(http.StatusOK, ToPersonalisationFieldResponse(field))
}

// enforceFieldCap returns non-nil after it has already written the 403
// response and the handler should abort. Returns nil to continue. A no-op
// when the gate dependencies are not wired.
func (h *PersonalisationHandler) enforceFieldCap(c *gin.Context, tenantID, storeID, productID string) error {
	if h.resolver == nil || h.subRepo == nil || h.subDB == nil {
		return nil
	}
	tenantUUID, terr := uuid.Parse(tenantID)
	storeUUID, serr := uuid.Parse(storeID)
	if terr != nil || serr != nil {
		return nil // scope errors surface from the service layer
	}

	current, err := h.svc.CountPersonalisationFields(c.Request.Context(), productID, storeID, tenantID)
	if err != nil {
		// Let the service's Create return the real error (NotFound etc.)
		// rather than duplicating error-shape logic here.
		return nil
	}

	plan := subscription.PlanTrial
	if sub, err := h.subRepo.GetByStoreID(c.Request.Context(), h.subDB, tenantUUID, storeUUID); err == nil {
		plan = sub.Plan
	}
	// Fail-closed on a missing subscription row: the trial cap is the
	// conservative default, matching enforceImageCap.

	limit := plangate.Limit(plan, plangate.FeaturePersonalisationFields)
	if limit == plangate.Unlimited {
		return nil
	}
	if int(current) >= limit {
		respondFieldCap(c, plan, limit, int(current))
		return fmt.Errorf("plan gate: personalisation-fields cap reached")
	}
	return nil
}

func respondFieldCap(c *gin.Context, plan subscription.SubscriptionPlan, limit, current int) {
	c.AbortWithStatusJSON(http.StatusForbidden, gin.H{
		"error": "plan_limit_exceeded",
		"message": fmt.Sprintf(
			"This product is at its personalisation fields cap (%d). Upgrade your plan to add more.", limit),
		"feature": string(plangate.FeaturePersonalisationFields),
		"limit":   limit,
		"current": current,
		"plan":    string(plan),
	})
}
