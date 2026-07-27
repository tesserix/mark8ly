# mobile-admin — Native UX Re-architecture

**Date:** 2026-07-27
**Status:** Design approved, ready for planning
**Scope:** `apps/mobile-admin`, `packages/mobile-shared`, plus three fields in `services/marketplace-api`

---

## Problem

The app reads as a scaled-down web app rather than a native mobile app. This was first
observed 2026-07-25 while capturing App Store screenshots on an iPhone 17 Pro Max simulator
and deferred to a next-release design pass. A code read on 2026-07-27 confirmed the cause is
not the visual identity — the Paper·Ink·Moss editorial system is intentional, on-brand, and
stays. The cause is **metrics and behaviour**:

1. **Body text is 14 pt** (`lib/theme.ts:167`). iOS body is 17 pt, Android body-large is 16 sp.
   Every list row's primary line is set at a desktop table size.
2. **Rows are 56–76 pt with 16×12 padding** (`app/(tabs)/index.tsx:351`) — shallow, dense,
   hairline-divided; a responsive web table.
3. **`activeOpacity={0.6}` across 41 files.** A whole-row 60 % fade is the loudest
   React-Native-styled-like-the-web signal in the app.
4. **Headers are static `<View>`s** (`components/ui/PageHeader.tsx`). Nothing collapses on scroll.
5. **Zero row gestures** anywhere, though `react-native-gesture-handler` 2.31 is installed.
6. **Haptics in 7 files, all inside sheets.** Nothing on tab change, filter change, or a status change.
7. **`Image` from react-native, not `expo-image`** (`components/ProductRow.tsx:36`) — thumbnails pop in.
8. **Dashboard runs out of content**, leaving the bottom third of a 6.9″ canvas empty.

A second problem surfaced during design review: the app **gives the eye nothing to hold**.
Every surface sits at one value, one weight, one distance — no foreground, no background.
It is an admin app for a shop with no photography on its home screen.

The floating dock was previously judged the most native-feeling part of the app. On closer
review it is not: four grey outline icons and one solid-ink lozenge, at 20 pt icons in a 52 pt
bar, with an active pill whose width changes per tab. It is being redesigned.

## Locked decisions

| Decision | Choice |
|---|---|
| Scope | Full native re-architecture |
| Brand vs. native idiom | **Brand wins, native motion.** Serif titles, solid Paper, hairlines, no blur. Native *behaviour* only — collapse-on-scroll, springs, gestures, haptics. |
| Jobs served | All four: triage/fulfil orders, check numbers, fix catalogue, respond to people |
| npm dependencies | **Zero new packages.** Everything is built from what is installed. |
| Dashboard | Becomes an **action queue** — compressed metrics band, then everything waiting on you |
| Sequence | **Spine → Slice → Rollout** (mechanical global pass, then design-heavy pilot, then apply) |
| Dock | **Ink** — solid `#0E0E0C` bar, paper icons, labelled tabs |
| Surface | **Depth and photography** — one elevated card, real chart, product imagery, harder serif |
| API gap | **Add the three missing fields** to marketplace-api rather than work around them |

## Non-goals

- No change to the visual identity. Paper·Ink·Moss, Source Serif 4 + Source Sans 3 stay.
- No new features. Nothing here adds a capability the app doesn't already have.
- No navigation restructure. The five tabs stay as they are.
- No dark mode. mobile-admin is light-only and remains so.
- No stock-iOS chrome. No translucent navbars, no system segmented controls, no blur.
- No web-admin changes. This is the mobile client only (plus the shared API fields).

## Delivery increments

The three steps are separately shippable and should be separate releases. Each leaves the app
in a coherent state on its own — none of them depends on a later step to look finished.

| Increment | Ships | Depends on |
|---|---|---|
| **1 — Spine** | Type scale, density, press feedback, haptics, images. App-wide. | — |
| **API** | Three dashboard fields in marketplace-api. | — (can ship any time, including first) |
| **2 — Slice** | Ink dock, collapsing header, Dashboard queue, Orders gestures. | Increment 1. Renders correctly with or without the API fields. |
| **3 — Rollout** | The pattern applied to the remaining screens. | Increment 2 (uses its primitives). |

---

## Step 1 — The spine

Global and mechanical. No layout re-architecture. Applies app-wide in one pass so no screen
is ever left half-migrated. **This step alone closes the 2026-07-25 observation** and is
independently shippable if Step 2 needs another design round.

### 1.1 Type scale

Both token sources move together and must keep agreeing: `apps/mobile-admin/lib/theme.ts`
(`theme.text.*`) and `apps/mobile-admin/tailwind.config.js` (`fontSize`). Families, weights,
and the serif/sans split are unchanged — only sizes and line heights move.

| Preset | Now | New | Rationale |
|---|---|---|---|
| `heroNumeral` *(new)* | — | 44 / 48, `-0.8` | Home revenue figure only |
| `display` | 36 / 42, `-0.5` | 40 / 46, `-0.6` | Other display numerals |
| `h1` | 28 / 34, `-0.3` | 30 / 36, `-0.4` | Large title |
| `h2` | 22 / 28, `-0.2` | 24 / 30, `-0.25` | Stat-strip numerals |
| `h3` | 18 / 24 | 20 / 26 | Section and sheet titles |
| `bodyLg` | 16 / 22 | 19 / 26 | Emphasis body |
| `body` | 14 / 20 | **17 / 24** | iOS body / Android body-large |
| `bodyEmphasis` | 14 / 20, 600 | **17 / 24, 600** | Row primary line |
| `label` | 13 / 18 | 15 / 20 | iOS subhead — form labels |
| `caption` | 12 / 16 | 13 / 18 | iOS footnote — metadata |
| `eyebrow` | 11 / 14, `+1.2` | 12 / 16, `+1.2` | Section rules |
| `mono` | 13 / 18 | 15 / 20 | Order numbers, SKUs |

`heroNumeral` is a new preset in both `theme.text` and the `Text` component's
`PRESET_CLASSES`. It exists so the Home revenue figure can carry the screen without
inflating `display` everywhere it is already used.

### 1.2 Density

| Surface | Now | New |
|---|---|---|
| Single-line row min height | 56 | 64 |
| Two-line row min height | ~76 | 88 |
| Row padding (h × v) | 16 × 12 | 20 × 14 |
| List thumbnail | 52 | 60 |
| Compact thumbnail *(new)* | — | 38 |
| Screen horizontal gutter | 16 | 20 |
| Minimum touch target | 44 | 44 (unchanged) |

`theme.touchTarget` stays at 44. Rows now comfortably exceed it rather than the rule being
relaxed.

### 1.3 Press feedback

Every `TouchableOpacity` in the app (41 files) becomes a `Pressable`. `activeOpacity` is
removed entirely — no opacity-based press feedback survives anywhere.

- **iOS:** background shifts to `theme.colors.sink` (`#ECEAE3`) while pressed.
- **Android:** `android_ripple` with ink at 12 % opacity, borderless where the target is an icon.

`theme.colors.sink = "#ECEAE3"` is added to `lib/theme.ts` (the value already exists in
`tailwind.config.js` as `paper.sink`; the two token sources are being reconciled, not extended).

### 1.4 Haptics

`packages/mobile-shared/haptics/feedback.ts` already exists but its trigger names are
storefront-shaped (`addToCart`, `checkoutStep`, `wishlistToggle`). An **admin trigger set** is
added to the same module — the storefront names are left untouched so the storefront app is
unaffected.

| Moment | Trigger | expo-haptics call |
|---|---|---|
| Tab change · filter chip · segment change | `selectionChanged` | `selectionAsync()` |
| Swipe crosses the action threshold | `swipeThreshold` | `impactAsync(Light)` |
| Long-press opens the action sheet | `menuOpen` | `impactAsync(Medium)` |
| Order fulfilled · review approved · save succeeded | `actionSucceeded` | `notificationAsync(Success)` |
| Action failed · validation blocked | `actionFailed` | `notificationAsync(Error)` |

All calls are fire-and-forget and must not block the interaction or reject into an unhandled
promise. The module swallows platform errors (haptics are unavailable on some Android devices
and on simulators) and logs at debug level.

### 1.5 Images

`components/ProductRow.tsx` and every other `Image` usage moves to `expo-image`, which is
already a dependency and currently used only in `ProductMediaPicker`.

- `transition={200}` fade-in
- `contentFit="cover"`
- `placeholder` → sink-coloured field with the existing `Package` lucide glyph
- `recyclingKey` set to the item id so FlatList recycling doesn't flash the wrong image

A shared `components/ui/Thumb.tsx` wraps this so the placeholder, radius, and failure
behaviour are defined once.

---

## Step 2 — The slice

The design-heavy pilot. Dashboard, Orders, and the Dock. Sign-off happens here; Step 3 is
application, not new decisions.

### 2.1 Dock — Ink

`components/navigation/Dock.tsx` is rewritten. The floating, detached geometry is kept; the
fill, proportions, and active-state mechanic change.

| Property | Now | New |
|---|---|---|
| Bar height | 52 | 64 |
| Corner radius | 26 | 30 |
| Fill | `#FFFFFF` + hairline border | `#0E0E0C`, no border |
| Shadow | `0 4 16 / 0.10` | `0 10 26 -8 / 0.45` |
| Icon size | 19 active / 22 inactive | 24, uniform |
| Labels | Active tab only | **All five tabs**, 11 pt / 650 |
| Active mechanic | Ink pill, width varies per tab | Filled paper icon + 100 % paper label |
| Inactive | Outline icon, `#5C5953` | Outline icon + label at `rgba(247,246,242,0.60)` |

Rationale for each change: icon-only inactive tabs are a guess, not navigation; a
width-changing pill makes every sibling slot shift sideways on tab change; and 19–22 pt icons
in a 52 pt bar leave it visually empty.

**Contrast:** `rgba(247,246,242,0.60)` composited on `#0E0E0C` resolves to ≈ `#9A9996`,
which is ≈ 6.8:1 against the bar — clears WCAG AA for the inactive labels. Do not lower this
alpha; at 0.40 it resolves to ≈ `#6B6A67` and drops to ≈ 3.6:1, which fails.

**Deliberate change from the approved mockup:** the moss dot under the active tab is
**dropped**. The active state is already fully carried by the filled icon plus the
full-opacity label, and a persistent moss mark in the chrome would put a second moss element
in view whenever a screen also shows a moss primary action — violating one-accent-per-view.
If the dot is wanted back, it is a one-line addition and the contrast is fine (8.5:1), but the
accent rule then needs an explicit carve-out.

Motion: the active-state crossfade keeps the existing 220 ms ease-out-quart
(`Easing.bezier(0.22, 1, 0.36, 1)`) and stays gated on `useReducedMotion()`.

### 2.2 Collapsing header

New primitive: `components/ui/CollapsingHeader.tsx`. Replaces `PageHeader` on scrolling
screens; `PageHeader` itself is retained for non-scrolling and modal screens.

- **Expanded** (scroll offset 0): eyebrow, serif `h1` title, optional right slot.
- **Collapsed** (offset ≥ 64): compact bar — `h3` serif title, `caption` subtitle beneath,
  right slot; hairline bottom rule.
- Driven by a `useAnimatedScrollHandler` shared value and `interpolate`, not by re-render.
- Surface is **solid Paper with a hairline** — not a blur, not a translucent material.
- The whole transition is skipped when `useReducedMotion()` is true; the collapsed state is
  rendered immediately at any non-zero offset.

### 2.3 Dashboard — action queue

`app/(tabs)/index.tsx` is restructured.

**Header.** Store name as the serif title (it is the merchant's shop — name it), date as the
caption above it, tenant monogram avatar as the right slot with the notification badge.
Collapses per 2.2.

**Metrics band.** One elevated white card on the Paper ground:
- `heroNumeral` revenue figure for the month, with `↗ 12.4 %` as a moss-tint badge
- `caption` line: today's and this week's revenue
- **`components/ui/RevenueChart.tsx`** — a 104 pt area chart built on `react-native-svg`
  (already a dependency): moss-tint fill, moss 2.25 pt stroke, three faint gridlines, and an
  emphasised endpoint dot with a soft halo. Replaces the existing 74 × 20 `Sparkline`.
- Hairline, then the four-up order strip (Today / Pending / Fulfilled / Cancelled) at `h2` serif

This is **exactly one card**. It is a deliberate, bounded exception to the design system's
"hairline rules between sections, not bordered cards" — spent on the single thing worth
elevating. Everything below it is hairline-separated rows on the Paper ground. Do not
generalise this into a card grid.

**Needs-you queue.** A single prioritised list filling the remaining canvas, composed from
four sources:

| Type | Source | Leading swipe (drag right) | Trailing swipe (drag left) |
|---|---|---|---|
| Order (pending) | `dashboard.recent_orders` filtered to `status === "pending"` | Approve | Cancel |
| Review (pending) | `reviews?status=pending&limit=4` | Approve | Reject |
| Low stock | `dashboard.low_stock` | — | — |
| Ticket (open) | `tickets?status=open&limit=4` | — | Close |

Ordering is by urgency then recency: pending orders first (money waiting), then low stock
(sales at risk), then unanswered tickets, then pending reviews. Each type is capped at
**3 rows**; when a type exceeds its cap, a "See all N" row is appended for that type. Total
queue is capped at 12 rows.

The per-type `limit=4` fetches one more than the cap so overflow is detectable without a
second request. `N` in "See all N" uses the authoritative total where one exists —
`stats.orders_pending` for orders and `stats.pending_reviews` for reviews — and otherwise the
fetched length, rendered as "See all" with no number rather than a number that might be wrong.

Each row carries: a 60 pt product thumbnail (or the customer monogram disc where no product
image applies), a 17 pt primary line, a 13 pt secondary line, and a right column with the
serif amount and a typed badge.

**Empty state.** When the queue is empty, the remaining canvas becomes a serif "All clear"
editorial moment — not blank paper, and not an apology.

**Composition module:** `lib/queue.ts` exports a pure `buildQueue(sources): QueueItem[]`
taking the four already-fetched payloads and returning the sorted, capped, typed list. Keeping
it pure and separate from the screen makes the ordering and capping rules unit-testable
without rendering.

### 2.4 Orders

`app/(tabs)/orders/index.tsx`.

- **Header** collapses per 2.2. Right side shows the pending count.
- **Search** moves from a permanently pinned field into the scroll content, revealed by
  pulling down — it stops consuming ~56 pt of every screen. Implemented as a list header with
  the FlatList's initial `contentOffset` set past it.
  *If this proves undiscoverable in device testing, the revert is to pin it — one prop.*
- **Filters** change from an underlined `SegmentedControl` to 40 pt pill chips in a horizontal
  `ScrollView`. Each chip is a real touch target and more statuses fit. `selectionChanged`
  haptic on change.
- **Rows** at 88 pt: order number + customer name at 17 pt/600, status badge right; serif
  tabular total at `h3` and relative time on the second line.
- **Swipe:** dragging the row **rightward** reveals **Approve** (moss) at the leading edge;
  dragging **leftward** reveals **Cancel** (danger) at the trailing edge. This is the platform
  convention — leading is constructive, trailing is destructive — and it applies to every
  swipe surface in the app, including the Home queue. (The round-two mockup showed both
  actions on the trailing edge; that was a rendering shortcut, not the spec.) Threshold at
  40 % of row width, with the `swipeThreshold` haptic fired once per crossing.
- **Long-press** opens a Paper action sheet (`@gorhom/bottom-sheet`, already installed) with
  the full set: Fulfil, Email label, Refund, Cancel. This is the zero-dependency answer to a
  native context menu, and it matches the system anyway — solid surface, hairline rules, no blur.

### 2.5 New shared primitives

Each has one purpose, a documented interface, and is testable in isolation.

| File | Purpose | Depends on |
|---|---|---|
| `components/ui/PressableRow.tsx` | Row press surface with platform-adaptive feedback | `Pressable`, theme |
| `components/ui/SwipeRow.tsx` | Horizontal swipe container with left/right action sets, spring return, threshold haptic | gesture-handler, reanimated, haptics |
| `components/ui/CollapsingHeader.tsx` | Scroll-driven serif header collapse | reanimated |
| `components/ui/ActionSheet.tsx` | Long-press action menu on a Paper sheet | `@gorhom/bottom-sheet`, haptics |
| `components/ui/RevenueChart.tsx` | Area chart with fill, gridlines, emphasised endpoint | `react-native-svg` |
| `components/ui/Thumb.tsx` | expo-image thumbnail with placeholder and failure state | `expo-image` |
| `components/dashboard/QueueRow.tsx` | One typed needs-you row | PressableRow, SwipeRow, Thumb |
| `lib/queue.ts` | Pure queue composition, ordering, capping | — |

`SwipeRow` takes `leftActions` / `rightActions` as arrays of
`{ key, label, icon, tone, onPress }` and owns none of the business logic — the caller
supplies handlers. This keeps it reusable across orders, reviews, tickets, coupons, and
products without any of them leaking into it.

---

## Step 3 — Rollout

Applying the proven pattern. Every screen gets the same four things: collapsing header,
deeper rows, native press feedback, and — where there is an obvious primary action — a swipe.

Swipe edges follow the same convention throughout: **leading (drag right) is constructive,
trailing (drag left) is destructive or dismissive.**

| Screen | Leading swipe | Trailing swipe | Long-press sheet |
|---|---|---|---|
| Products list | Activate | Set to draft | Edit price · Adjust stock · Duplicate · Archive |
| Customers list | — | — | Email · Call · Block |
| Reviews | Approve | Reject | Reply · Approve · Reject · Report |
| Tickets | — | Close | Reply · Assign · Close |
| Coupons · Gift cards | Enable | Disable | Edit · Duplicate · Delete |
| Campaigns · Segments | — | — | Edit · Duplicate · Delete |
| Order detail | — | — | Actions move into a sticky bottom action bar |
| Product editor · create | — | — | Density and type only; sheet behaviour unchanged |
| More · Account · Settings | — | — | Grouped inset lists at native row height |

---

## API changes

Three fields, in `services/marketplace-api`, admin dashboard DTO
(`internal/handlers/admin/dashboard.go` and its response structs):

| Struct | Field | Type | Why |
|---|---|---|---|
| `recent_orders[]` | `customer_name` | `*string` (omitempty) | Rows currently can only show an email address |
| `recent_orders[]` | `image_url` | `*string` (omitempty) | First line item's product image, for the queue thumbnail |
| `low_stock[]` | `image_url` | `*string` (omitempty) | Product image for the queue thumbnail |

Client side, `packages/mobile-shared/api/schemas/dashboard.ts` adds all three as
`.optional()` — **not** `.nullable()`, matching the repo's established rule that a Go
`omitempty` pointer is absent from the JSON rather than null.

The mobile client must render correctly **before and after** this ships: a missing
`customer_name` falls back to `customer_email`, and a missing `image_url` falls back to the
monogram disc. This is not a temporary shim — merchants can have orders whose products were
since deleted, so both fallbacks are permanent behaviour.

---

## Error handling

- **Image load failure** → `Thumb` renders the sink-field placeholder. Never a broken-image
  box, never an empty gap that changes row height.
- **Swipe action failure** → the row springs back to rest, `actionFailed` haptic fires, and an
  inline error message replaces the row's secondary line for 4 seconds. The optimistic
  react-query update is rolled back in `onError`.
- **Queue partial failure** → each of the four sources is an independent query. If one fails,
  the other three still render and a single muted row states which section could not load,
  with a retry. The screen never fails wholesale because tickets timed out.
- **Empty queue** → the "All clear" state, distinct from the error state and from loading.
- **Reduced motion** → every new animation (header collapse, swipe spring, sheet entrance,
  chart draw-in, dock crossfade) is gated on `useReducedMotion()`, matching the existing
  animations in the app.
- **Haptics unavailable** → swallowed and logged at debug; never surfaces to the user, never
  rejects unhandled.

---

## Testing

Current gates: **352 jest tests / 48 suites** in mobile-admin, **83 vitest** in mobile-shared,
and `tsc --noEmit` clean in both. All must stay green at every step. New assertions are
added, never loosened.

**New unit tests**
- `Text` renders each preset at its specified size — extends the existing `Text.test.tsx`,
  and covers the new `heroNumeral` preset.
- Row primitives meet their minimum heights (64 single-line, 88 two-line).
- `lib/queue.ts` — ordering by urgency, per-type cap of 3, total cap of 12, "See all" row
  appended only when a type exceeds its cap, empty input → empty output.
- Admin haptics triggers each call the correct `expo-haptics` function (mocked), and a
  throwing platform call does not reject.
- `SwipeRow` — action fires past threshold, does not fire below it, springs back on release,
  fires the threshold haptic exactly once per crossing.
- `CollapsingHeader` — collapsed at offset ≥ 64, expanded at 0, collapsed immediately when
  reduced motion is on.
- `Dock` — renders five labelled tabs, `accessibilityState.selected` set on the active tab only.
- Dashboard schema accepts payloads both with and without the three new optional fields.

**Regression guard**
A token test asserting the banned text colours never reappear in `lib/theme.ts` or
`tailwind.config.js`: `rgba(14, 14, 12, 0.5)` and `#7A766E` both fail 4.5:1 on Paper and were
removed in the 2026-07-17 pass.

**Device verification** (each step, iOS simulator + one Android emulator)
The 2026-07-17 pass left the product editor, product create, and customer detail screens
unverified because they are reachable only by tapping a row — deep links do not push nested
routes, only the five tab roots. Those three screens need human taps this time.

---

## Guardrails — must not regress

From the 2026-07-17 design pass. These are gates, not preferences; a sweeping re-scale is
exactly the kind of change that quietly undoes them.

1. **Contrast.** Never reintroduce `rgba(14, 14, 12, 0.5)` or `#7A766E` as text colour — both
   fail 4.5:1 on Paper. Tertiary text stays `#5C5953` in both token sources.
2. **One accent per view.** Moss is never decorative. On the new Dashboard it is spent on the
   chart fill/stroke/endpoint and the Approve swipe — nowhere else.
3. **Badges.** Success stays a moss *tint* (`#E8EEE2` / `#2D4A2B`), never a solid moss fill.
   Warning stays bronze-on-amber-tint (`#7A4A0F` / `#F4E6CB`), never white-on-amber.
4. **No centered heroes.** Left-aligned, asymmetric. The new metrics band keeps this.
5. **No glassmorphism.** The collapsing header and all sheets are solid Paper with hairlines.
6. **Reduced motion.** Every animation gates on `useReducedMotion()`.
7. **Touch targets.** 44 pt minimum holds.
8. **Radii.** `theme.radii.md` and tailwind `rounded-md` both stay at 6 px — they were
   deliberately reconciled and must not drift apart again.
9. **Modal sheet trap.** A `presentation:"modal"` screen is presented above the root
   `BottomSheetModalProvider`, so any sheet it mounts portals behind it. `products/new` is the
   only modal screen; any sheet added there needs its own local provider, and its footer must
   use `useSafeAreaInsets().bottom`, not `useDockClearance()`.
10. **Lockfile.** The root lockfile cannot be regenerated locally — `npm install` collapses the
    deliberate multi-version tree and breaks mobile-admin. Since this design adds zero npm
    dependencies, no lockfile edit should be needed at all; if a task appears to need one,
    that task is wrong.

---

## Risks and open trade-offs

**Unified queue vs. typed sections.** Home mixes orders, reviews, stock, and tickets in one
list. It fills the canvas and serves all four jobs, but a merchant with 200 pending orders may
find it noisy. Mitigated by the per-type cap of 3 and the total cap of 12. If it still reads as
busy on device, the fallback is typed sections with the same caps — a change to `lib/queue.ts`
and the section rendering only, not to the row components.

**Pull-to-reveal search on Orders.** Native-correct but discoverable by convention only.
If merchants search often, pinning the field is the better trade and is a one-prop revert.

**One elevated card.** Knowingly bends "hairline rules, not bordered cards". Bounded to a
single card on a single screen. If it reads as inconsistent once built, the chart moves to the
Paper ground between two hairlines.

**Ink dock.** A dark bar at the foot of a light app is a commitment. It is the one place the
design is deliberately loud, and it keeps moss free for on-screen actions.

**Backend coupling.** Step 2's photography depends on the three API fields. The client is
specified to degrade gracefully, so mobile work is not blocked on the deploy — but Home will
look closer to the flat mockup until it lands.
