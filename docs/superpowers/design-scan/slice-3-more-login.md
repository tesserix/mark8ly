# Design audit — slice 3: More tab + Login

Scope: `app/(tabs)/more/{index,account,notifications,security,support}.tsx`, `app/login.tsx`, `components/auth/LinkAccountPrompt.tsx`.
Measured against `lib/theme.ts` tokens and the Paper·Ink·Moss editorial system. Read-only — no fixes applied.

---

## P0

**[P0]** `app/login.tsx:1-193` (whole file) — Login form has no `ScrollView`/`KeyboardAvoidingView`. All content sits in a single `flex-1 justify-center` column with two `TextInput`s, three CTAs, and a modal trigger. On a small device, with Dynamic Type/larger accessibility font sizes, or when the keyboard opens over the password field, the lower controls (Apple sign-in, error text) can be pushed off-screen with no way to scroll to them — an unrecoverable auth blocker for a subset of users/devices. — Fix: wrap the form in `KeyboardAvoidingView` (`behavior="padding"` on iOS) + `ScrollView` with `keyboardShouldPersistTaps="handled"`, matching the pattern any other scroll screen in the app would use.

**[P0]** Systemic contrast failure on `textTertiary` / `ink-muted` — the nativewind token `ink.muted` (`tailwind.config.js:26`, `#7A766E`) rendered on the Paper background (`#F7F6F2`) measures ≈4.19:1, and the parallel `theme.ts` `textTertiary` value (`rgba(14,14,12,0.5)`, used directly via inline `color` props) composites to ≈3.56:1 on Paper — both below the WCAG AA 4.5:1 floor for text under 18pt/14pt-bold. This token is the default color for every eyebrow, caption, and secondary label in the slice: `app/(tabs)/more/index.tsx:47` (chevron via `theme.colors.textTertiary`), `:62` (`PageHeader eyebrow="MORE"`); `account.tsx:25` (`InfoRow` label), `:51`, `:58` (`Eyebrow` "Profile"/"Store"); `security.tsx:123` (`Eyebrow` "Ways to sign in"), `:136` (connection status caption); `notifications.tsx:81` (timestamp caption); `login.tsx:103` (subtitle, `text-ink-muted` directly on Paper, ≈4.19:1, fails). — Fix: darken `ink.muted` (and the `theme.ts` `textTertiary` rgba alpha) until it clears 4.5:1 against `#F7F6F2` — e.g. drop to roughly `#6B675F` / raise rgba alpha to ~0.62 — and re-verify against both Paper and Paper-elevated white.

---

## P1

**[P1]** `app/login.tsx:96` — `<View className="flex-1 justify-center px-6">` vertically centers the entire login block (eyebrow, wordmark, subtitle, inputs, three CTAs) as one symmetric column with no offset element or varied rhythm. This is the exact "centered login hero" pattern the design bar calls out as an AI-slop tell. — Fix: anchor content from the top (e.g. `pt-16` / safe-area offset) with asymmetric spacing instead of `justify-center`; let the wordmark sit high and left, not mid-screen.

**[P1]** `app/(tabs)/more/security.tsx:125-166` — Each unlinked provider row renders its own `Link` action in `color="accent"` (moss). If both Google and Apple are unlinked (a common first-run state), two moss "Link" labels render simultaneously in the same view — a direct violation of "one moss accent per view, spent once." — Fix: reserve moss for at most one emphasized action per screen; render the non-primary `Link` action in ink/bodyEmphasis and let moss appear only on a single highlighted "recommended" method, or drop moss entirely here in favor of ink text + underline.

**[P1]** `app/(tabs)/more/notifications.tsx:24-25,67-68,111-113` — `TYPE_DOT` maps both `new_order` and `order_fulfilled` to `theme.colors.accent`, rendered as both a status dot (`:68`) and a 3px unread bar (`:67`) per row, and the header's "Mark all" action (`:111-113`) also uses `color="accent"`. Any inbox with more than one unread order notification shows several simultaneous moss elements plus the moss header action — the opposite of "one accent, spent once." — Fix: use a neutral ink/tertiary dot for status and reserve moss solely for the single header action, or drop color-coded dots in favor of a text/icon distinction that doesn't consume the accent per-row.

**[P1]** `app/(tabs)/more/support.tsx:64-78` — `SupportPalette.bubbleOwn` and `.primary` are both bound to `theme.colors.accent`. Every message the merchant sends renders as a moss bubble, so any conversation with more than one outgoing message guarantees multiple simultaneous moss elements on screen — a structural, not incidental, one-accent violation baked into this screen's palette wiring. — Fix: use ink (or a neutral elevated surface) for the merchant's own bubbles and reserve moss for a single actionable element (e.g. the send button), consistent with "ink is the primary CTA color."

**[P1]** `app/login.tsx:108-126` and `components/auth/LinkAccountPrompt.tsx:108-116` — Email/password `TextInput`s have `secureTextEntry` where relevant but no `textContentType` (`"username"`/`"password"`) or `autoComplete` (`"email"`/`"current-password"`). This is explicitly called out in the design bar as a P1-class field-security/AA gap — password managers and iOS/Android autofill can't reliably target these fields, degrading both security hygiene and usability for an auth-critical flow. — Fix: add `textContentType="username"`/`autoComplete="email"` to the email field and `textContentType="password"`/`autoComplete="current-password"` (or `"password"` on registration-adjacent flows) to both password fields.

**[P1]** `app/login.tsx:112,122` and `components/auth/LinkAccountPrompt.tsx:112` — `placeholderTextColor="#7A766E"` is a hardcoded hex (duplicated three times) instead of a theme reference, violating "only `lib/theme.ts` tokens — no hardcoded hex." It happens to equal `ink.muted`, which compounds the P0 contrast failure above once fixed there this literal will silently drift out of sync. — Fix: export a shared constant (e.g. from `lib/theme.ts` or a small hook) and reference it in all three call sites instead of inlining the hex.

**[P1]** `components/auth/LinkAccountPrompt.tsx:99` — `rounded-t-xl` resolves to Tailwind's built-in `xl` (12px) because `borderRadius` in `tailwind.config.js:76-83` only defines `none/sm/DEFAULT/md/lg/full` — `xl` isn't part of the system's radius scale (0/4/6/10/14/pill) at all, so this sheet silently uses an off-system value. — Fix: use `rounded-t-lg` (14px, the system's largest defined radius) or add an explicit `xl` mapping to the token scale if a bigger sheet radius is genuinely wanted.

---

## P2

**[P2]** `app/(tabs)/more/index.tsx:63`, `account.tsx:52,59`, `security.tsx:124` — `Card` is used everywhere in its `outline` variant, i.e. a full hairline border around the whole block. The design principle calls for "hairline rules between sections, not bordered cards" — every settings surface in this slice is, structurally, a bordered card (just with a thin border weight), not rule-separated content. — Fix: consider a `Card` `variant="ghost"` + `Hairline` above/below the block for top-level list surfaces, reserving the outlined card for genuinely card-like content (e.g. the profile info block), not full-screen row lists.

**[P2]** `app/login.tsx:140-178` — "Sign in" (ink) and "Sign in with Apple" (ink) are both styled as filled primary buttons, with "Continue with Google" as the only outlined/secondary treatment sandwiched between them. Two ink-filled CTAs compete for primary-action weight on one screen. — Fix: give only one action (email/password sign-in) the filled-ink treatment; render both social options as outlined/secondary so there is a single visual primary.

**[P2]** `app/(tabs)/more/notifications.tsx:134-136` — `RefreshControl` sets `tintColor` (iOS-only) but no `colors` prop, so the pull-to-refresh spinner ignores the theme entirely on Android and falls back to the OS default color. — Fix: add `colors={[theme.colors.text]}` alongside `tintColor` for Android parity.

**[P2]** `app/(tabs)/more/index.tsx:105` — `Linking.openURL(browserUrl)` has no `.catch`/error handling; if the URL can't be opened the failure is silently swallowed with no user feedback. — Fix: wrap in try/catch (or `.catch`) and surface a toast/alert on failure.

**[P2]** `app/(tabs)/more/index.tsx:137,144` — Badge `borderRadius: 10` is a magic number that happens to equal `theme.radii.lg` but isn't written as a token reference; `badgeLabel.fontSize: 10` sits below the smallest defined type preset (`caption` = 12), effectively introducing an off-system type size. — Fix: reference `theme.radii.lg` directly, and either accept `caption` (12px) for the badge or add a deliberate "micro" preset to the type scale rather than a one-off literal.

## P3

**[P3]** `components/auth/LinkAccountPrompt.tsx:98` — Overlay uses `bg-ink/40` while `theme.ts` defines `overlay: "rgba(14, 14, 12, 0.45)"` — a trivial opacity mismatch (40 vs 45) between the two parallel token systems (nativewind classes vs `theme.ts` raw values). — Fix: use `bg-ink/45` to match the canonical overlay token.

**[P3]** `components/ui/BackHeader.tsx:38` (affects `account.tsx`, `security.tsx`, `notifications.tsx`, `support.tsx` via shared header) — Detail-screen titles render in `bodyEmphasis` (sans), while top-level `PageHeader` titles render in `h1` (serif). The two header styles diverge in whether serif "carries the brand" at the section-title level — worth a deliberate call on whether nav-bar titles should stay sans (common iOS/Android convention) or move to a small serif preset for consistency.

**[P3]** `app/(tabs)/more/notifications.tsx:67-68` — The colored unread bar and status dot are separate child views inside a row whose parent already sets `accessible={true}` with a composed label; on Android these decorative children can still occasionally surface to the accessibility tree. — Fix: add `importantForAccessibility="no-hide-descendants"` to the row's inner content wrapper for belt-and-braces safety.

---

## Counts

- P0: 2
- P1: 7
- P2: 5
- P3: 3
