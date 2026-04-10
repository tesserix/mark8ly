package authz

// Marketing M3 — Loyalty role policy.
// Same approach as orders_roles.go and marketing_roles.go.

// LoyaltyViewRole gates GET loyalty program config and member lists.
var LoyaltyViewRole = RoleStaff

// LoyaltyEditRole gates PUT program config and POST point adjustments.
var LoyaltyEditRole = RoleAdmin
