# Design — mobile-admin modernise & re-scaffold

**Date:** 2026-07-14
**App:** `mark8ly/apps/mobile-admin`
**Status:** Design approved — pending spec review, then implementation plan

## Problem

The merchant admin mobile app (`apps/mobile-admin`) was built once and left to rot. It
is a generation behind the rest of the mobile fleet, was never tested, and has drifted
out of sync with the web admin portal.

Concretely:

- **Stack is EOL for the current toolchain:** Expo 52 / RN 0.76 / React 18.3 / Reanimated 3.
  It will not build on Xcode 26.5 / iOS 26, and it depends on `@react-native-firebase/*`
  so there is no Expo-Go shortcut — it needs a native build. A boot attempt on the
  iPhone 17 Pro simulator failed twice: first on a corrupted `ajv`/`ajv-keywords`
  dependency tree during `expo prebuild`, and even past that the SDK predates the
  simulator's iOS. **The app cannot run on a current simulator as-is.**
- **Styling is hand-rolled** `StyleSheet` + `theme.ts` using *system* fonts
  (Georgia / System), not the brand type (Source Serif 4 / Source Sans 3). No nativewind.
- **Missing modern niceties** the Home-Chef apps have: haptics, bottom sheets,
  `expo-image`, offline/NetInfo, biometric auth, i18n, rich push, native dirs / EAS.
- **Feature drift vs web admin:** mobile has Dashboard / Orders / Products / Customers /
  More. Web admin also has **Marketing, proper Settings, Stores management**, deeper Support.
- **Zero tests.**

What is *not* rotten: the `api-client` (401 refresh + tenant self-correction), the
auth/tenant gating, and the TanStack Query data hooks are well-built and worth keeping.

## Goals

1. **Stabilise** — the app runs correctly against the live backend and is tested.
2. **Modernise the stack** — Expo 56 / RN 0.86 / React 19 / Reanimated 4, nativewind,
   brand fonts, dev-client / EAS.
3. **Design parity with the admin portal** — Paper · Ink · Moss editorial system,
   Source Serif / Sans, Home-Chef UX patterns (bottom sheets, haptics, skeletons).
4. **Feature parity with the web admin** — Marketing, Settings, Stores management so a
   merchant can run their store from the phone.

## Non-goals

- Public App Store / Play Store listing. Target is **internal / TestFlight** distribution.
  Native config and EAS are set up, but store assets and review readiness are out of scope.
- Changing the mark8ly backend or auth model (GIP Firebase auth, `X-Tenant-ID`, Bearer).
- Re-implementing the web admin in mobile pixel-for-pixel — mobile IA is its own thing.

## Approach — re-scaffold from Home-Chef, port the domain layer

A fresh Expo 56 app scaffolded from the **Home-Chef vendor** shell (the closest analog —
a business-management app), with mark8ly's proven domain layer ported in. The legacy app
is replaced, not patched.

### Lift from Home-Chef (app shell)

- nativewind config: `global.css`, `tailwind.config.js`, `metro.config.js`, `babel.config.js`,
  `nativewind-env.d.ts`, `tsconfig.json`
- Font-gated splash + `useFonts` boot loading
- Provider stack:
  `ErrorBoundary → AuthProvider → GestureHandlerRootView → QueryClientProvider →
  BottomSheetModalProvider → ToastProvider → UndoSnackbarProvider → AppNavigator`
- Custom floating **Dock** tab bar (`components/navigation/Dock`)
- `OfflineBanner`, Toast + UndoSnackbar UI primitives
- Rich push: notification categories, Android channels, deep-link route resolution,
  cold-start replay, badge management, lock-screen actions via SecureStore + fetch
- `focusManager` ↔ AppState wiring (refetch on foreground)
- **Force-upgrade gate** (`useMinVersion` → `/upgrade-required`, 426 defense-in-depth)
- **Biometric lock** (`useBiometricLock`)
- **i18n** (i18next side-effect init + persisted locale)
- EAS config (`eas.json`, `app.json`)

### Keep from mark8ly (domain layer)

- `@repo/mobile-shared`: Firebase GIP `AuthProvider`, `api/client` (401-refresh + tenant
  self-correction), `stores/tenant-store`, `config/env`
- Data hooks: `use-orders`, `use-products`, `use-customers`, `use-dashboard`,
  `use-notifications`, `use-store`, `use-tenant-resolver`
- TenantGate + store-switcher flows, store-scoped 403/404 self-correction

> **Auth seam:** Home-Chef auth goes through a BFF (`X-Auth-Token`, SecureStore). mark8ly
> uses Firebase GIP directly (`X-Tenant-ID`, Bearer). We lift Home-Chef's *shell* but keep
> mark8ly's `@repo/mobile-shared` auth/api/tenant — it already matches the mark8ly backend.
> The `AuthProvider` in the ported provider stack is mark8ly's, not Home-Chef's.

### Swap

- Fonts → **Source Serif 4** (headings, numerals, brand) + **Source Sans 3** (UI/body),
  replacing Geist/Inter.
- Colors → **Paper · Ink · Moss** nativewind theme tokens, mirroring
  `packages/ui/src/styles/mark8ly-tokens.css` and the existing `theme.ts` palette
  (`paper #F7F6F2`, `ink #0E0E0C`, `moss #2D4A2B`, functional `signal`/`danger`/`warning`).

## Information architecture — 5 tabs + More hub

| Tab | Screens | Origin |
|---|---|---|
| **Dashboard** | stats / overview | port + restyle |
| **Orders** | list, detail, actions | port + restyle |
| **Products** | list, detail, create wizard (camera) | port + restyle |
| **Customers** | list, detail | port + restyle |
| **More** (hub) | Marketing\*, Settings\*, Stores management, Support chat, Account, Notifications | \* = net-new |

- Detail views are stack routes under each tab (as today).
- `*` Marketing and proper Settings are the net-new feature-parity work; Stores gets a
  real management screen (today only a picker/selector exists).

## Design system (nativewind, RN port of `.impeccable.md`)

- nativewind preset exposing `paper / ink / moss / signal / danger / warning` +
  spacing / radii / single-elevation shadow tokens.
- `Text` component maps presets (`display / h1 / h2 / h3 / eyebrow / bodyLg / body /
  bodyEmphasis / caption / mono`) to Source Serif (headings, numerals) vs Source Sans (UI).
- Editorial rules: hairline rules between sections (not bordered cards), 6px default radius,
  asymmetric/left-aligned layout, one accent per view. Honor `prefers-reduced-motion`.
- WCAG 2.1 AA: visible moss focus ring, screen-reader-friendly form errors, keyboard/switch
  nav, accessible bottom sheets.

## Phased delivery

- **Phase 0 — Scaffold & boot.** New Expo 56 app; nativewind + Source Serif/Sans fonts;
  provider stack; Dock tab bar shell; login screen. **Exit criterion: `expo run:ios` green
  on iPhone 17 Pro (iOS 26).** This is where the "run on the sim" request lands.
- **Phase 1 — Domain port.** Wire `@repo/mobile-shared` auth/api/tenant + hooks; TenantGate
  + store switcher; Dashboard tab live against backend.
- **Phase 2 — Core screens.** Orders, Products, Customers (list / detail / actions / create
  wizard with camera) ported and restyled in nativewind + brand system.
- **Phase 3 — Feature parity.** More hub + Marketing, Settings, Stores management, Support
  chat, Account, Notifications.
- **Phase 4 — Platform polish (adopt all extras).** Rich push + deep links, offline banner,
  haptics, bottom sheets, skeletons, **biometric lock**, **i18n scaffolding**,
  **force-upgrade gate**.
- **Phase 5 — Test & harden.** jest-expo unit/component on api-client / hooks / gates;
  **Maestro E2E** for login → dashboard → orders → products; EAS internal / TestFlight build.

## Testing strategy

- **Unit / component (jest-expo):** api-client (401 refresh, tenant self-correction),
  data hooks, auth/tenant gate, `Text` preset mapping, force-upgrade + biometric hooks.
- **E2E (Maestro):** login → dashboard; orders list → detail → status action; products
  list → create wizard; store switch; offline banner appearance.
- **Per-phase manual smoke** on the iPhone 17 Pro simulator; every phase must boot and
  exercise its flow before it's called done.

## Risks & mitigations

- **RN 0.76 → 0.86 / React 18 → 19 breaking changes in ported screens.** Mitigate by porting
  screen-by-screen behind the restyle, not lifting files wholesale.
- **Auth-model mismatch when porting Home-Chef's shell** (BFF vs Firebase GIP). Mitigate by
  keeping mark8ly's `@repo/mobile-shared` AuthProvider and only lifting shell/UI concerns.
- **nativewind + brand-font metro/babel config drift** across the monorepo. Mitigate by
  copying Home-Chef's known-good config and adapting paths.
- **Broken root `ajv`/`ajv-keywords` tree** surfaced during the boot attempt — a fresh
  Expo 56 install sidesteps it, but confirm the monorepo install is clean in Phase 0.
- **db-f1-micro / budget constraints** — no new backend load; app reuses existing endpoints.

## Open questions

- Marketing + Settings screen scope: which web-admin sub-features are must-have on mobile
  vs deferred? (Resolve at Phase 3 planning.)
- Whether the new app keeps the `@repo/mobile-shared` package name or grows app-local
  shell modules for the lifted Home-Chef pieces. (Resolve at Phase 0 planning.)
