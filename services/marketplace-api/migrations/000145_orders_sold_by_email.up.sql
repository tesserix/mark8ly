-- In-store sales (#1109): who rang the sale up, as the admin shows it. The
-- admin cannot resolve sold_by_user_id to a person, so keep the seller's
-- email at the time of the sale.
ALTER TABLE orders ADD COLUMN sold_by_email varchar(320);
