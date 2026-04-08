-- Phase R — widen the invitations.role CHECK constraint to
-- include the store-level "manager" role. The Phase P migration
-- (0007) only allowed admin/staff/viewer because tenant-wide was
-- the only scope at the time. Phase R adds store-scoped invites
-- whose role values come from the store DSL (manager/staff/
-- viewer) and need to coexist with the tenant values.

ALTER TABLE invitations DROP CONSTRAINT IF EXISTS invitations_role_valid;

ALTER TABLE invitations ADD CONSTRAINT invitations_role_valid
    CHECK (role IN ('admin', 'manager', 'staff', 'viewer'));
