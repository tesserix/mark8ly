package authz

// CustomersViewRole gates GET /admin/customers. Staff can view.
var CustomersViewRole = RoleStaff

// CustomersEditRole gates PATCH tags/notes and POST block/unblock. Admin required.
var CustomersEditRole = RoleAdmin
