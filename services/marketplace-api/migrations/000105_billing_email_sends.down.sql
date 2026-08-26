-- DESTRUCTIVE: drops every claim marker. Rolling back past 105 means the
-- next dunning or win-back run re-sends to every merchant currently inside
-- an eligibility window — real duplicate mail to real merchants, not just
-- lost bookkeeping.
DROP INDEX IF EXISTS billing_email_sends_sent_at_idx;
DROP TABLE IF EXISTS billing_email_sends;
