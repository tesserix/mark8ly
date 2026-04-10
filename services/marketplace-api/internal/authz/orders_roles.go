package authz

// This file is the single source of truth for which Role is required by
// each orders / returns / abandoned-carts admin endpoint. M4 (HTTP
// handlers) imports these constants and passes them to
// Middleware.RequireTenantRelation so route registration stays declarative
// and the policy is reviewable in one place.
//
// Why a constants file instead of inlining RoleAdmin / RoleStaff at each
// route registration:
//
//   - Reading the role policy for orders requires reading exactly this
//     file, not grepping the routes table.
//   - Tightening or relaxing a policy ("downgrade refund from owner to
//     admin") is one edit here, not N edits across registrations.
//   - The tests in this package (middleware_test.go) already cover the
//     cross-tenant denial mechanism generically; this file does not need
//     its own integration tests.
//
// Marketplace-api never writes FGA tuples — all tuple writes happen in
// platform-api during onboarding and invitation accept (see client.go
// package comment). M3 adds NO new types to the FGA model; orders are
// authorized purely against tenant-level role tuples already written by
// platform-api.
//
// Spec reference: §5.2 (permission map), §13.1.1 (read-only client).

// -----------------------------------------------------------------------------
// Orders endpoints
// -----------------------------------------------------------------------------

// OrdersViewRole gates GET /admin/orders, GET /admin/orders/:id, and any
// other read-only orders endpoint. Staff is the lowest tier that has any
// admin-panel access in the role hierarchy.
var OrdersViewRole = RoleStaff

// OrdersEditRole gates non-financial mutating endpoints: confirm,
// fulfill, cancel, edit notes. Admin tier — staff cannot mutate orders.
var OrdersEditRole = RoleAdmin

// OrdersRefundRole gates refund and partial-refund endpoints. Same tier
// as edit in slice 1; the seam exists so a future policy change can
// promote refunds to owner-only without touching every refund route.
var OrdersRefundRole = RoleAdmin

// -----------------------------------------------------------------------------
// Returns endpoints
// -----------------------------------------------------------------------------

// ReturnsViewRole gates GET /admin/returns and GET /admin/returns/:id.
var ReturnsViewRole = RoleStaff

// ReturnsEditRole gates approve / reject / mark-received / mark-refunded.
// Admin tier — refunds are financial operations.
var ReturnsEditRole = RoleAdmin

// -----------------------------------------------------------------------------
// Abandoned carts endpoints
// -----------------------------------------------------------------------------

// AbandonedCartsViewRole gates GET /admin/abandoned-carts.
var AbandonedCartsViewRole = RoleStaff

// AbandonedCartsEditRole gates POST /admin/abandoned-carts/:id/recovery-email
// (and any future mutating action). Admin tier — sending customer-facing
// emails is not a staff-tier operation.
var AbandonedCartsEditRole = RoleAdmin
