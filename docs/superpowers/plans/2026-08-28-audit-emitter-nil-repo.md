# audit.NewEmitter accepts a nil Repo and kills the process (#318) Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** A wiring mistake that disables auditing should refuse to start, not crash the process asynchronously three seconds later — and not silently succeed with auditing off.

**Architecture:** `NewEmitter` becomes a fallible constructor returning `(*Emitter, error)`. Three `main.go` call sites treat the error as fatal. The existing, complete "nil `*Emitter` means auditing is off" contract stays the supported opt-out.

**Issue:** [#318](https://github.com/tesserix/mark8ly/issues/318)

---

## Findings that shape this plan

Verified in the code, not assumed.

**1. The failure is a process kill, not a failed write.** `NewEmitter` (`internal/audit/emitter.go:76-95`) stores `cfg.Repo` unguarded and immediately starts `cfg.Workers` goroutines. `write` (`:210`) calls `e.repo.Create(...)`. A nil-pointer dereference in a goroutine is unrecoverable — it takes the whole process down. In tests it kills the binary, which is why this reads as an unexplained crash rather than a readable failure.

**2. The nil-`*Emitter` contract is real and COMPLETE.** All six exported methods are nil-safe: `Emit` (`:100`), `EmitSync` (`:146`) and `Stop` (`:173`) guard the receiver explicitly; `EmitStateTransition` (`:394`), `EmitAPIKeyEvent` (`:451`) and `EmitPlanChange` (`:502`) build a map and then delegate to `e.Emit(...)`, which is legal on a nil receiver because they never dereference `e` themselves. So `var em *audit.Emitter` is a fully supported "auditing off".

**3. That contract is the *documented* opt-out.** `Emit`'s comment says the receiver guard exists so it is "safe to call when wiring opted out (e.g. unit tests)". The way to opt out is to not construct an emitter — **not** to construct one with a nil `Repo`. `internal/handlers/platformadmin/audit.go` relies on this from the other side, warning loudly via `slog.Default()` when it is handed a nil emitter.

**4. Only three production call sites**, all in `main`: `cmd/marketplace-api/main.go:560`, `cmd/reconciliation-cron/main.go:53`, `cmd/break-glass-rotation/main.go:59`. A signature change is cheap, and `main` is exactly where a wiring failure should stop the process.

**5. Why not "return nil and log".** It is uniform with the contract and would work — but on this system, silently-off auditing is itself the bug class being tracked (#369, #259 are audit/compliance issues, and there is a whole platform audit surface). A warn line at startup scrolls past; a refusal to boot does not.

---

## Global Constraints

- **No migration.** This is a Go API change. Any DDL means the plan is wrong.
- **Do not weaken any existing nil guard.** `Emit`, `EmitSync` and `Stop` must keep their receiver checks — the nil-`*Emitter` opt-out is the supported path and other code depends on it.
- **Do not "fix" the three delegating methods by adding receiver guards.** They are already safe by delegation. Adding guards implies they were broken and invites someone to remove the real guard in `Emit`.
- **A nil `DB` is NOT the same as a nil `Repo`.** Only `Repo` is dereferenced by `write`. Check what `DB` is actually used for before deciding whether it deserves the same treatment; if it does not, say so rather than validating it for symmetry.
- Go: run from the service root, `cd services/marketplace-api && go test ./... -count=1`, never path-scoped. Plus `go build ./...`, `go vet ./...`, `go vet -tags=integration ./...`.
- Conventional single-line commit messages, no signature, no `Co-Authored-By` trailer.
- **Use explicit paths when staging (`git add <path>`), never `git add -A`.**
- **Pre-existing failures — not yours to fix:** `internal/billing/trial/subscribe_integration_test.go` (19 tests, #317), `internal/subscription/planchange` integration (9 FAIL), `internal/whitelabel` integration (nil-pointer panic — **note this is the very crash #318 describes; expect it to change or disappear**), and `TestIntegration_ProductService_UpdateAggregate_OptionValueInUseRejected` (`variant_matrix_mismatch`).

---

## Tasks

### Task 1 — Make the constructor fallible

- [ ] `NewEmitter(cfg EmitterConfig) (*Emitter, error)`. Return a non-nil error when `cfg.Repo` is nil, naming the field and stating that the way to disable auditing is a nil `*Emitter`, not a nil `Repo` — the message is the documentation most people will read.
- [ ] Return before starting any goroutine. The current bug is that workers are already running by the time anything is wrong.
- [ ] Decide on `cfg.DB` deliberately: establish what `write`/`EmitSync` actually use it for and whether nil is legitimate. If it is, leave it alone and say why in the report; if it is dereferenced on a live path, treat it the same as `Repo`.
- [ ] Update the doc comment: what the error means, and that the supported opt-out is a nil `*Emitter`.

**Verify:** `go build ./...` — expect the three call sites to fail to compile. That is the change working.

### Task 2 — The three call sites, and the test that started this

- [ ] `cmd/marketplace-api/main.go:560`, `cmd/reconciliation-cron/main.go:53`, `cmd/break-glass-rotation/main.go:59` — handle the error as **fatal**, matching how each binary already reports startup failures rather than introducing a fourth style.
- [ ] `internal/whitelabel/lifecycle/advancer_integration_test.go:61-63` constructs with `DB: nil, Repo: nil` — the reproduction in the issue. It wanted auditing off, so give it the supported opt-out: a nil `*Emitter`. Do not have it assert on the new error unless that is genuinely what it is testing.
- [ ] Sweep for other `NewEmitter` callers including tests — the three above are the non-test ones, but tests may construct it too.

**Verify:** `go build ./...`, `go vet ./...`, `go vet -tags=integration ./...`, `go test ./... -count=1`. Report whether the `internal/whitelabel` integration panic on the known-failures list is now gone — if it is, that is this fix landing.

### Task 3 — Prove both halves

- [ ] Unit test: `NewEmitter` with a nil `Repo` returns a non-nil error **and a nil emitter**, and — the part that matters — **starts no goroutines**. Assert that meaningfully: compare `runtime.NumGoroutine()` around the call, or construct with `Workers: 8` and show none appear. A test that only checks the error would pass even if the workers still leaked.
- [ ] Unit test: a nil `*Emitter` remains safe across **all six** exported methods — `Emit`, `EmitSync`, `Stop`, `EmitStateTransition`, `EmitAPIKeyEvent`, `EmitPlanChange`. This is the contract the fix now depends on; nothing currently pins it, and the three delegating ones are safe only by an implementation detail that a future refactor could break without noticing.
- [ ] Unit test: a valid config still constructs, starts its workers, and writes.

**Verify:** `go test ./internal/audit/... -count=1`, then the full suite from the service root.

### Task 4 — Close out

- [ ] Comment on #318 with what shipped, and note that the fix removes a known-failure entry: the `internal/whitelabel` integration nil-pointer panic was this bug.
- [ ] If Task 1 concluded `cfg.DB` deserves the same treatment but it was left out of scope, file that separately rather than folding it in silently.

---

## Out of scope

- **Making the three delegating methods guard their own receiver.** They are already safe; adding guards would imply otherwise.
- **Changing what `Emit` does on a failed write.** Fire-and-forget with a logged error is the intended design.
- **Auditing whether every code path that should emit does.** That is a different question from whether the emitter can be constructed broken.
