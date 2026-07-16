# Fix Batch 2 — screen-level a11y + one-accent design pass

Scope: mobile-admin login/auth keyboard safety + field a11y, decorative-moss strip-out (one-accent
rule), and small a11y/touch-target nits. Findings source: `slice-1-dashboard-catalog.md`,
`slice-2-orders-customers.md`, `slice-3-more-login.md`. Batch 1 (tokens/primitives, including the
`textTertiary` contrast fix) was already done — this batch only touches screen-level code.

All paths below are relative to `apps/mobile-admin/` unless noted.

---

## A. Login + auth a11y (P0 auth + security)

### `app/login.tsx`

- **Keyboard-safe wrapper** — the whole `<View className="flex-1 justify-center px-6">` form was
  previously a bare column with no scroll escape hatch. Wrapped it in
  `KeyboardAvoidingView` (`behavior={Platform.OS === 'ios' ? 'padding' : 'height'}`) +
  `ScrollView` (`contentContainerStyle={{ flexGrow: 1 }}`, `keyboardShouldPersistTaps="handled"`).
  The inner `View`'s `className="flex-1 justify-center px-6"` is untouched — same
  alignment/centering as before (Batch 3's job, not this one); it just now sits inside a
  `flexGrow: 1` scroll container so the Apple button / error text stay reachable when the
  keyboard covers the lower controls on a small device. Addresses slice-3 P0
  (`slice-3-more-login.md` line 10).
- **Email field** (was line 108-117, now ~119-129): added `textContentType="emailAddress"`,
  `autoComplete="email"`. (`keyboardType="email-address"` and `autoCapitalize="none"` already
  existed.)
- **Password field** (was line 118-126, now ~130-139): added `textContentType="password"`,
  `autoComplete="password"`. (`secureTextEntry` already existed.)
- **`placeholderTextColor="#7A766E"` → `theme.colors.textTertiary`** (2 of the 3 hardcoded
  hex sites; added `import { theme } from '@/lib/theme'`). Addresses slice-3 P1 (line 28).
- Sign-in / Google / Apple buttons and the inline error already had
  `accessibilityRole`/`accessibilityLabel` and `accessibilityRole="alert"` +
  `accessibilityLiveRegion="polite"` — no changes needed there, verified still present.

### `components/auth/LinkAccountPrompt.tsx`

- Password field (line ~108-116): added `textContentType="password"`, `autoComplete="password"`
  (this is a re-auth, not account creation, so `"password"` not `"newPassword"`). No email field
  exists in this component — the conflicting email is shown as static text, not an editable
  input, so the email-field instruction didn't apply here.
- **`placeholderTextColor="#7A766E"` → `theme.colors.textTertiary`** (3rd of the 3 hardcoded hex
  sites; added `import { theme } from '@/lib/theme'`).
- All actionable elements (Sign in and link, Continue with Google/Apple to link, Cancel) already
  had `accessibilityRole`/`accessibilityLabel`; the error text already had
  `accessibilityRole="alert"` + `accessibilityLiveRegion="polite"`. No changes needed.

---

## B. Strip decorative moss (one-accent rule)

| File | Was | Now | Why |
|---|---|---|---|
| `components/DashboardStats.tsx:41,45` | `ArrowUpRight` icon `color={theme.colors.accent}`; caption `color={positive ? "accent" : "danger"}` | icon `color={theme.colors.text}`; caption `color={positive ? "text" : "danger"}` | Revenue trend is a data delta, not the primary action. Danger side (negative trend) untouched — functional tone. |
| `components/CustomerRow.tsx:41-45,81` | avatar `backgroundColor: theme.colors.accent`, initial `Text color="inverse"` | avatar `backgroundColor: theme.colors.surfaceAlt`, initial `Text color="text"` | Moss avatar repeated once per row in the customer list — worst violation in the slice (20 customers = 20 moss circles). |
| `app/(tabs)/customers/[id].tsx:124-128,200` | profile avatar `backgroundColor: theme.colors.accent`, initial `Text color="inverse"` | `backgroundColor: theme.colors.surfaceAlt`, `Text color="text"` | Same avatar treatment on the detail screen; `unblockBtn`'s moss fill left as-is — that's the screen's one legitimate CTA. |
| `app/(tabs)/more/security.tsx:158` | unlinked-provider `Link` label `Text color="accent"` | `Text color="text"` | Rendered once per unlinked provider — Google + Apple both unlinked (common first-run state) meant two simultaneous moss labels. |
| `app/(tabs)/more/notifications.tsx:24-32` | `TYPE_DOT.new_order` / `TYPE_DOT.order_fulfilled` = `theme.colors.accent` (drives both the per-row dot and the unread bar) | both → `theme.colors.textSecondary` | Per-row indicator repeats once per notification. `"Mark all"` header action (line ~111, `Text color="accent"`) was **kept** moss — it's the screen's single primary action and, with the dots neutralized, is now the only moss element on screen. |
| `app/(tabs)/more/support.tsx:65-78` (`SupportPalette`) | `bubbleOwn: theme.colors.accent`, `textOnOwn: theme.colors.palette.white` | `bubbleOwn: theme.colors.elevated`, `textOnOwn: theme.colors.text` | Every outgoing chat message was a moss bubble — a conversation with 2+ merchant messages guaranteed multiple simultaneous moss elements. `primary`/`onPrimary` (Send button, submit button, active reason chip) **kept** moss — that's the single-CTA-per-state usage the rule allows. `elevated` (pure white) was chosen over `surfaceAlt` so the own-bubble still reads distinct from the other-party bubble, which already uses `palette.surface` (= `surfaceAlt`) with a border. |
| `app/(tabs)/products/[id].tsx:329-338` (Active `Switch`) | `trackColor.true: theme.colors.accent` | `trackColor.true: theme.colors.text` | This screen's header `Save` action is already the one moss accent (`rightSlot` `Text color="accent"`); the Switch's on-state was a second live moss element at rest next to it — the exact violation the whole-branch review flagged in the code comment at line ~209. |

No test assertions referenced any of the changed color values (verified via grep across
`__tests__/` before editing), so no test files needed updates for this section.

---

## C. Small a11y + polish nits

- **`app/(tabs)/products/[id].tsx:247-253`** — the empty-thumbnail placeholder (`Package` icon
  branch, no product media) had no accessibility exposure. Added `accessible` +
  `accessibilityLabel="No product image"` to the wrapping `View`.
- **`components/products/CreateNextStepsBanner.tsx:86`** — chip `minHeight: 36` → `minHeight:
  theme.touchTarget` (44), meeting the 44pt minimum touch target used everywhere else in the app.
  No test asserted the old `36` value (verified in `__tests__/create-next-steps-banner.test.tsx`
  — it only checks text/press behavior, not styles), so no test changes were needed.

---

## Gates

**Full test suite** (`npx jest`, from `apps/mobile-admin/`):

```
Test Suites: 47 passed, 47 total
Tests:       346 passed, 346 total
Snapshots:   0 total
Time:        5.596 s
```

Baseline (346/346) held — no test file needed changes for this batch (no test asserted an old
moss color or the old `36` chip height).

**TypeScript** (`npx tsc --noEmit --pretty false 2>&1 | grep -c "error TS"`):

- `apps/mobile-admin`: `0`
- `packages/mobile-shared`: `0`

Both match the required baseline of 0.

---

## Files touched

- `apps/mobile-admin/app/login.tsx`
- `apps/mobile-admin/components/auth/LinkAccountPrompt.tsx`
- `apps/mobile-admin/components/DashboardStats.tsx`
- `apps/mobile-admin/components/CustomerRow.tsx`
- `apps/mobile-admin/app/(tabs)/customers/[id].tsx`
- `apps/mobile-admin/app/(tabs)/more/security.tsx`
- `apps/mobile-admin/app/(tabs)/more/notifications.tsx`
- `apps/mobile-admin/app/(tabs)/more/support.tsx`
- `apps/mobile-admin/app/(tabs)/products/[id].tsx`
- `apps/mobile-admin/components/products/CreateNextStepsBanner.tsx`

No changes to `node_modules`, Metro/Babel/tsconfig config, or any test file. No page
layout/centering or animation changes — those remain Batch 3 / Batch 4 scope.
