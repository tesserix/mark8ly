package authz

// Settings S1-S5 role constants.

// Account — owner-only for delete, admin for profile changes.
var AccountViewRole = RoleStaff
var AccountEditRole = RoleAdmin
var AccountDeleteRole = RoleOwner

// Domains — admin can view, owner manages.
var DomainsViewRole = RoleAdmin
var DomainsEditRole = RoleOwner

// Subscription — admin can view, owner manages billing.
var SubscriptionViewRole = RoleAdmin
var SubscriptionEditRole = RoleOwner

// Audit logs — staff can view (read-only feature).
var AuditLogsViewRole = RoleStaff

// Notifications — staff can view/manage their own.
var NotificationsViewRole = RoleStaff
var NotificationsEditRole = RoleStaff
