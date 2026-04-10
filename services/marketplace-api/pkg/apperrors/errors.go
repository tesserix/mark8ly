// Package apperrors is the marketplace-api typed error envelope.
// Every error that escapes the service layer flows through *Error so
// that M5's HTTP handlers render a consistent JSON envelope
// ({"error": "<code>", "message": "...", "details": {...}}) without
// type-switching on driver-level errors. Codes match spec §13.4 + §14.13.
package apperrors

import (
	"errors"
	"fmt"
)

// Code is an enumerated string identifying a failure mode.
type Code string

const (
	CodeValidationFailed        Code = "validation_failed"
	CodeVariantMatrixMismatch   Code = "variant_matrix_mismatch"
	CodeTooManyOptions          Code = "too_many_options"
	CodeTooManyVariants         Code = "too_many_variants"
	CodeCurrencyMismatch        Code = "currency_mismatch"
	CodeHandleTaken             Code = "handle_taken"
	CodeSKUTaken                Code = "sku_taken"
	CodeSlugTaken               Code = "slug_taken"
	CodeCategoryNotEmpty        Code = "category_not_empty"
	CodeCategoryHasChildren     Code = "category_has_children"
	CodeTargetStoreInvalid      Code = "target_store_invalid"
	CodeUploadNotFound          Code = "upload_not_found"
	CodeForbidden               Code = "forbidden"
	CodeNotFound                Code = "not_found"
	CodePayloadTooLarge         Code = "payload_too_large"
	CodeUnsupportedMediaType    Code = "unsupported_media_type"
	CodeRateLimited             Code = "rate_limited"
	CodeCurrencyChangeForbidden Code = "currency_change_forbidden"
	CodeOptionValueInUse        Code = "option_value_in_use"

	// Orders slice 1 — added in Orders M2.
	CodeInvalidTransition       Code = "invalid_transition"
	CodeRefundExceedsTotal      Code = "refund_exceeds_total"
	CodeIdempotencyConflict     Code = "idempotency_conflict"
	CodeReturnItemsExceedOrdered Code = "return_items_exceed_ordered"
	CodeRecoveryTooRecent       Code = "recovery_too_recent"

	// Coupons M1.
	CodeCouponNotFound          Code = "coupon_not_found"
	CodeCouponExpired           Code = "coupon_expired"
	CodeCouponUsageLimitReached Code = "coupon_usage_limit_reached"
	CodeCouponInvalid           Code = "coupon_invalid"
	CodeCouponMinPurchaseNotMet Code = "coupon_min_purchase_not_met"

	// Gift cards — Marketing M2.
	CodeInsufficientGiftCardBalance Code = "insufficient_gift_card_balance"
	CodeGiftCardExpired             Code = "gift_card_expired"
	CodeGiftCardNotFound            Code = "gift_card_not_found"

	// Loyalty M3.
	CodeInsufficientLoyaltyPoints Code = "insufficient_loyalty_points"
	CodeLoyaltyNotEnrolled        Code = "loyalty_not_enrolled"

	// Campaign M4.
	CodeCampaignNotFound     Code = "campaign_not_found"
	CodeCampaignNotDraft     Code = "campaign_not_draft"
	CodeCampaignNotSending   Code = "campaign_not_sending"
	CodeCampaignNotPaused    Code = "campaign_not_paused"
	CodeSegmentNotFound      Code = "segment_not_found"
	CodeSegmentInvalidRules  Code = "segment_invalid_rules"
	CodeCampaignNoRecipients Code = "campaign_no_recipients"
	CodeCampaignSchedulePast Code = "campaign_schedule_past"
)

// Error is the marketplace-api envelope. Satisfies the error interface.
type Error struct {
	Code    Code           // typed code (stable wire contract)
	Message string         // human-readable, PII-free
	Details map[string]any // extra structured data rendered into details{}
	wrapped error          // underlying cause for errors.Is / %w
}

func (e *Error) Error() string {
	if e.wrapped != nil {
		return fmt.Sprintf("%s: %s: %v", e.Code, e.Message, e.wrapped)
	}
	return fmt.Sprintf("%s: %s", e.Code, e.Message)
}

func (e *Error) Unwrap() error { return e.wrapped }

// Sentinel values for errors.Is comparisons. The wrapped *Error does NOT
// equal the sentinel; Is() below does the code-level comparison.
var (
	ErrValidationFailed        = &Error{Code: CodeValidationFailed}
	ErrVariantMatrixMismatch   = &Error{Code: CodeVariantMatrixMismatch}
	ErrTooManyOptions          = &Error{Code: CodeTooManyOptions}
	ErrTooManyVariants         = &Error{Code: CodeTooManyVariants}
	ErrCurrencyMismatch        = &Error{Code: CodeCurrencyMismatch}
	ErrHandleTaken             = &Error{Code: CodeHandleTaken}
	ErrSKUTaken                = &Error{Code: CodeSKUTaken}
	ErrSlugTaken               = &Error{Code: CodeSlugTaken}
	ErrCategoryNotEmpty        = &Error{Code: CodeCategoryNotEmpty}
	ErrCategoryHasChildren     = &Error{Code: CodeCategoryHasChildren}
	ErrTargetStoreInvalid      = &Error{Code: CodeTargetStoreInvalid}
	ErrUploadNotFound          = &Error{Code: CodeUploadNotFound}
	ErrForbidden               = &Error{Code: CodeForbidden}
	ErrNotFound                = &Error{Code: CodeNotFound}
	ErrPayloadTooLarge         = &Error{Code: CodePayloadTooLarge}
	ErrUnsupportedMediaType    = &Error{Code: CodeUnsupportedMediaType}
	ErrRateLimited             = &Error{Code: CodeRateLimited}
	ErrCurrencyChangeForbidden = &Error{Code: CodeCurrencyChangeForbidden}
	ErrOptionValueInUse        = &Error{Code: CodeOptionValueInUse}

	// Orders slice 1 sentinels.
	ErrInvalidTransition        = &Error{Code: CodeInvalidTransition}
	ErrRefundExceedsTotal       = &Error{Code: CodeRefundExceedsTotal}
	ErrIdempotencyConflict      = &Error{Code: CodeIdempotencyConflict}
	ErrReturnItemsExceedOrdered = &Error{Code: CodeReturnItemsExceedOrdered}
	ErrRecoveryTooRecent        = &Error{Code: CodeRecoveryTooRecent}

	// Coupons M1 sentinels.
	ErrCouponNotFound          = &Error{Code: CodeCouponNotFound}
	ErrCouponExpired           = &Error{Code: CodeCouponExpired}
	ErrCouponUsageLimitReached = &Error{Code: CodeCouponUsageLimitReached}
	ErrCouponInvalid           = &Error{Code: CodeCouponInvalid}
	ErrCouponMinPurchaseNotMet = &Error{Code: CodeCouponMinPurchaseNotMet}

	// Gift card sentinels.
	ErrInsufficientGiftCardBalance = &Error{Code: CodeInsufficientGiftCardBalance}
	ErrGiftCardExpired             = &Error{Code: CodeGiftCardExpired}
	ErrGiftCardNotFound            = &Error{Code: CodeGiftCardNotFound}

	// Loyalty M3 sentinels.
	ErrInsufficientLoyaltyPoints = &Error{Code: CodeInsufficientLoyaltyPoints}
	ErrLoyaltyNotEnrolled        = &Error{Code: CodeLoyaltyNotEnrolled}

	// Campaign M4 sentinels.
	ErrCampaignNotFound     = &Error{Code: CodeCampaignNotFound}
	ErrCampaignNotDraft     = &Error{Code: CodeCampaignNotDraft}
	ErrCampaignNotSending   = &Error{Code: CodeCampaignNotSending}
	ErrCampaignNotPaused    = &Error{Code: CodeCampaignNotPaused}
	ErrSegmentNotFound      = &Error{Code: CodeSegmentNotFound}
	ErrSegmentInvalidRules  = &Error{Code: CodeSegmentInvalidRules}
	ErrCampaignNoRecipients = &Error{Code: CodeCampaignNoRecipients}
	ErrCampaignSchedulePast = &Error{Code: CodeCampaignSchedulePast}
)

// Is makes errors.Is(err, sentinel) match when the codes are equal,
// so callers can write `errors.Is(err, apperrors.ErrHandleTaken)` regardless
// of which constructor built the error.
func (e *Error) Is(target error) bool {
	var t *Error
	if !errors.As(target, &t) {
		return false
	}
	return e.Code == t.Code
}

// IsKnownCode reports whether the given code string is one of the
// enumerated codes. Used by tests to assert enumeration coverage.
func IsKnownCode(s string) bool {
	switch Code(s) {
	case CodeValidationFailed, CodeVariantMatrixMismatch, CodeTooManyOptions,
		CodeTooManyVariants, CodeCurrencyMismatch, CodeHandleTaken, CodeSKUTaken,
		CodeSlugTaken, CodeCategoryNotEmpty, CodeCategoryHasChildren,
		CodeTargetStoreInvalid, CodeUploadNotFound, CodeForbidden, CodeNotFound,
		CodePayloadTooLarge, CodeUnsupportedMediaType, CodeRateLimited,
		CodeCurrencyChangeForbidden, CodeOptionValueInUse,
		CodeInvalidTransition, CodeRefundExceedsTotal, CodeIdempotencyConflict,
		CodeReturnItemsExceedOrdered, CodeRecoveryTooRecent,
		CodeCouponNotFound, CodeCouponExpired, CodeCouponUsageLimitReached,
		CodeCouponInvalid, CodeCouponMinPurchaseNotMet,
		CodeInsufficientGiftCardBalance, CodeGiftCardExpired, CodeGiftCardNotFound,
		CodeInsufficientLoyaltyPoints, CodeLoyaltyNotEnrolled,
		CodeCampaignNotFound, CodeCampaignNotDraft, CodeCampaignNotSending,
		CodeCampaignNotPaused, CodeSegmentNotFound, CodeSegmentInvalidRules,
		CodeCampaignNoRecipients, CodeCampaignSchedulePast:
		return true
	}
	return false
}

// ---------- constructors ----------

func New(code Code, msg string) *Error { return &Error{Code: code, Message: msg} }

func Wrap(code Code, msg string, err error) *Error {
	return &Error{Code: code, Message: msg, wrapped: err}
}

func ValidationFailed(field, msg string) *Error {
	return &Error{Code: CodeValidationFailed, Message: msg,
		Details: map[string]any{"field": field}}
}

func HandleTaken(attempted, suggested string) *Error {
	return &Error{Code: CodeHandleTaken,
		Message: fmt.Sprintf("handle %q is already in use in this store", attempted),
		Details: map[string]any{"attempted": attempted, "suggested": suggested}}
}

func SKUTaken(sku string) *Error {
	return &Error{Code: CodeSKUTaken,
		Message: fmt.Sprintf("SKU %q is already in use in this store", sku),
		Details: map[string]any{"sku": sku}}
}

func SlugTaken(attempted, suggested string) *Error {
	return &Error{Code: CodeSlugTaken,
		Message: fmt.Sprintf("slug %q is already in use in this store", attempted),
		Details: map[string]any{"attempted": attempted, "suggested": suggested}}
}

func CategoryNotEmpty(productCount int64) *Error {
	return &Error{Code: CodeCategoryNotEmpty,
		Message: "category still has products and cannot be deleted",
		Details: map[string]any{"product_count": productCount}}
}

func CategoryHasChildren(childCount int64) *Error {
	return &Error{Code: CodeCategoryHasChildren,
		Message: "category has sub-categories and cannot be deleted",
		Details: map[string]any{"child_count": childCount}}
}

func VariantMatrixMismatch(expected, got int) *Error {
	return &Error{Code: CodeVariantMatrixMismatch,
		Message: "variant count does not match option-value product",
		Details: map[string]any{"expected": expected, "got": got}}
}

func TooManyOptions(got int) *Error {
	return &Error{Code: CodeTooManyOptions,
		Message: "a product may not have more than 3 option axes",
		Details: map[string]any{"got": got}}
}

func TooManyVariants(got int) *Error {
	return &Error{Code: CodeTooManyVariants,
		Message: "a product may not have more than 500 variants",
		Details: map[string]any{"got": got}}
}

func UploadNotFound(key string) *Error {
	return &Error{Code: CodeUploadNotFound,
		Message: "referenced upload was not found in storage",
		Details: map[string]any{"storage_key": key}}
}

func PayloadTooLarge(key string, size int64) *Error {
	return &Error{Code: CodePayloadTooLarge,
		Message: "uploaded object exceeds the maximum size",
		Details: map[string]any{"storage_key": key, "bytes": size}}
}

func UnsupportedMediaType(key, ct string) *Error {
	return &Error{Code: CodeUnsupportedMediaType,
		Message: "uploaded object has an unsupported content type",
		Details: map[string]any{"storage_key": key, "content_type": ct}}
}

func TargetStoreInvalid(storeID, reason string) *Error {
	return &Error{Code: CodeTargetStoreInvalid,
		Message: "copy target store is invalid",
		Details: map[string]any{"store_id": storeID, "reason": reason}}
}

func CurrencyChangeForbidden() *Error {
	return &Error{Code: CodeCurrencyChangeForbidden,
		Message: "changing currency is not supported in slice 1"}
}

func CurrencyMismatch(source, target string) *Error {
	return &Error{Code: CodeCurrencyMismatch,
		Message: "variant currency does not match store currency",
		Details: map[string]any{"source": source, "target": target}}
}

func NotFound(resource string) *Error {
	return &Error{Code: CodeNotFound,
		Message: fmt.Sprintf("%s not found", resource)}
}

func Forbidden() *Error { return &Error{Code: CodeForbidden, Message: "forbidden"} }

// OptionValueInUse is returned when an aggregate PATCH removes an
// option value that is still referenced by a surviving variant. The
// client is expected to drop orphaned variants in generateVariants()
// before sending the PATCH; hitting this error is a contract bug.
func OptionValueInUse(option, value string) *Error {
	return &Error{Code: CodeOptionValueInUse,
		Message: fmt.Sprintf("option value %q on %q is still referenced by a surviving variant", value, option),
		Details: map[string]any{"option": option, "value": value}}
}

// ---------- Orders slice 1 constructors ----------

// InvalidTransition is returned when a status change is requested from a
// state that does not legally transition to the target state. Used across
// OrderStatus, PaymentStatus, and FulfillmentStatus axes.
func InvalidTransition(axis, from, to string) *Error {
	return &Error{Code: CodeInvalidTransition,
		Message: fmt.Sprintf("%s cannot transition from %q to %q", axis, from, to),
		Details: map[string]any{"axis": axis, "from": from, "to": to}}
}

// RefundExceedsTotal is returned when a refund amount plus the existing
// refunded_amount would exceed grand_total. The atomic single-UPDATE in
// order.Service.RecordRefund is the authoritative guard; this error is
// returned when that UPDATE matches zero rows.
func RefundExceedsTotal(grandTotal, requested, alreadyRefunded string) *Error {
	return &Error{Code: CodeRefundExceedsTotal,
		Message: "refund amount would exceed the order grand total",
		Details: map[string]any{
			"grand_total":       grandTotal,
			"requested":         requested,
			"already_refunded":  alreadyRefunded,
		}}
}

// IdempotencyConflict is returned when a Create call reuses an existing
// (store_id, idempotency_key) pair with a different payload shape. Storefront
// clients that receive this should regenerate the key for the new request.
func IdempotencyConflict(key string) *Error {
	return &Error{Code: CodeIdempotencyConflict,
		Message: "idempotency key was previously used with a different payload",
		Details: map[string]any{"idempotency_key": key}}
}

// ReturnItemsExceedOrdered is returned when a return request asks for more
// of an item than the underlying order contained.
func ReturnItemsExceedOrdered(sku string, requested, ordered int) *Error {
	return &Error{Code: CodeReturnItemsExceedOrdered,
		Message: fmt.Sprintf("return requests %d of %q but only %d were ordered", requested, sku, ordered),
		Details: map[string]any{"sku": sku, "requested": requested, "ordered": ordered}}
}

// RecoveryTooRecent is returned when abandoned_cart.TriggerRecoveryEmail is
// called for a cart whose recovery_sent_at is inside the 24h dedup window.
func RecoveryTooRecent(lastSentAt string) *Error {
	return &Error{Code: CodeRecoveryTooRecent,
		Message: "recovery email was already sent within the last 24 hours",
		Details: map[string]any{"last_sent_at": lastSentAt}}
}

// ---------- Coupons M1 constructors ----------

func CouponNotFound(code string) *Error {
	return &Error{Code: CodeCouponNotFound,
		Message: fmt.Sprintf("coupon %q not found", code),
		Details: map[string]any{"code": code}}
}

func CouponExpired(code string) *Error {
	return &Error{Code: CodeCouponExpired,
		Message: fmt.Sprintf("coupon %q has expired", code),
		Details: map[string]any{"code": code}}
}

func CouponUsageLimitReached(code string, limit int) *Error {
	return &Error{Code: CodeCouponUsageLimitReached,
		Message: fmt.Sprintf("coupon %q has reached its usage limit", code),
		Details: map[string]any{"code": code, "usage_limit": limit}}
}

func CouponInvalid(reason string) *Error {
	return &Error{Code: CodeCouponInvalid,
		Message: reason}
}

func CouponMinPurchaseNotMet(code, minPurchase, subtotal string) *Error {
	return &Error{Code: CodeCouponMinPurchaseNotMet,
		Message: fmt.Sprintf("coupon %q requires a minimum purchase of %s (subtotal: %s)", code, minPurchase, subtotal),
		Details: map[string]any{"code": code, "min_purchase": minPurchase, "subtotal": subtotal}}
}

// ---------- Loyalty M3 constructors ----------

func InsufficientLoyaltyPoints(available, requested int) *Error {
	return &Error{Code: CodeInsufficientLoyaltyPoints,
		Message: "customer does not have enough loyalty points",
		Details: map[string]any{"available": available, "requested": requested}}
}

func LoyaltyNotEnrolled(email string) *Error {
	return &Error{Code: CodeLoyaltyNotEnrolled,
		Message: "customer is not enrolled in the loyalty program",
		Details: map[string]any{"email": email}}
}
