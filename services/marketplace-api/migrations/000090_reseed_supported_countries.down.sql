-- 000090_reseed_supported_countries.down.sql
-- No-op. The up migration only reasserts seed rows that were originally
-- introduced by migration 000008. Rolling back to 89 should not delete
-- the catalogue — those rows belong to migration 000008's lifecycle.
SELECT 1;
