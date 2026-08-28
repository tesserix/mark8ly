# Declare `inbox` and guard the conformance declaration (#415) Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** mark8ly's estate queue is implemented, deployed, and invisible by contract. Declare it, record why the other mounted routes are *not* declarable, and add the guard that makes the next omission fail in CI instead of surfacing as an empty console queue with no diagnostic.

**Architecture:** One key added to `admin-conformance.json`; one prose doc carrying the reasoning JSON cannot hold; one Go test in `platformadmin` that reads the declaration, builds the real router, and reconciles the two in both directions.

**Issue:** https://github.com/tesserix/mark8ly/issues/415

---

## Findings that shape this plan

All four were verified against the actual sources, not inferred. They change what the issue asks for.

**1. The declarable vocabulary is CLOSED — 9 ids.** `design-system/packages/admin-conformance/src/contract.ts:13-126` defines `ENDPOINT_IDS` as exactly `kpis`, `inbox`, `audit-logs`, `entities`, `health`, `billing/subscriptions`, `billing/trials`, `tenant-lifecycle`, `lifecycle/reason-codes`. mark8ly declares 8. **`inbox` is the only real gap.**

**2. The issue's "check the neighbours" list cannot be declared at all.** `outbox`, `email-sends`, `notifications`, `tickets`, `break-glass`, `conversions`, `onboarding-funnel` are not contract endpoints. `declaration.ts:parseEndpoints` **throws** on an unrecognised key (`unknown endpoint "break-glass"`), which fails the entire conformance run rather than skipping that entry. Their absence is structural, not a forgotten decision. That collapses acceptance item 2 from "record seven decisions" to "record one fact."

**3. The issue's SLA expectation is backwards.** It says mark8ly's declaration "is not the same value" as Kora's `slaDeclared: false`. It must be `false`. `checks.ts:206-220` requires **every** item to carry `due_at` when an SLA is declared. Of mark8ly's five inbox kinds only `sea_review` sets it (`internal/inbox/sea_review.go:72`); `arbitrage`, `migration_fastpath`, `onboarding` and `erasure` never do — and `internal/inbox/erasure.go:13` records that as deliberate: *"GDPR's 30-day window is real, but the table has no due column and deriving a statutory deadline in a read endpoint would be inventing policy in the wrong place."* So `true` fails conformance the moment the queue holds anything but a SEA review.

**4. `admin-conformance.json` is parsed with `JSON.parse`.** It cannot carry comments. Every rationale in this plan goes in a doc, not the file.

**5. Declaring `inbox` is safe today.** The suite runs as a nightly CronJob against production. `kubectl` shows `mark8ly-marketplace-api-admin` running `main-f141913` (rolled out 2026-08-28T04:15Z), which is current `main` — so the deployed revision serves `/admin/inbox`. Verified rather than assumed, because over-declaring is the direction that turns a trusted surface permanently red.

---

## Global Constraints

- **No migration.** `ExpectedSchemaVersion` does not move. Any DDL means this plan is wrong.
- **`admin-conformance.json` is strict JSON.** No comments, no trailing commas. It must remain parseable by `JSON.parse` or the nightly run dies at load.
- **Do not declare an endpoint mark8ly does not serve at its federated base URL.** Over-declaring is worse than under-declaring: the suite's own reasoning is that an under-declared product is a visibly missing source, an over-declared one is a permanent red failure on a surface operators are meant to trust.
- Test packages in marketplace-api are EXTERNAL (`package platformadmin_test`).
- Run from the service root: `cd services/marketplace-api && go test ./... -count=1`. Never path-scope the whole-suite run, or the root schema-version guard silently does not run. Use `-count=1`.
- `go build ./...`, `go vet ./...` and `go vet -tags=integration ./...` must all pass.
- Never mount anything on the merchant `/admin/tenants/:tenantId` group — two different wildcard names at one path position panic gin at router build time.
- Conventional single-line commit messages, no signature, no `Co-Authored-By` trailer, no emoji.
- **Pre-existing failures — not yours to fix:** `internal/billing/trial/subscribe_integration_test.go` (19 tests, #317), `internal/subscription/planchange` integration (9 FAIL), `internal/whitelabel` integration (nil-pointer panic).

---

## File Structure

| file | responsibility |
|---|---|
| `admin-conformance.json` (modify) | add the `inbox` key |
| `docs/admin-conformance.md` (create) | the reasoning the JSON cannot hold: closed vocabulary, why seven routes are undeclarable, why `slaDeclared` is `false`, implemented ≠ declared ≠ wired |
| `services/marketplace-api/internal/handlers/platformadmin/conformance_declaration_test.go` (create) | the guard, in both directions |
| `services/marketplace-api/internal/handlers/platformadmin/routes.go` (modify) | point the existing conformance comment at the new doc |

---

## Tasks

### Task 1 — Declare `inbox`, and write down why everything else is absent

- [ ] `admin-conformance.json`: add `"inbox": { "slaDeclared": false }`. Keep the file valid JSON; place the key so the block still reads in contract order.
- [ ] Create `docs/admin-conformance.md` covering, with file:line citations to the suite's own source so a reader can check the claims:
  - **The vocabulary is closed to 9 ids**, naming them, citing `contract.ts:13-126`, and stating that an unknown key throws and fails the whole run (`declaration.ts:parseEndpoints`).
  - **The seven mounted-but-undeclarable routes** — `outbox`, `email-sends`, `notifications`, `tickets`, `break-glass`, `entities/conversions`, `onboarding-funnel` — listed explicitly, with the statement that they are mark8ly-specific reads the console consumes directly and *cannot* appear in this file. This is the "recorded, not inferred" the issue asks for; it is one fact, not seven decisions.
  - **Why `slaDeclared` is `false` despite a real SLA.** `sea_manual_review` carries a five-business-day `sla_due_at` that pauses a subscription clock, and it is the only one of five kinds that sets `due_at`. Declaring `true` requires `due_at` on *every* item (`checks.ts:206-220`), which the other four deliberately do not carry. State plainly what flipping it to `true` would require: `due_at` on all five kinds, including inventing a statutory deadline for erasure that `erasure.go:13` explicitly refuses to invent. Link the upstream issue from Task 3.
  - **Implemented ≠ declared ≠ wired** — the three states the issue names. This file speaks only to the second; the third is per-environment `Deps` wiring plus platform-api's federation registry, and cross-reference `tesserix-home#407`, which is the wired half for the estate.
- [ ] `routes.go`: extend the existing conformance comment (near the `EstateUsers` field) to point at the new doc rather than restating it.

**Verify:** `python3 -c "import json;json.load(open('admin-conformance.json'))"` (or `jq . admin-conformance.json`) exits 0 — the file must stay strictly parseable.

### Task 2 — The guard test

- [ ] Create `conformance_declaration_test.go` in `package platformadmin_test`.
- [ ] Read `admin-conformance.json` from the repo root. The package sits at `services/marketplace-api/internal/handlers/platformadmin`, so the root is five levels up. Resolve it relative to the test file rather than the working directory, and **fail loudly if the file cannot be found** — a guard that silently skips when the path drifts is the exact failure it exists to prevent.
- [ ] Mirror the 9 contract ids as a Go constant, with a comment naming `design-system/packages/admin-conformance/src/contract.ts` as the source of truth and stating that this is a cross-repo, cross-language mirror that must be updated when the contract adds an endpoint.
- [ ] Declare the mapping from contract endpoint id → the route template(s) that implement it. Derive the route set by building the REAL router via `platformadmin.Register` with every dependency wired, enumerating gin's own route table — reuse the existing `allWriteRoutesDeps`-style helper rather than hand-writing route strings.
- [ ] **Assert both directions.** This is the point of the task:
  - A contract id whose route IS mounted but which is NOT declared → fail, naming the id and the route. This is #415's bug: `inbox` shipped and stayed invisible.
  - A contract id that IS declared but whose route is NOT mounted → fail, naming the id. This is the over-declaring direction that turns the nightly job permanently red.
- [ ] Assert every key in the file is one of the 9 ids, mirroring `declaration.ts`'s throw-on-unknown-key rule, so a typo fails in mark8ly's own CI rather than in the nightly run against production.
- [ ] Assert the test cannot pass vacuously: require that the router actually yielded routes and that at least one contract id was matched. Read `TestAllWriteRoutesDeclareACapability`'s doc comment first — it explains why the write-side guard wires every dependency, and the same trap applies here.
- [ ] Note in the test's doc comment that it verifies *declared vs mounted*, and deliberately says nothing about *wired in the deployed environment* — that third state is what the nightly CronJob and platform-api's federation registry cover.

**Verify:** `go test ./internal/handlers/platformadmin/... -count=1`, then the full suite from the service root, plus both vets. Confirm the test FAILS if you temporarily remove the `inbox` key — capture that as evidence, then restore.

### Task 3 — Raise the per-kind SLA gap upstream

- [ ] File an issue against the design-system repo (the `admin-conformance` package) proposing per-kind SLA declaration.
- [ ] Argument: `slaDeclared` is one flag per product, but SLA reality is per queue kind. mark8ly has five kinds; one (`sea_manual_review`) carries a real five-business-day deadline that pauses a subscription clock, four deliberately carry none. `true` fails the suite, `false` understates a real commitment and costs the console the deadline signal on the one queue that has one. **Neither value is honest.**
- [ ] Cite `checks.ts:206-220` for the all-or-nothing assertion and `erasure.go:13` for a documented, principled refusal to invent a `due_at`. Note Kora declares `false` with no SLA at all, so today the two products are indistinguishable on this flag despite differing materially.
- [ ] Cross-reference #415 and link back from `docs/admin-conformance.md`.

---

## Out of scope

- **Any change to the seven undeclarable routes.** They cannot be declared; the fix is documenting that, not restructuring the contract.
- **Flipping `slaDeclared` to `true`.** It would require `due_at` on all five kinds, including a statutory deadline for erasure that the code deliberately refuses to invent. That is a product decision, and Task 3 is where it gets raised.
- **`FEDERATION_MARK8LY_ENDPOINTS`.** The "wired" half of the estate queue's visibility is `tesserix-home#407`, already filed. Declaring `inbox` here does not by itself make it appear in the console.
