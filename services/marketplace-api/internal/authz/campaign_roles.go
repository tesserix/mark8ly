package authz

// Marketing M4 — Campaign role policy.
// Same approach as orders_roles.go and loyalty_roles.go.

// CampaignsViewRole gates GET endpoints for campaigns and segments.
var CampaignsViewRole = RoleStaff

// CampaignsEditRole gates POST/PATCH/DELETE and send/schedule/pause/resume.
var CampaignsEditRole = RoleAdmin
