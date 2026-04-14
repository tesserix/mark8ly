package authz

// PageViewRole gates GET /admin/stores/:storeId/pages endpoints. Staff can view.
var PageViewRole = RoleStaff

// PageEditRole gates create/update/delete on /admin/stores/:storeId/pages. Admin required.
var PageEditRole = RoleAdmin
