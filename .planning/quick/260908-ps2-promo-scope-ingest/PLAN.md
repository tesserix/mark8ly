---
id: 260908-ps2
slug: promo-scope-ingest
date: 2026-09-08
issue: 795
kind: quick
branch: feat/795-ingest-promo-scoping
---

# mark8ly honours the promo scope the console publishes (#795)

tesserix-home#593 shipped the authoring half: the console publishes
`allowed_plans` and `annual_only` on `/api/v1/promo-catalog`, and an operator can
scope a code to Pro-only or annual-only today. **mark8ly reads neither.** An
operator narrowing a code gets silence, and the code goes on applying to every
plan and both periods.

## Two things are wrong, not one — and the first is a proven bug

**1. `AllowedPlans` cannot be written at all.** `internal/promo/model.go:54`
declares `[]string` with `gorm:"type:text[];serializer:json"`. Verified against a
real Postgres 15 with the real model and the real table on 2026-09-08:

```
ERROR: malformed array literal: "["pro","studio"]" (SQLSTATE 22P02)
INSERT … "allowed_plans" … VALUES (…,'["pro","studio"]',…)
```

GORM marshals the slice to a JSON document and sends it as a text literal;
Postgres wants `{pro,studio}`. **The write fails outright.** It has never
surfaced because the column has only ever held NULL. `pq.StringArray` — what the
other five array columns in this service use — round-trips correctly, verified
the same way.

**2. The ingest deliberately preserves both columns.** `consolepromo/store.go`'s
`upsertColumns` omits them, and because it upserts assigning only listed
columns, a re-sync **preserves** whatever an existing row already holds. Its
comment says all four omitted columns are "mark8ly policy the console cannot
express (#726)". That was true when written and is now wrong for two of them.

## Decisions, settled — do not re-open these

1. **`allowed_plans` and `annual_only` become console-owned and JOIN
   `upsertColumns`.** A console publication is the last word, consistent with the
   reasoning already written for `valid_until` directly below it. Otherwise
   re-scoping an existing code silently never lands.
2. **`max_per_email` and `min_effective_price_per_currency` stay preserved.**
   They are abuse controls (§7.3, §7.4) the console deliberately cannot express —
   see tesserix-home#593 and `0051`'s header. The comment must be rewritten to
   describe TWO rules rather than one, or it will justify the wrong behaviour to
   whoever reads it next.
3. **An unknown plan is a `skipped` row with a reason, never a stored value.**
   The sync already reports `skipped_reasons`; use it. Storing an unrecognised
   plan would scope a code to nothing while reading as scoped.
4. **`null` means every plan; `{}` must never be written.** The redeemer guards
   on `len(AllowedPlans) > 0` (`internal/promo/validator.go:107`), so `{}` and
   `NULL` are the same fact to it — and the console refuses `{}` outright for
   that reason. One spelling crosses the boundary.

## THE LESSON THIS TASK EXISTS UNDER

The regression test for the model fix must assert **the stored representation**,
not that a write succeeded. `require.NoError` after the fix passes equally against
a future change that reintroduces a serializer; only asserting `{pro,studio}` in
the column pins the behaviour that was actually broken.

The same applies to the ingest: covering first-ingest alone would pass against
the current broken preservation. **The re-scope path is the test that matters** —
publish, ingest, re-scope, re-ingest, assert the row changed.

## Tasks

- **T1 — fix the model, with a regression test.** `pq.StringArray`, matching the
  other five array columns. An integration test that writes a scoped code, reads
  the raw column, and asserts `{pro,studio}` — plus the read-back through GORM.
- **T2 — ingest the two fields.** DTO in `consolepromo/catalog.go`, mapping in
  `mapper.go` with plan-vocabulary validation, both columns into `upsertColumns`,
  and that comment rewritten per decision 2. Cover the re-scope path.

## Done means

- [ ] A scoped code written through the model stores `{pro,studio}`, asserted on the column
- [ ] The console's `allowed_plans` / `annual_only` reach `promo_codes`
- [ ] Re-scoping a code the ingest has already seen CHANGES the stored row
- [ ] An unknown plan is skipped with a reason, not stored
- [ ] `max_per_email` and `min_effective_price_per_currency` are still preserved
- [ ] `upsertColumns`' comment describes two rules, not one
