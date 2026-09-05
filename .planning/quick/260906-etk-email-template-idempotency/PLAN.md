# Honour the Idempotency-Key on the email-template PUT

#730. tesserix-home's platform-api sends an `Idempotency-Key` on
`PUT /admin/email-templates/:key` and mark8ly ignores it.

Verified in the caller, not assumed:
`tesserix-home/platform-api/internal/platform/federation/client.go:203-207`
returns `ErrIdempotencyKeyRequired` before any HTTP call, so **every**
federated write already carries the header.

## Why a retry actually costs something

A template upsert is *nearly* idempotent by shape — the same body twice
yields the same subject, bodies and status. But `version` bumps on every
UPSERT and a revision row is appended per change, so a retried write inflates
the counter and records a change nobody made. On this surface the revision
trail is what stands in for an audit record, which is why a duplicate matters
more than the identical-looking row suggests.

## Shape: copy billing_trial_extend.go

It is the only route on this surface that honours a key today, and its
ordering comments record reasoning worth reusing rather than rediscovering:

- header checked FIRST, before the key or the body
- `Reserve` AFTER all validation and immediately before the write, so a
  malformed request cannot leave a key claimed with an empty response for the
  full TTL
- fail CLOSED on a reserve/lookup error — a caller who cannot be told whether
  the key was used must not reach a second UPSERT
- `!claimed` → `Lookup`; a hit replays the stored bytes, a miss is 409
  `in_progress`
- `Release` on failure so a corrected retry is not blocked until the TTL

## Two decisions

**The header is REQUIRED, not optional.** Matching trial-extend. Safe
because the only caller cannot omit it, and because the conformance suite
never probes this route — `email-templates` is not one of the seventeen
declarable ids, so no nightly run sends a PUT.

**`tenant_id` is the nil uuid.** `idempotency_keys.tenant_id` is NOT NULL
(migration 000001) and this registry is estate-wide — a template key belongs
to the product, not a tenant. The nil uuid is honest; borrowing an unrelated
id to satisfy the constraint is not.

## Tasks

1. `db *gorm.DB` on the handler and its constructor; `routes.go` passes
   `deps.DB`. Kept separate from `writable` because unit tests exercise a
   writable handler with no database.
2. Header check, scoped key `email_template_upsert:<key>:<header>`,
   Reserve/Lookup/Complete/Release around the existing write.
3. Marshal the response once so the stored replay body and the served body
   are the same bytes.

## Done when

- No header → 400 `idempotency_key_required`, and nothing is written.
- Same key twice → the second replays the stored body, with no version bump.
- A different key is a new write and does bump.
- A key does not replay across template keys.
- A nil-db handler still serves the write (the unit-test configuration).
