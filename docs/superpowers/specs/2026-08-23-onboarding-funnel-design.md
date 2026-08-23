# Onboarding Funnel — Design

**Issue:** #283. Part of the platform console integration series (#260), after #274, #275, #276, #277, #279.

**Goal:** Expose mark8ly's onboarding funnel to the Tesserix platform console, so the estate-wide view of *lead → onboarding session → tenant* stops being split across three places with no endpoint over the middle one.

## The finding that shapes this design

`internal/onboarding/models.go` declares `StatusAbandoned` and `StatusExpired`, migration 0004 CHECK-constrains both, and the package doc comment states that a session "may transition to `status="abandoned"` (browser close) or `"expired"` (gc)".

**Neither value is ever written.** The repository exposes `Create`, `GetByID`, `UpdateDraft`, `MarkEmailVerified` and `CompleteInTx` — no status transition to either. There is no gc, cron, sweep or reaper for onboarding sessions anywhere in the workspace (verified by searching all Go, TypeScript and SQL outside tests). In production, every session is `in_progress`, `verifying` or `completed`.

Two consequences, and they are the whole reason this document exists:

1. **`abandoned` cannot be read from the status column.** It is derived from idle time.
2. **The package doc is wrong** and will mislead the next reader. Recorded as a follow-up issue rather than fixed here, to keep this change read-only.

`last_activity_at` is genuinely maintained — `UpdateDraft`, `MarkEmailVerified` and `CompleteInTx` all set it — and migration 0004 indexes it (`idx_onboarding_sessions_last_activity`). So deriving from it is both honest and cheap.

## Definitions

Fixed for this endpoint. The console must not be able to reach two different numbers for the same word.

- **Started** — a session row exists. Every session counts, whatever its status.
- **Email verified** — `email_verified_at IS NOT NULL`. A **subset counter that cuts across the others**, not a funnel stage: a verified session may be completed, in flight or abandoned. This is the single most misreadable field on the response and the spec says so out loud.
- **Completed** — `status = 'completed'`. `completed_at` is the timestamp.
- **Abandoned** — not completed, and `last_activity_at <= now() - 24 hours`.
- **In flight** — not completed, and `last_activity_at > now() - 24 hours`.

**The 24-hour cutoff is a product decision**, chosen because onboarding is normally one sitting and the only legitimate long pause is waiting on the verification email, which is minutes to hours. It is a named constant, not a literal scattered through queries.

**Boundary rule: exactly 24 hours idle is abandoned**, not in flight. Arbitrary, but stated, and pinned by a test — otherwise it is decided accidentally by whichever comparison operator someone typed.

### The partition invariant

    completed + in_flight + abandoned == started

Exact, for any window. This is acceptance criterion "counters and rows agree with each other for the same window" made checkable, and it is asserted directly by a test. `email_verified` is deliberately outside it.

## `GET /admin/onboarding/funnel`

```json
{"data": {
  "started": 412,
  "email_verified": 301,
  "completed": 188,
  "in_flight": 34,
  "abandoned": 190,
  "median_completion_seconds": 743,
  "last_24h": {"started": 12, "completed": 5},
  "window": {"from": "2026-08-01T00:00:00Z", "to": "2026-08-23T00:00:00Z"}
}}
```

- Single object. **No `pagination` key.**
- `median_completion_seconds` is `percentile_cont(0.5)` over `completed_at - created_at` for sessions completed in the window. **`null` when nothing completed** — never `0`, which reads as "instant".
- `last_24h` is always `now()-24h … now()` and **ignores the window**. It is a live pulse for the console header. Intersecting it with the window would return `0` for any historical window, which looks like a data problem rather than a definition.
- `window` echoes the effective window back, including the defaults, so the console can render what it actually got rather than what it thinks it asked for.
- Computed in **one query** with `FILTER (WHERE …)` aggregates. Five separate counts could observe five different database states and break the partition invariant for reasons unrelated to the data.

## `GET /admin/onboarding/sessions`

Standard envelope: `{"data": [...], "pagination": {"page","limit","total"}}`. Empty is `200` with `[]`.

Row shape:

```json
{
  "id": "<bare uuid>",
  "email": "founder@acme.example",
  "status": "in_progress",
  "created_at": "2026-08-22T10:00:00Z",
  "last_activity_at": "2026-08-22T10:14:00Z",
  "idle_hours": 31.4,
  "abandoned": true,
  "completed_at": null,
  "tenant_id": null
}
```

- `status` is the **stored** status, unmodified. `abandoned` is the **derived** flag. Keeping them as separate fields rather than overwriting `status` with a synthetic `"abandoned"` means the console can see that the stored status is `in_progress` while the flag says abandoned — which is the truth, and which is what makes the dead-status problem visible rather than hidden.
- `idle_hours` is a float, computed as `EXTRACT(EPOCH FROM now() - last_activity_at) / 3600`, so a caller can apply its own threshold without a second round trip.
- `tenant_id` is **bare** and `null` until completion.
- Ids bare, timestamps RFC3339 UTC with offset, no `source` field.

## Shared window and the anti-drift rule

Both endpoints accept `created_from` and `created_to` (RFC3339), matching the tenant directory's parameter names. `sessions` additionally accepts `status` and `abandoned=true|false`.

**The abandoned predicate is defined once in SQL and shared by both queries**, the way `applyDirectoryFilter` is shared by the directory's count and page queries. Two hand-written copies is exactly how the funnel's `abandoned` count and the rows' `abandoned` flags come to disagree — the failure the acceptance criterion is written to prevent.

Defaults follow the directory: missing parameter takes the default and never errors, `limit` clamps at 500, `pagination.limit` reports the **effective** (clamped) limit so `total / limit` is a correct page count.

## Architecture

Unchanged from #277/#279 — this is the third endpoint through the same path, and it should not invent a fourth shape.

| Layer | Responsibility |
|---|---|
| `platform-api` repository | Both queries. Owns the SQL, the shared predicate and the constant. |
| `platform-api` internal endpoints | `GET /internal/onboarding/funnel` and `/internal/onboarding/sessions`, on the **strict** auth group — both return estate-wide data, so an unconfigured deploy must refuse rather than serve the lot. |
| `marketplace-api` client | A new `onboardingfunnel` package, modelled on `tenantdirectory`. Separate package: a funnel read is a different concern from a tenant directory read and the two will diverge. |
| `marketplace-api` handler | Owns the wire shape. Projects field by field. |

**Projection, not passthrough.** The session row upstream carries `draft` — a JSONB blob of whatever the wizard has collected, which may hold business details and is not the console's business. It must never reach the console. The projection is what guarantees that, and a test asserts `draft` is absent from the response.

**Error semantics** inherit #279's: upstream unreachable or 5xx is `503 upstream_unavailable`, never an empty result, because "we could not ask" and "nothing there" are different answers. A non-404 4xx becomes `500` — that means our own configuration is broken and retrying never helps.

## Testing

Integration tests (platform-api, internal `package onboarding`, `//go:build integration`):

- Sessions seeded either side of the 24h boundary, **including one at exactly 24h idle**, pinning the boundary rule.
- The partition invariant asserted over a mixed fixture: completed + in flight + abandoned equals started, exactly.
- Median with an **even** and an **odd** number of completions (the even case is where a wrong percentile implementation shows up), and `null` with zero completions.
- The funnel's `abandoned` count equals the number of rows the sessions endpoint flags abandoned over the same window — the two must be computed from the shared predicate and this test is what proves they are.
- Window filtering: a session outside the window appears in neither.
- `last_24h` unaffected by a historical window.

Handler tests (marketplace-api, external `package platformadmin_test`):

- Golden fixture per endpoint, each proven by mutation against a field **rename** and a field **addition**.
- `draft` absent from a session row even when the stub supplies it.
- Empty result is `200` with `[]`, not `null` or `{}`.
- `median_completion_seconds` serialises as `null`, not omitted and not `0`.
- Upstream unavailable is `503`, not an empty funnel.

## Out of scope

- **Writing** `abandoned`/`expired` statuses, or adding the gc the package doc imagines. This change is read-only. Follow-up issue filed.
- Verifications and invitations. The issue mentions them as nearby data; the funnel it specifies is sessions-only. Adding them later is additive.
