package authz

// Marketing M2 — Gift Cards role policy.
// Same approach as orders_roles.go.

// GiftCardsViewRole gates GET /admin/gift-cards. Staff can view.
var GiftCardsViewRole = RoleStaff

// GiftCardsEditRole gates POST /admin/gift-cards (issue). Admin can issue.
var GiftCardsEditRole = RoleAdmin
