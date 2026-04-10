package authz

// ReviewsViewRole gates GET /admin/reviews. Staff can view.
var ReviewsViewRole = RoleStaff

// ReviewsEditRole gates approve/reject/featured/reply. Admin required.
var ReviewsEditRole = RoleAdmin
