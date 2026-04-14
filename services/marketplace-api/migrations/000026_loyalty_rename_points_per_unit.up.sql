-- Migration 000026: rename loyalty_programs.points_per_dollar → points_per_unit.
-- Points are awarded per unit of the store's own currency, not per dollar.
ALTER TABLE loyalty_programs RENAME COLUMN points_per_dollar TO points_per_unit;
