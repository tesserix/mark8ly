-- 000074_shipping_zones_ie_nz_vn.down.sql
-- Remove the three v2-rollout rows but preserve the table: merchant-edited
-- rows may exist on rollback, and dropping the table would lose them.

DELETE FROM shipping_zones WHERE country_code IN ('IE', 'NZ', 'VN');
