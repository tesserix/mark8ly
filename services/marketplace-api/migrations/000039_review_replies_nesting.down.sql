BEGIN;

ALTER TABLE review_reactions
    DROP CONSTRAINT IF EXISTS review_reactions_reaction_valid;

ALTER TABLE review_replies
    DROP CONSTRAINT IF EXISTS review_replies_author_type_valid;

DROP INDEX IF EXISTS rr_parent_idx;

ALTER TABLE review_replies
    DROP COLUMN IF EXISTS parent_reply_id;

ALTER TABLE reviews
    DROP COLUMN IF EXISTS useful_count;

COMMIT;
