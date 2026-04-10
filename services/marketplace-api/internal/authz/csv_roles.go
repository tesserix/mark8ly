package authz

// This file defines the role gates for CSV import/export endpoints.
// Pattern matches internal/authz/orders_roles.go.

// CSVImportRole gates POST /admin/stores/:storeId/csv-imports (submit)
// and POST /admin/stores/:storeId/csv-imports/:id/cancel.
// Admin tier — bulk mutations are not a staff-tier operation.
var CSVImportRole = RoleAdmin

// CSVExportRole gates GET /admin/stores/:storeId/products/export.csv.
// Staff can view products, so they can export them.
var CSVExportRole = RoleStaff

// CSVImportViewRole gates GET /admin/stores/:storeId/csv-imports (list)
// and GET /admin/stores/:storeId/csv-imports/:id (status).
// Staff tier — same as product viewing.
var CSVImportViewRole = RoleStaff
