-- Allow cashier invitations (#1104). The API accepted the role but this
-- constraint did not, so every cashier invite failed on insert.
ALTER TABLE invitations DROP CONSTRAINT IF EXISTS invitations_role_valid;

ALTER TABLE invitations ADD CONSTRAINT invitations_role_valid
    CHECK (role IN ('admin', 'manager', 'staff', 'viewer', 'cashier'));
