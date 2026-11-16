UPDATE merchant_testimonials SET published = false, consented_at = NULL, updated_at = now() WHERE store_id IS NULL;
ALTER TABLE merchant_testimonials DROP CONSTRAINT merchant_testimonials_draft_check;
ALTER TABLE merchant_testimonials ADD CONSTRAINT merchant_testimonials_draft_check
    CHECK (store_id IS NOT NULL OR (
        store_name IS NOT NULL AND char_length(btrim(store_name)) BETWEEN 1 AND 200 AND NOT published
    ));
