-- 000143_product_description_plain_text.down.sql
--
-- Intentionally a no-op. The up migration decodes entities and strips
-- tags; the original HTML cannot be reconstructed, and plain text is a
-- valid description under either policy version.
SELECT 1;
