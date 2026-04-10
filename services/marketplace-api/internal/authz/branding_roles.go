package authz

// BrandingViewRole gates GET /admin/stores/:storeId/branding. Staff can view.
var BrandingViewRole = RoleStaff

// BrandingEditRole gates PUT /admin/stores/:storeId/branding. Admin required.
var BrandingEditRole = RoleAdmin
