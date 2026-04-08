-- Records the GIP UID of the user who accepted each invitation.
--
-- Needed so admin Settings → Team can change a teammate's role
-- without a roundtrip to auth-bff to resolve email → UID: FGA tuples
-- are keyed by `user:<UID>`, so the role-change service needs the UID
-- to rewrite the tuple, and the invitations table is the cleanest
-- place to store it since we already have the row in hand at the
-- moment of accept.
--
-- Nullable: existing rows have no accepted_by_user_id yet. The role-
-- change endpoint falls back to "cannot change — re-invite required"
-- when this column is NULL, and every invitation accepted AFTER this
-- migration ships carries its UID.

ALTER TABLE invitations
  ADD COLUMN IF NOT EXISTS accepted_by_user_id VARCHAR(128);

-- Back-fill is intentionally skipped. For tenants with a single
-- pre-migration teammate, the owner can revoke+reinvite them as a
-- simple workaround. No data loss — the FGA tuple is still intact,
-- only the admin UI's role-change CTA is gated on the new column.
