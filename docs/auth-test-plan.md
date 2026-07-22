# Auth test plan — sign-in & account linking (web + mobile)

Covers password, Google, and Apple sign-in plus the one-account-per-email
auto-merge, across the three surfaces:

| Surface | URL / app | Providers |
|---|---|---|
| **Web admin** | `{slug}-admin.mark8ly.com` | password, Google, **Apple** |
| **Web onboarding** | `mark8ly.com` | password, Google (account creation) |
| **Mobile admin** | Mark8ly Admin (iOS/TestFlight) | password, Google, Apple |

> Apple exists on web admin **and** mobile admin. Web **onboarding** has **no
> Apple** yet, so an Apple-first user cannot create a brand-new account — they
> can only add Apple to an account that already exists.

---

## 0. Test accounts & the one-account walkthrough

### Which email to use

**Primary account: `mahesh.sangawar@gmail.com` — use it for password, Google,
AND Apple.** This single email is the best test account because it exercises
the whole auto-merge feature on one identity you fully control:

| Provider | Why this email works |
|---|---|
| **Password** | You own the Gmail inbox → you receive the onboarding OTP and set the password |
| **Google** | It *is* your Google account |
| **Apple** | It is your Apple ID email (choose **Share My Email**, not Hide, so Apple returns this same address) |

> As of this cleanup it is a **clean slate** — no GIP account, no store — so
> you onboard it fresh in step ① below. One email → all three providers → same
> store is exactly the "flexibility" behaviour we want to prove.

Existing live accounts (already have stores + correct claims), usable if you
know their credentials: `demo@mark8ly.com` (password → The Bondi Store),
`parimiti03@gmail.com` (Google → The Facade Factory).

### The single-account merge walkthrough (run this in order)

This one sequence covers the happy path for all three providers **and** the
auto-merge. Do it on web admin first, then repeat the sign-ins on mobile.

- [ ] **① Create it with a PASSWORD.** Go to `mark8ly.com`, onboard with
      `mahesh.sangawar@gmail.com` → verify the OTP from your Gmail → set a
      password (≥8 chars, **write it down**) → note the store slug. This
      creates the GIP account (password provider) + store + `tenant_id` claim.
- [ ] **② Password sign-in.** Go to `{slug}-admin.mark8ly.com` → sign in with
      the email + password → dashboard loads. *(Password provider ✓)*
- [ ] **③ Add Google (merge).** On the login page click **Continue with
      Google** → pick `mahesh.sangawar@gmail.com`. Same email → **link prompt**
      → re-enter your **password** → Google is linked. *(Now password + Google
      both sign you into the same store.)*
- [ ] **④ Add Apple (merge) — SHARE My Email.** Click **Sign in with Apple** →
      authenticate → **choose "Share My Email"** (critical: Hide My Email would
      fork a separate account — that's test C5, not this one). Same email →
      **link prompt** → re-auth with password or Google → Apple is linked.
- [ ] **⑤ Prove the flexibility.** Sign out and sign back in three times — once
      with **password**, once with **Google**, once with **Apple** — each must
      land on the **same** store. *(This is the whole point of auto-merge.)*

After ⑤ you've covered A1, B1, C3, E1, E2, E3, and E7 on one account. The
sections below add the negative paths, edge cases, and mobile.

**Do you need a second email?** Not required. The Hide My Email test (C5) even
produces a no-store account for you (the relay address), so you can test the
"No store yet" empty state without a second real inbox. If you *have* a spare
Google account, it's handy for the "fresh provider, no store" cases (B3, C1).

**Expected baseline for every "happy" sign-in:** you land on the admin
dashboard for your store, no error toast, no redirect loop.

### Label legend (used in sections A–G below)

The detailed sections use short labels — here's what they map to for you:

| Label | Your concrete account |
|---|---|
| **PW-STORE** | `mahesh.sangawar@gmail.com` after step ① (email + your password) |
| **GOOGLE-STORE** | `mahesh.sangawar@gmail.com` via Google (after step ③ links it), or a spare Google account |
| **APPLE-ID** | your Apple ID = `mahesh.sangawar@gmail.com`; **Share My Email** for merge tests, **Hide My Email** for C5 |

Since all three are the same email here, the "linking" sections (E) are the
main event; the per-provider happy paths (A1/B1/C1) are already covered by the
walkthrough above.

---

## A. Web admin — password

- [ ] **A1 Happy path.** Sign in with PW-STORE email + correct password →
      dashboard loads.
- [ ] **A2 Wrong password.** Correct email, wrong password → inline error like
      "Couldn't sign you in. Check your details and try again." **No** mention
      of "wrong password" specifically (GIP enumeration protection collapses
      wrong-password and no-such-user into one generic error — this is
      intentional, not a bug).
- [ ] **A3 Unknown email.** An email with no account → same generic error as
      A2 (must not reveal whether the account exists).
- [ ] **A4 Empty fields.** Submit with blank email / blank password → field
      validation, no network call.
- [ ] **A5 Signed-in persistence.** After A1, refresh the page → still signed
      in (session cookie), no re-login.

## B. Web admin — Google

- [ ] **B1 Happy path.** "Continue with Google" as GOOGLE-STORE → Google popup
      → dashboard loads.
- [ ] **B2 Cancel.** Open the Google popup, close it → returns to the login
      form quietly, **no** error toast.
- [ ] **B3 Google account with no store.** Sign in with a Google account that
      has never onboarded → **"No store found for this Google account. Start a
      new store from the home page."** (You are authenticated but have no
      tenant — this is the empty state, not an error/bounce.)

## C. Web admin — Apple  ⭐ (newly enabled)

- [ ] **C0 Button visible.** The **"Sign in with Apple"** button appears on the
      login form next to Google. (If it's missing, the build didn't get
      `NEXT_PUBLIC_APPLE_SERVICES_ID` — stop and report.)
- [ ] **C1 First-time Apple on a NEW email.** Use an Apple ID whose email has
      **no** Mark8ly account → you authenticate, but there's no store →
      **"No store yet" / start a store** empty state (NOT a crash, NOT a
      bounce). Apple-first signup is a known gap; this is expected.
- [ ] **C2 Apple that shares email with PW-STORE.** Sign in with Apple using
      the same email as PW-STORE → **link prompt** appears ("An account already
      exists for {email}…"). See section E for the link flow.
- [ ] **C3 Returning Apple (already linked).** After E-series linking, sign in
      with Apple again → straight to dashboard, no prompt.
- [ ] **C4 Cancel.** Start Apple sign-in, cancel the Apple dialog → back to the
      login form, **no** error.
- [ ] **C5 Hide My Email.** Sign in with Apple and choose **Hide My Email** →
      you get a `@privaterelay.appleid.com` identity → **"No store yet"**
      (a separate account was created; the relay email matches nothing). This
      is Apple-by-design, **not** a bug. Confirm it does NOT crash or loop.
- [ ] **C6 Client-secret health.** (One-time) Apple sign-in should keep working
      day-over-day with no "invalid client" errors — the GIP provider uses
      code-flow config so the secret auto-rotates. If it breaks with an Apple
      error and no deploy happened, that's the secret path.

## D. Web onboarding — account creation (`mark8ly.com`)

- [ ] **D1 Create with password.** New email → verify OTP → set a password
      (≥8 chars) → store is created → auto-login to admin. Confirm you can
      later sign in at web admin with that email+password (ties into A1).
- [ ] **D2 Create with Google.** New email → verify → **Continue with Google**
      → the Google account's **verified email must match** the email you signed
      up with. Mismatch → **"This Google account (X) doesn't match the email
      you signed up with (Y)."**
- [ ] **D3 Weak password.** Password < 8 chars → rejected with a clear message.
- [ ] **D4 Existing email re-onboard.** Onboard again with an email that
      already has an account → it should sign you into the existing GIP account
      and create a **new** tenant (multi-store), not dead-end. Confirm the new
      store appears.
- [ ] **D5 No Apple button.** Confirm onboarding has **no** "Sign in with
      Apple" option (expected — parity gap).

## E. Account linking / auto-merge  (the core feature)

One email = one account. Signing in with a *new* provider for an email that
already exists triggers a **link prompt**: re-authenticate with the method the
account already has, and the new provider gets attached.

- [ ] **E1 Password account, add Google.** As PW-STORE, click Continue with
      Google (same email) → link prompt → re-enter the **password** → Google is
      linked → dashboard. Afterwards, both password **and** Google sign you in.
- [ ] **E2 Password account, add Apple.** As PW-STORE, Sign in with Apple (same
      email) → link prompt → re-enter password → Apple linked. Afterwards all
      three (password, Google if linked, Apple) work.
- [ ] **E3 Google account, add Apple.** As GOOGLE-STORE, Sign in with Apple
      (same email) → link prompt offers **Continue with Google** to re-auth →
      Apple linked.
- [ ] **E4 Wrong re-auth.** In a link prompt, enter the **wrong** password →
      "That password is incorrect." Account is NOT modified; you can retry.
- [ ] **E5 Cancel link.** Open a link prompt, hit Cancel → back to login, no
      partial link, nothing changed.
- [ ] **E6 Link prompt never dead-ends.** Because GIP enumeration protection
      hides which methods exist, the prompt should **fail open** — it offers
      password + the other providers even if it can't confirm them. Verify you
      always have at least one actionable re-auth option (never just Cancel).
- [ ] **E7 Post-link matrix.** After E1–E3 on one account, sign out and sign in
      with **each** linked provider in turn → every one lands on the same
      store. (This is the "user flexibility" goal: password OR Google OR Apple,
      same account.)

## F. Mobile admin app

Install the current TestFlight build. Apple/Google here use **native** SDKs.

- [ ] **F1 Password sign-in.** Email + password → dashboard.
- [ ] **F2 Google sign-in.** Continue with Google (native sheet) → dashboard.
      Cancel the native sheet → returns quietly, no error.
- [ ] **F3 Apple sign-in.** Sign in with Apple (native iOS dialog) → dashboard.
      Cancel → quiet return.
- [ ] **F4 Link prompt on conflict.** Sign in with a provider not yet on the
      account (same email) → bottom-sheet **"Link your account"** → re-auth
      with the existing method → linked. Mirrors E1–E3.
- [ ] **F5 No-store account → "No store yet" (regression).** Sign in with a
      valid identity that has **no** tenant (e.g. a fresh Apple ID) → the app
      shows its **"No store yet"** empty state and **stays signed in**. It must
      **NOT** bounce back to the login screen in a loop. *(This is the exact
      bug that was fixed — the API now returns 404, not the 401 that used to
      sign the user out.)*
- [ ] **F6 Fresh-onboard → mobile login works with no rebuild.** Onboard a NEW
      merchant on `mark8ly.com`, then immediately sign into that account in the
      mobile app → store loads. (Proves the auto-written `tenant_id` claim
      reaches mobile without an app update.)
- [ ] **F7 Token refresh / session.** Leave the app backgrounded past token
      expiry, return → it silently refreshes and stays signed in (does not
      dump you to login on the first 401).
- [ ] **F8 Sign out.** Sign out → returns to login; signing back in works.
- [ ] **F9 Wrong password.** → generic "incorrect" message, no crash, no
      enumeration leak (same rationale as A2).

## G. Known-limitation checks (confirm expected, not "bugs")

- [ ] **G1 Hide My Email forks an account.** (Web C5 / mobile equivalent) The
      relay-email account is separate and has no store. **Supported fix:**
      while signed in on the *real* account, go to **Settings → Security →
      Link Apple** and confirm it binds the Apple identity (relay and all) to
      the existing account. After that, Apple sign-in reaches the store.
- [ ] **G2 Apple-first has no store (web).** An Apple ID that never onboarded
      can't create a store from the login page (onboarding lacks Apple) — it
      dead-ends at "No store yet." Confirm this is graceful, not a crash.
- [ ] **G3 Cross-provider consistency.** Pick one account with all three
      providers linked; from a logged-out state, each provider resolves to the
      identical store, orders, and settings.

---

## Pass criteria

- Every **happy path** (A1, B1, C1-returning, D1/D2, E7, F1-F3) lands on the
  right store with no loop and no raw error string.
- Every **wrong/cancel** path (A2-A4, B2, C4, E4-E5, F9) shows a friendly
  message or quiet return — never a stack trace, a native SDK path, or a
  redirect loop.
- **No-store** identities (B3, C1, C5, F5) render the "No store yet" empty
  state and **stay signed in** — the mobile bounce loop must not reappear.
- After linking, **all** linked providers open the **same** account (E7, G3).
