BEGIN;

-- Nested replies: a customer or merchant can reply to a specific reply
-- (not just the top-level review). null parent_reply_id means the reply
-- is anchored directly to the review (the top-level comment). ON DELETE
-- CASCADE so deleting a parent removes the whole sub-thread atomically.
ALTER TABLE review_replies
    ADD COLUMN IF NOT EXISTS parent_reply_id UUID REFERENCES review_replies(id) ON DELETE CASCADE;

CREATE INDEX IF NOT EXISTS rr_parent_idx ON review_replies (parent_reply_id);

-- The existing author_type column is untyped; lock it to the two values
-- the storefront + admin paths actually use so a future bug can't slip
-- a third role in silently.
ALTER TABLE review_replies
    DROP CONSTRAINT IF EXISTS review_replies_author_type_valid;

ALTER TABLE review_replies
    ADD CONSTRAINT review_replies_author_type_valid
        CHECK (author_type IN ('customer', 'merchant'));

-- Widen the reaction enum for the storefront's three-way toggle
-- (helpful / not_helpful / useful). The UNIQUE(review_id, customer_profile_id)
-- constraint already enforces one active reaction per user per review;
-- switching reactions is an UPDATE, not a new row.
ALTER TABLE review_reactions
    DROP CONSTRAINT IF EXISTS review_reactions_reaction_valid;

ALTER TABLE review_reactions
    ADD CONSTRAINT review_reactions_reaction_valid
        CHECK (reaction IN ('helpful', 'not_helpful', 'useful'));

-- Denormalised counter for the new "useful" reaction, mirroring the
-- existing helpful_count / not_helpful_count columns. Kept in lockstep
-- by UpdateHelpfulCounts on every reaction write.
ALTER TABLE reviews
    ADD COLUMN IF NOT EXISTS useful_count INT NOT NULL DEFAULT 0;

COMMIT;
