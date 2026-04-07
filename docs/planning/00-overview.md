# Mark8ly Rebuild — Planning Discussion Index

This directory captures the design discussion that produced the rebuild plan for
Mark8ly. These docs are decision records, not implementation specs. They exist
so future-you (and any collaborators) can understand **why** the structure looks
the way it does, not just what it is.

## Context

- **Current state:** A multi-tenant marketplace platform recently migrated from
  Keycloak → Google Identity Platform (GIP) and bolted on OpenFGA for
  authorization. The migration left the system with intermittent auth/authz
  bugs (onboarding sometimes fails, "tenant not found" after login, login
  flakiness).
- **Codebase:** Spread across ~6 repos including ~30 Go microservices and
  3 Next.js apps (onboarding, admin, storefront). All sharing one Cloud SQL
  db-f1-micro instance. Backed up under `../mark8ly_backup/` as a read-only
  reference for the rebuild.
- **Status:** App is **not yet live**. No real users. This is the cheapest
  possible time to delete features and restructure.
- **Scope of this rebuild:** Start with a clean monorepo, port the onboarding
  app first as a vertical slice, then re-evaluate before tackling admin and
  storefront.

## Documents

| # | Doc | Purpose |
|---|---|---|
| 00 | [overview.md](00-overview.md) | This file. Index + context. |
| 01 | [architecture-decisions.md](01-architecture-decisions.md) | Backend service collapse: ~30 services → 6. What to merge, keep, delete. |
| 02 | [monorepo-structure.md](02-monorepo-structure.md) | Turborepo layout, polyglot workspace, shared packages strategy. |
| 03 | [frontend-strategy.md](03-frontend-strategy.md) | Why no MFE for admin. Frontend restructure plan. |
| 04 | [auth-and-authz.md](04-auth-and-authz.md) | GIP + OpenFGA integration, suspected bug sources, fix strategy. |
| 05 | [migrations-strategy.md](05-migrations-strategy.md) | Why no GORM AutoMigrate. golang-migrate two-binary pattern. |
| 06 | [onboarding-phase-plan.md](06-onboarding-phase-plan.md) | Phase A–H plan for the onboarding-only first slice. |
| 07 | [open-decisions.md](07-open-decisions.md) | Decisions still pending before Phase A kickoff. |

## Reading order

If you're new to the plan, read in order. If you're returning to it, start at
[07-open-decisions.md](07-open-decisions.md) to see what's still in flight.

## Guiding principles (the short version)

1. **Behavioral parity first, improvements second.** The rebuild ports working
   logic from `mark8ly_backup/` into a clean structure. New features and
   redesigns come after parity, not during.
2. **Vertical slice over horizontal layer.** Port one full feature end-to-end
   (onboarding) before generalizing patterns.
3. **Premature shared packages are worse than duplication.** Wait for the
   second consumer before extracting.
4. **Fix bugs in old code before porting them.** Otherwise the rebuild
   preserves the bugs perfectly.
5. **Local e2e is non-negotiable from day one.** Every "sometimes" bug becomes
   a deterministic 5-minute repro.
6. **Pre-launch is the only cheap time to delete features.** Be ruthless.
