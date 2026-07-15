# Session handoff — 2026-07-15 (mark8ly)

Paste everything below the line into a new session as the opening prompt.

---

You are taking over work in `/Users/Mahesh.Sangawar/personal/tesserix-new/mark8ly` (Tesserix "Mark8ly" — Go microservices + Next.js web apps + an Expo mobile-admin app). **Commit directly to `main`, single-line conventional messages, no signatures, no PRs.**

**Read these memory files first** (`~/.claude/projects/-Users-Mahesh-Sangawar-personal-tesserix-new-mark8ly/memory/`):
- `project_mobile_admin_modernise.md` — mobile-admin program state.
- `mobile_admin_nativewind_metro_landmines.md` — **CRITICAL: 15 build/runtime traps.** Read before touching the Expo app.
- `MEMORY.md` — index.

## YOUR IMMEDIATE JOB: finish the Connected-accounts feature (1 of 4 tasks done)

**Plan:** `docs/superpowers/plans/2026-07-15-mobile-admin-connected-accounts.md`
**Spec:** `docs/superpowers/specs/2026-07-15-mobile-admin-connected-accounts-design.md`
**Ledger (read this first — it has the live state):** `.superpowers/sdd/progress.md`
**Per-task briefs (already generated):** `.superpowers/sdd/ca-task-{1,2,3,4}-brief.md`

Execute with `superpowers:subagent-driven-development` (fresh subagent per task → `scripts/review-package BASE HEAD` → task reviewer → next). Base commit for this feature: **`c3b7dd52`**.

- **Task 1 — DONE + committed (`5b346266`) but NOT REVIEWED.** Review it first. Two things to check: (a) there's no literal RED transcript (test+impl were written together); (b) the implementer added TS overload signatures to the test's `setCurrentUser` helper (not in the brief) for strict-null-check — confirm assertions were not weakened. The load-bearing behaviour: **`unlinkProvider` must throw `LastSignInMethodError` BEFORE calling `unlink` when `providerData.length <= 1`** (a merchant must never lock themselves out).
- **Tasks 2, 3, 4 — pending.** Briefs are written; follow them.
- Finish with an **opus whole-branch review** (`scripts/review-package c3b7dd52 HEAD`). The last two features' final reviews each caught a Critical the per-task reviews missed — do not skip it, and **verify its claims yourself** (one of its findings last time was a false positive that I disproved with a negative control).

**What this feature is:** a signed-in Settings→Security screen listing sign-in methods (password/Google/Apple) with link/remove. It links to `currentUser`, so **Apple "Hide My Email" works** (the relay email never has to match). Login-time linking (already shipped) can't cover that.

## State of the machine right now
- **Real-mode metro is running on `:8081`** and the app is installed on the **iPhone 17 Pro** simulator in REAL (non-demo) mode, pointed at **prod** (`https://api.mark8ly.com`).
- `apps/mobile-admin/.env.local`, `GoogleService-Info.plist`, `google-services.json` are all in place and **gitignored**.
- **NEVER run `npm ci` / `npm install` / `rm -rf node_modules`** while metro is running — it wipes the running app's deps. `npm install --package-lock-only` here causes a **4871-line mass re-resolve** — don't (see landmine 15).
- Verify commands: `cd apps/mobile-admin && npx jest` (45/45 currently) · `npx tsc --noEmit` (**2 pre-existing `app/(tabs)/_layout.tsx` expo-notifications errors are expected — ignore them**) · `EXPO_PUBLIC_AUTH_BACKEND=demo npx expo config --json | grep -c googleServicesFile` → must be `0`.
- To restart real metro: `cd apps/mobile-admin && npx expo start --dev-client --port 8081` (**NO** `EXPO_PUBLIC_AUTH_BACKEND=demo` — a stale demo metro silently serves the demo bundle so Firebase never inits; check with `ps eww <pid> | grep EXPO_PUBLIC_AUTH_BACKEND`). Reopen app: `xcrun simctl openurl <udid> "exp+mark8ly-admin://expo-development-client/?url=http%3A%2F%2Flocalhost%3A8081"`.

## Waiting on the USER (don't block on these)
1. **Tap-through verify** of email + Google sign-in on the running sim (I can't tap headlessly). This now includes the password→Google **merge** case.
2. **Apple provider not yet enabled.** Steps given: Apple Developer (paid) → App ID `com.mark8ly.admin` + Sign In with Apple capability; create a Services ID; create a Key with Sign in with Apple → download `.p8`, note Key ID + Team ID. Then Identity Platform (project `tesseracthub-480811`) → tenant `MP-Internal-e986p` → Providers → Apple → fill Services ID / Team ID / Key ID / `.p8`. **Sim must be signed into an Apple ID** or Apple sign-in errors. Until this is done, Apple link/sign-in will fail — that's expected, not a bug.

## Completed this session (all on `main` — do NOT redo)
1. **Web-admin product image resolution fix** (`a072a2ee..36edc970`, 7 tasks). Add now uploads the **pristine original** (it becomes `gcs_path_original`); cropping is optional via the existing non-destructive recrop path (aspect selector 4:5/1:1/3:4/Free, quality 0.95, advisory min-res guard). Backend: recrop signs the PUT with the caller's `content_type`. Final review: Ready.
2. **mobile-admin Phase 1a — social auth** (`36edc970..2dfd20fc`). Google + Apple buttons, credential-free. Final review caught 3 demo-path breakers (fixed in `2dfd20fc`).
3. **Phase 1b real-auth boot — VERIFIED.** Real build compiles, Firebase/GIP init, login renders with all 3 methods. Fixed a live bug: `gip.ts` used `firebaseAuth.tenantId = …` but **RNFirebase v22+ makes `tenantId` a read-only getter** → now `setTenantId()` (`c6f86be5`) + regression test (`fd9ffa8e`).
4. **Login-time account linking** (`1ef9f134..509da51c`, 9 commits). Password-registered merchant tapping Google/Apple now gets a link prompt instead of an error — web parity. Final review caught a **Critical**: **RNFirebase auth errors carry NO `email`** (my spec wrongly assumed web-SDK behaviour), so the merge was a no-op; fixed by decoding the id_token's `email` claim (`33e9abe6`).

## Config facts (already resolved — don't re-derive)
- GIP tenant **`MP-Internal-e986p`**; Firebase/GCP project **`tesseracthub-480811`** (number `849928263410`) — same as the web admin.
- Google **web** client `849928263410-5djgu3n40c5tpr86votuptkitqveegor.apps.googleusercontent.com` (public-by-design, reused from web admin); iOS client + `iosUrlScheme` came from the plist.
- **Web admin has Google + password only — NO Apple sign-in.** Tenant is *one-account-per-email* (`signInWithIdp` returns `needConfirmation`); web merges via `apps/admin/lib/gip/{signup,link}.ts`.

## Deferred / known (mention, don't auto-do)
- Spec's exact error copy strings for the link prompt; `provider.tsx` wrapper identity churn (cosmetic).
- **Android Google sign-in needs an Android OAuth client (`client_type: 1`) with the signing SHA-1** — the current `google-services.json` only has web (`client_type: 3`) clients. iOS is unaffected.
- EAS/TestFlight prep: `app.config.js` still has placeholder `extra.eas.projectId: 'your-eas-project-id'` (run `eas init`); `.env.local` values must move to EAS env/file secrets for cloud builds.
- Mobile product upload uses **multipart** but the backend is **signed-URL only** — mobile image upload is broken against the real backend (Phase 2 product-wizard fix).
- Dashboard notification bell; products FAB dock-clearance; Phase 0 whole-branch review package (`.superpowers/sdd/review-d449b4db..HEAD.diff`, never run).
