-- 000043_business_entity_attestations.down.sql
DROP TRIGGER IF EXISTS business_entity_no_update ON business_entity_attestations;
DROP FUNCTION IF EXISTS raise_immutable_exception();
DROP TABLE IF EXISTS business_entity_attestations;
