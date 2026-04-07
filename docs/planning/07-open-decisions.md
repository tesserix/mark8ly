# Open Decisions

Decisions still pending before Phase A kickoff (and a record of the ones
already settled).

## Settled

| # | Decision | Choice | Reasoning |
|---|---|---|---|
| 1 | Backend service count | Collapse to ~6 (`platform-api`, `marketplace-api`, `auth-bff`, `notification`, `payment`, + managed `openfga`) | See [01-architecture-decisions.md](01-architecture-decisions.md) |
| 2 | Repo strategy | Single polyglot Turborepo (`mark8ly/`) | Reusability-across-products is dead; one repo = atomic changes, easy local dev |
| 3 | `platform-api` location | **In this repo for now**, may extract later | Solo product today; revisit only if a second product reuses it |
| 4 | Frontend split (MFE for admin?) | **No MFE.** Stay with one Next.js app per surface. | See [03-frontend-strategy.md](03-frontend-strategy.md) |
| 5 | First slice scope | **Onboarding only.** Admin/storefront re-evaluated after. | See [06-onboarding-phase-plan.md](06-onboarding-phase-plan.md) |
| 6 | Tickets service | **Delete entirely.** Replace with SaaS (Plain / Help Scout / Crisp). | App not live, no users, support tooling is a tarpit |
| 7 | `qr-service`, `status-service`, `feature-flags-service`, `analytics-service` | **Delete.** | Inline / SaaS / SQL view replacements |
| 8 | `notification` and `document` for the onboarding slice | **Inlined into platform-api** for this phase. Extract when there's a second consumer. | Avoid premature service extraction |
| 9 | Auth provider | **Real GIP everywhere — no emulator.** Dev hits the prod GCP project (`tesseracthub-480811`) and the existing tenant pools. Same code path in dev and prod. | Emulator behavior diverges from real GIP; bugs found in dev should be real bugs |
| 9a | Local secret loading | **`infra/dev/load-secrets.sh` pulls from GCP Secret Manager via `gcloud`** into a gitignored `infra/dev/.env.local`. `make dev` runs it automatically. | Zero hand-managed secrets; new contributor needs only `gcloud auth login` |
| 10 | Authorization | **OpenFGA, real, from day one.** Container in docker-compose. | Same reason |
| 11 | OpenFGA model | **Fresh minimal model** (`user`, `tenant` with `member`, `owner`). Grow when admin/storefront port. | Don't carry old `go-shared/authz` model complexity that isn't exercised |
| 12 | Migration tool | **golang-migrate** (already in use) | Stick with the standard, delete AutoMigrate everywhere |
| 13 | Migration execution | **Two-binary pattern**: `cmd/server` + `cmd/migrate`. SQL migrations embedded via `//go:embed`. `cmd/server` calls `AssertVersion` on startup and refuses to start on mismatch. | See [05-migrations-strategy.md](05-migrations-strategy.md) |
| 14 | GORM AutoMigrate | **Deleted everywhere.** Lint rule to prevent reintroduction. | Source of current Knative cold-start failures |
| 15 | Seeds | **Separate `cmd/seed` binary** reading JSON files. Idempotent inserts. | Seeds and migrations have different lifecycles |
| 16 | Marketing/CMS pages in onboarding app | **Port as-is in the same phase as the wizard.** | Working today, low-risk, removing adds friction |
| 17 | Old `go-shared` Go module | **Don't copy in.** Write fresh `pkg/` slices in each service. Hoist when ≥3 services share the same code. | Avoid carrying complexity for unused features |
| 18 | Design system | **Keep `@tesserix/web`** (and `@tesserix/hooks`, `@tesserix/tokens`, `@tesserix/utils`) as the UI foundation for all three apps. Pin all apps to the same version. | Working today, all apps already use it, built on shadcn/ui + Radix |
| 19 | Frontend duplicate-code cleanup | **Promote duplicates to `packages/*` only when 2nd consumer appears.** Primitives stay in `@tesserix/web`; `packages/ui` is for app-level *compositions* (`<TenantSwitcher>`, `<EmptyState>`, `<DataTable>`), not buttons/inputs. | See [03-frontend-strategy.md](03-frontend-strategy.md) |

## Still pending — answer before Phase A

| # | Decision | Options | Lean |
|---|---|---|---|
| P1 | Time budget | Full days / part-time / evenings only | Sets whether "4–5 weeks" is calendar weeks or wall-clock weeks. Need to know to set realistic milestones. |
| P2 | Per-service `pkg/` vs hoisted `go-shared` module | Duplicate small slices per service / create `go-shared` from day one | **Per-service `pkg/` for now.** Hoist when actual reuse pressure appears. |
| P3 | `mp-marketing`, `mp-gift-cards`, `mp-loyalty`, `mp-approvals` | Delete / defer (port stub only) / port full | **Defer all four** unless validated as launch features. Confirm. |
| P4 | Schema-version-mismatch behavior in `cmd/server` | Fail fast (loud) / warn-and-continue | **Fail fast.** Strong recommendation. Confirm. |
| P5 | Marketing `apps/marketing` split | Keep marketing pages in `apps/onboarding` forever / split into `apps/marketing` later | **Keep in onboarding for now**, split only if marketing grows beyond static pages. Confirm. |
| P6 | CI scope for the onboarding slice | Lint+test+build only / add image build + push to GAR / add deploy | **Lint+test+build only.** Image push and deploy come in the infra-rewrite phase. Confirm. |

## Decisions deferred until after the onboarding slice

These don't block Phase A but will need answers later:

| # | Decision | When |
|---|---|---|
| D1 | Whether to do the admin restructure at all (or just keep old admin running) | After Phase H, based on how Phase D–F felt |
| D2 | Whether to do the storefront restructure at all | After Phase H |
| D3 | Codegen tool for TS types from Go structs (oapi-codegen / openapi-typescript / homegrown) | When a second consumer of `platform-api` types appears |
| D4 | Whether to extract `go-shared` Go module | When ≥3 services share the same code |
| D5 | Whether to extract `packages/api-client`, `packages/auth`, `packages/tenant`, `packages/domain-types` | When the second frontend app lands |
| D6 | Pub/Sub real vs emulator in dev | When there's an actual cross-service event flow worth testing locally |
| D7 | Terraform / K8s / ArgoCD rewrite | After all three apps are ported (Phase 4 of the overall plan) |
| D8 | When to delete `mark8ly_backup/` | After cutover |
| D9 | Whether to replace tickets-service with Plain / Help Scout / Crisp / something else | Before launch, not before |
| D10 | Whether to keep `feature-flags-service` deletion as "use a `flags` table" or adopt LaunchDarkly free tier | Before launch |
