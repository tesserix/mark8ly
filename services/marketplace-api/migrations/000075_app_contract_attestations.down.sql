-- 000075_app_contract_attestations.down.sql
-- Attestation records are legal-compliance artifacts. Rollback deliberately
-- DROPs the table rather than preserving orphan rows — we never want to lose
-- the schema on rollback and end up with rows that can't be read.

DROP TABLE IF EXISTS app_contract_attestations;
