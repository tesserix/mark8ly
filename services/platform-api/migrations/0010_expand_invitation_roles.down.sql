ALTER TABLE invitations DROP CONSTRAINT IF EXISTS invitations_role_valid;
ALTER TABLE invitations ADD CONSTRAINT invitations_role_valid
    CHECK (role IN ('admin', 'staff', 'viewer'));
