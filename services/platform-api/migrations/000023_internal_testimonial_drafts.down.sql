DELETE FROM merchant_testimonials WHERE store_id IS NULL;
DROP INDEX merchant_testimonials_unlinked_name_idx;
ALTER TABLE merchant_testimonials DROP CONSTRAINT merchant_testimonials_draft_check;
ALTER TABLE merchant_testimonials DROP CONSTRAINT merchant_testimonials_pkey;
ALTER TABLE merchant_testimonials DROP CONSTRAINT merchant_testimonials_store_id_key;
ALTER TABLE merchant_testimonials ALTER COLUMN store_id SET NOT NULL;
ALTER TABLE merchant_testimonials ADD PRIMARY KEY (store_id);
ALTER TABLE merchant_testimonials DROP COLUMN id;
ALTER TABLE merchant_testimonials DROP COLUMN store_name;
