-- Gap #6 — add gcs_path_original to product_media.
-- Backfill existing rows with storage_key (the uploaded content-addressed
-- key IS the original object before any recrop). New rows written by the
-- service layer set gcs_path_original explicitly.
ALTER TABLE product_media ADD COLUMN gcs_path_original text NOT NULL DEFAULT '';
UPDATE product_media SET gcs_path_original = storage_key WHERE gcs_path_original = '';
ALTER TABLE product_media ALTER COLUMN gcs_path_original DROP DEFAULT;
