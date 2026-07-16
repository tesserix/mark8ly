# Fix Batch 3 — asymmetric recomposition of three centered layouts

Scope: recompose three centered/uniform-grid compositions in mobile-admin into left-aligned,
asymmetric editorial layouts, per the design system rule "Asymmetric, never centered — left-aligned
text, asymmetric grids, varied vertical rhythm. Centered heroes and same-sized card grids are AI-slop
tells we explicitly reject." Behaviour, data, props, and handlers are unchanged in all three files —
only composition/alignment/hierarchy moved.

## 1. `components/DashboardStats.tsx`

**Before:** a uniform 2×2 grid of equal-size, bordered `Card`s (`StatCard`) — each with a centered-feel
eyebrow/value/trend stack — plus a bordered "compact strip" of 4 equal-width, `alignItems: "center"`
columns (`CompactStat`) for order counts. This was the textbook "same-sized card grid" anti-pattern:
every stat given identical visual weight regardless of importance.

**After:** left-aligned, asymmetric stat block with three tiers of visual weight:
- **Hero** — "This month" revenue promoted to a `preset="display"` serif numeral (36px), left-aligned,
  with its eyebrow label above and its trend indicator (arrow + %) beneath. This is the same metric/trend
  that previously lived in one of the four equal `StatCard`s (`revenue_month` + `revenue_change_pct`) —
  now it's the singular headline number instead of one-of-four identical boxes.
- **Secondary list** — `Today` / `This week` / `Customers` rendered as a left-aligned, hairline-separated
  list (`StatRow`: sans label left, serif `h3` numeral right of the row) instead of bordered cards.
- **Orders list** — `Today` / `Pending` / `Fulfilled` / `Cancelled` (previously the centered compact strip)
  now render as a second hairline-separated list under an "Orders" eyebrow, same `StatRow` shape.
- All `Card` usage and bordered/background boxes removed in favor of `Hairline` rules.
- Every `alignItems: "center"` used for centering content, and the card-grid shape, is gone. (`row`'s
  `alignItems: "center"` that remains is cross-axis vertical centering of a label vs. a taller numeral in
  a horizontal row — not text/content centering.)

**Serif numerals:** hero value (`preset="display"`), all `StatRow` values (`preset="h3"`, serif) — matches
"editorial numerals" guidance. Labels use `preset="body"`/`"eyebrow"` (sans).

**Data preserved:** `revenue_today`, `revenue_week`, `revenue_month`, `revenue_change_pct`,
`customers_total`, `orders_today`, `orders_pending`, `orders_fulfilled`, `orders_cancelled` — same 9
fields, same formatting (`formatCurrency`), same trend arrow/percentage logic.

**Consumer:** `app/(tabs)/index.tsx` renders `<DashboardStats stats={data.stats} />` inside a
`contentPad` wrapper that already applies horizontal padding — no change needed there.

**Tests touched:** none exist for this component; none needed updating.

## 2. `app/login.tsx`

**Before:** `<View className="flex-1 justify-center px-6">` — the whole form (wordmark, subhead, fields,
buttons, provider links) vertically centered in the viewport as a "hero."

**After:** `<View className="flex-1 px-6 pt-16">` — `justify-center` removed, `pt-16` added so content is
anchored toward the upper area with generous top whitespace instead of floating mid-screen. Removing
`justify-center` reverts the column's `justifyContent` to its RN default (`flex-start`), so content now
reads top-down: eyebrow → serif `display` wordmark → subhead → fields → sign-in CTA → divider → social
buttons, all left-aligned (unchanged — `Text` defaults to left, no `align="center"` was ever set here).

Everything else is untouched: `KeyboardAvoidingView` + `ScrollView` wrapper, all auth handlers
(`handleSignIn`, `handleGoogleSignIn`, `handleAppleSignIn`), field props (`textContentType`,
`secureTextEntry`, `autoComplete`, etc.), error/notice handling, `LinkAccountPrompt` integration, and the
one-accent rule (primary CTA is `bg-ink`/ink, not moss; only the "Merchant admin" eyebrow uses
`text-moss`, unchanged from before this batch).

**Tests touched:** none. Ran the full `__tests__/login.test.tsx` suite (16 cases covering wordmark render,
sign-in invocation, error mapping, in-flight state, Google/Apple sign-in, link-prompt flow, and
involuntary-sign-out notices) — all still pass because every assertion queries by accessibility
label/text content, none of which changed.

## 3. `app/(tabs)/customers/[id].tsx`

**Before:** the profile header (`styles.profile` with `alignItems: "center"`) stacked the avatar, name
(`h1`, `align="center"`), email (`align="center"`), phone (`align="center"`), and "Joined" caption
(`align="center"`) all centered in a single vertical column.

**After:** left-aligned, two-part header:
- **Identity row** — avatar (unchanged 72×72 neutral `surfaceAlt` fill, no moss) on the left, and a
  column to its right with the customer name in serif (`preset="h2"`, was `h1` — reduced one step since
  it now sits beside the avatar rather than alone) and the email in sans body beneath it, plus the
  "Blocked" `StatusBadge` if applicable. All `align="center"` removed; everything reads left-to-right.
- **Info list** — phone (if present) and "Joined {date}" moved into a new `InfoRow` component (modeled on
  the existing `InfoRow` pattern in `app/(tabs)/orders/[id].tsx`: caption label + body value in a row) and
  presented as a left-aligned list separated by a `Hairline`, replacing the centered vertical stack of
  captions. Unlike the orders/[id] version (which right-aligns the value), this list's value is left-aligned
  per the batch spec ("left-aligned info list").

**Out of scope, left untouched:** the `StatTile` row (Orders/Spent/Avg) below the header is centered
inside bordered cards and is itself a "same-sized card grid" — but the batch instructions explicitly
scoped this file's recomposition to "the profile header (avatar, name, email, phone, joined)" only, so it
was not touched to avoid scope creep beyond the assignment.

**Data/behaviour preserved:** `displayName`, `initial`, `customer.email`, `customer.phone` (conditional),
`formatDate(customer.created_at)`, `isBlocked` → `StatusBadge`, and all loading/error/empty states
(unchanged — the `error` and `isLoading || !customer` early returns were not touched).

**Tests touched:** none exist for this screen (`__tests__/schemas-customers.test.tsx` covers the API
schema, not this component); none needed updating.

## Gates

- `npx jest` (full suite): **346/346 passed**, 47 suites, matches the stated baseline. Tail:
  ```
  Test Suites: 47 passed, 47 total
  Tests:       346 passed, 346 total
  Snapshots:   0 total
  Time:        5.173 s
  Ran all test suites.
  ```
  (One pre-existing unrelated warning: "A worker process has failed to exit gracefully" — not new,
  unrelated to these three files.)
- `apps/mobile-admin`: `npx tsc --noEmit --pretty false 2>&1 | grep -c "error TS"` → **0**
- `packages/mobile-shared`: `npx tsc --noEmit --pretty false 2>&1 | grep -c "error TS"` → **0**

No dependencies added, no animation added, no token values changed, only `lib/theme.ts` tokens and
`@/components/ui` primitives (`Text`, `Hairline`, `StatusBadge`, `BackHeader`, `Screen`) used throughout.
