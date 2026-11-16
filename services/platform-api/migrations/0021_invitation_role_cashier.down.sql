-- Cashier invitations would violate the narrower constraint; drop them first.
DELETE FROM invitations WHERE role = 'cashier';
ALTER TABLE invitations DROP CONSTRAINT IF EXISTS invitations_role_valid;
ALTER TABLE invitations ADD CONSTRAINT invitations_role_valid
    CHECK (role IN ('admin', 'manager', 'staff', 'viewer'));
