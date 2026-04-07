# Testing Strategy

> **Hard constraint:** No PR merges without tests for the changed code.
> Tests are a prerequisite, not a follow-up. The codebase is born tested.

## Test pyramid

| Layer | Scope | Runner | Speed | Coverage target |
|---|---|---|---|---|
| **Unit** | Single function/struct, no DB/HTTP/network | `go test ./...` / `vitest` | < 10ms each, full suite < 5s | 80%+ on `internal/<domain>/service.go` |
| **Integration** | Handler → service → repo with real Postgres + real OpenFGA | `go test -tags=integration ./...` | < 100ms each, full suite < 30s | Every endpoint × (happy path + 1 failure path) |
| **Contract** | OpenAPI shape vs frontend types | Part of unit suite | Fast | Added when codegen lands (post-Phase F) |
| **E2E** | Real browser → real stack via docker-compose | `make e2e` (Playwright) | Whole suite < 5min | Every critical flow + every Phase B regression |
| **Migration** | SQL up/down/idempotency, GORM ↔ schema match | Integration suite | Fast | Every migration |

## Conventions

### Naming
- **Go:** `TestThing_DoesX_WhenY` (Google-style, three-part). Example: `TestErrors_NotFound_ReturnsHTTPStatus404`.
- **TS:** Nested `describe` blocks: `describe('Thing', () => describe('when Y', () => it('does X', ...)))`.

### File layout
- **Go unit:** `_test.go` next to the file under test, same package.
- **Go integration:** `_integration_test.go` with `//go:build integration` tag.
- **TS unit:** `*.test.ts(x)` colocated with the file.
- **TS integration:** `*.integration.test.ts(x)` in the same dir.
- **E2E:** `apps/<app>/tests/e2e/*.spec.ts`.

### Fixtures: builder pattern
No JSON files. No `setup()` global state. Each test builds the data it needs:

```go
tenant := test.NewTenant().
    WithSlug("acme").
    WithOwner("user-123").
    Build()
```

```ts
const session = newSession()
  .forUser("user-123")
  .inTenant("acme")
  .build()
```

Builders live in `<package>/testbuild/` (Go) or `__testbuild__/` (TS).

### Test DB
- **Per-test transaction rollback** is the default. `pkg/testdb.NewTx(t)` returns a `*gorm.DB` in a transaction; cleanup auto-rolls-back.
- **Testcontainers** opt-in only when a test needs `LISTEN/NOTIFY` or multi-connection behavior.

### OpenFGA in tests
- **Unit tests** use `internal/authz.FakeClient` (in-process map). 30-line implementation.
- **Integration tests** use the real OpenFGA container from docker-compose.
- Tests are written against the `authz.Client` *interface*, never the FGA SDK directly.

### GIP in tests
- **Unit tests** use `internal/gip.FakeVerifier`. Returns canned `VerifiedToken` for canned input strings. ~30 lines.
- **Integration tests** also use the fake (unless explicitly testing GIP integration, in which case skip-without-creds).
- **E2E tests** use real GIP. The full OAuth dance, real tokens, real verification.

### Coverage
- **Phase A → end of Phase F:** report-only. `go test -coverprofile` per service, printed in CI.
- **From Phase G onward:** soft gate at "previous PR coverage − 2%". PRs that drop coverage by more than 2% require explicit approval.
- **No hard 80% gate.** Goodhart's law. The number is a *health signal*, not a target.

## Phase B regression tests

Each confirmed bug in [`auth-bugs.md`](./auth-bugs.md) maps to a named test in
the new code. The test fails if the bug is reintroduced.

| Bug | Test type | Location | Asserts |
|---|---|---|---|
| #1 Negative tenant cache | Integration | `apps/onboarding/lib/api/__tests__/tenant-cache.integration.test.ts` | Cache returns `undefined` (not cached) for not-found tenants |
| #2 FGA tuple after DB commit | Integration | `services/platform-api/internal/onboarding/completion_integration_test.go` | After completion, FGA `Check` succeeds within 50ms |
| #3 No retry on FGA failure | Integration | `services/platform-api/internal/outbox/outbox_integration_test.go` | Inject FGA failure → outbox written → drainer eventually succeeds |
| #4 Cookie SameSite blocks callback | E2E | `apps/onboarding/tests/e2e/auth-callback.spec.ts` | Browser receives session cookie after OAuth callback |
| #5 GIP tenant ID not validated | Unit + Integration | `services/auth-bff/internal/gip/verifier_test.go` | Token from wrong pool fails verification |
| #6 CSRF middleware too late | Unit | `services/auth-bff/internal/handlers/csrf_test.go` | Every state-mutating endpoint requires CSRF token |

These six tests are the **first six tests** written when porting auth-bff and
the onboarding completion handler in Phases D and E.

## Make targets

```
make test           # all unit tests across the monorepo
make test-unit      # alias for test
make test-int       # integration tests (spins up docker-compose)
make test-e2e       # Playwright e2e (full stack)
make test-all       # everything: unit + integration + e2e
make cover          # coverage report
```

## CI matrix

```yaml
jobs:
  unit:           # always runs, < 1 min
    - go test ./... per service
    - vitest run per app/package
  integration:    # runs on backend changes
    - docker compose up -d postgres openfga
    - go test -tags=integration ./...
  e2e:            # runs on PRs to main
    - make dev (full stack)
    - playwright test
```

## What "born tested" means in practice

For Phase C (location domain port):
1. Write the GORM model + migration **and** the model unit test in the same commit
2. Write the repository method **and** at least one integration test for it in the same commit
3. Write the service method **and** the unit test for it in the same commit
4. Write the handler **and** an integration test that hits it via httptest in the same commit
5. **No exceptions.** A PR that adds business logic without tests is not merged.

This is the rule that distinguishes a tested codebase from an untested one.
We're enforcing it from day one.
