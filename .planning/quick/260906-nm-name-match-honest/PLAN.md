# Stop recording a name match that never happened

#707. `US`, `CA` and `NZ` are format-only tax validators. All three return the
merchant's OWN submitted business name as the registry's answer:

    // internal/billing/tax/validators/us.go:42
    return tax.ValidationResult{Valid: true, RegistryName: req.BusinessName}

`service.go:132` then runs `CompareNames(in.BusinessName, res.RegistryName)` —
comparing a string with itself — and writes `tax_id_name_match = "matched"`.

So the database records that a registry confirmed the merchant's name, for
merchants where no registry was ever consulted.

## It is a deliberate workaround, which is why the fix is narrow

`us.go`'s own comment says so:

  "Mirrors BusinessName back so the orchestrator's name-match step writes
   'matched' trivially; the attestation checkbox is the real integrity gate."

The intent — do not block a US merchant when we have no registry to check
against — is legitimate and is preserved. What is wrong is the expression: it
records a positive assertion instead of an absence.

## The honest value already exists

`CompareNames` returns `NameNotChecked` when either side is empty
(`namematch.go:25-27`), and `store_subscriptions.tax_id_name_match` already
DEFAULTS to `not_checked` (`internal/subscription/models.go:128`). So the
correct value is the one the schema was designed around.

Verified nothing gates on it: `tax_id_name_match` is written by `service.go`
and surfaced, but no code branches on the value. Changing `matched` to
`not_checked` for these three countries blocks nothing and breaks no flow.

## Task

1. In `us.go`, `ca.go` and `nz.go`, return an empty `RegistryName` instead of
   echoing `req.BusinessName`. Keep `Valid: true` — these countries should
   still pass format validation and should still not be blocked.
2. Rewrite the three comments. Each currently describes the echo as
   intentional ("mirroring BusinessName as RegistryName"); they should say
   instead that no registry is consulted, so no name match is asserted, and
   that the real integrity gate is elsewhere (attestation for US).
3. Update the two tests that pin the echo — `us_test.go:20` and `ca_test.go:20`
   assert `RegistryName` equals the submitted name. That assertion encodes the
   defect. Replace with an assertion that no registry name is returned, so a
   future re-introduction fails.
4. Add a test asserting the end state that matters: for a format-only country,
   the recorded match is `not_checked`, not `matched`.

## Out of scope — needs a product decision, not this change

Whether format-only countries should set `tax_id_validated = true` at all, or
follow the SEA pattern (`MY/TH/PH/ID/VN` return `Valid: false,
ManualReviewRequired: true` and enqueue to `seaqueue`). That changes merchant
flow and needs a queue to route to. Note it on the issue; do not build it.

Also out of scope: NZ's enablement. It is gated off (`NZDisabled` unless
`NZTaxValidationEnabled`), and it stays that way — this only fixes what it
would record if enabled.

## Done when

- No format-only validator returns the submitted name as the registry name
- A format-only validation records `not_checked`
- Nothing is newly blocked: `Valid: true` unchanged for all three
- `go build ./...`, `go vet`, `go test ./internal/billing/tax/...` green

## Constraints

- Single atomic commit, single-line conventional message, NO attribution.
- SHARED CHECKOUT: no checkout/switch/stash/reset beyond this branch.
