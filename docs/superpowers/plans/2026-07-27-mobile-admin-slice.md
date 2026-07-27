# mobile-admin Native UX — Increment 2 (The Slice) Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Turn the Dashboard into an action queue, give Orders native gesture triage, and land the collapsing serif header — the screens from the approved mockup (`https://claude.ai/code/artifact/d064f7df-51a8-4900-bb5b-e5c71dc493f8`).

**Architecture:** Four new primitives (`CollapsingHeader`, `SwipeRow`, `ActionSheet`, `RevenueChart`) plus one pure module (`lib/queue.ts`), then two screen rewrites on top of them. Everything is built from packages already installed — reanimated 4.3, gesture-handler 2.31, `@gorhom/bottom-sheet` 5.2, `react-native-svg` 15.15.

**Tech Stack:** React Native 0.85.3 · Expo SDK 56 · expo-router 56 · NativeWind 4.2.5 · reanimated 4.3 · gesture-handler 2.31 · @gorhom/bottom-sheet 5.2 · react-native-svg 15.15

## Status of increment 1

Complete and on `main`. Available to you: the native type scale (`body` 17pt, `heroNumeral` 44pt), density tokens (`theme.row`, `theme.thumb`), press tokens (`theme.press`), `PressableRow`, `Thumb`, `adminHaptics`, and the Ink dock. The dashboard API now returns `customer_name` / `image_url` on `recent_orders` and `product_id` / `image_url` on `low_stock`.

---

## Global Constraints

- **🔴 NEVER pass a function as a `style` prop to `Pressable`.** Under NativeWind 4.2.5's JSX interop a function style is not resolved like a plain array and **the styles are silently dropped at runtime**. This shipped in increment 1 and left 24 call sites rendering with zero styles — no padding, no borders, `flexDirection` falling back to `column`. Use a plain array plus explicit `useState` press tracking driven by `onPressIn`/`onPressOut`, exactly as `components/ui/PressableRow.tsx` does. **Unit tests cannot catch this** — RNTL renders without the NativeWind runtime, so the resolved array looks correct in jest and is wrong on device.
- **Every task ends with a device screenshot.** `xcrun simctl io booted screenshot <path>`. Metro runs on **port 8082** (8081 is held by Docker). Do not mark a task done on green tests alone — increment 1 proved that green tests and a broken screen coexist happily.
- **Zero new npm dependencies.** The root lockfile cannot be regenerated locally.
- **Two token sources must agree:** `apps/mobile-admin/lib/theme.ts` and `apps/mobile-admin/tailwind.config.js`.
- **Banned text colours:** `rgba(14, 14, 12, 0.5)` and `#7A766E`. Tertiary is `#5C5953`. A guard test enforces this.
- **One accent per view.** Moss `#2D4A2B` is never decorative. On the new Dashboard it is spent on the chart fill/stroke/endpoint and the Approve swipe — nowhere else.
- **Badges:** success is moss *tint* (`#E8EEE2`/`#2D4A2B`), never solid moss fill. Warning is `#7A4A0F` on `#F4E6CB`.
- **Swipe convention, app-wide:** dragging a row **right** reveals the **constructive** action (Approve, moss) at the leading edge; dragging **left** reveals the **destructive** action (Cancel, danger) at the trailing edge.
- **No glassmorphism.** The collapsing header is solid Paper with a hairline — never a blur.
- **No centered heroes.** Left-aligned, asymmetric.
- **Reduced motion:** every animation gates on `useReducedMotion()`.
- **Minimum touch target 44pt**, as a real `minWidth`/`minHeight` box rather than `hitSlop` — an
  invisible overlay can overlap siblings. **Exception (amended 2026-07-27):** a small badge or control
  **overlaying another element** (e.g. the remove badge on a media thumbnail) keeps its small visible
  size and uses `hitSlop` instead. A 44pt visible box there eats the element beneath it and has to
  bleed onto neighbours to sit in a corner at all. When using `hitSlop`, compute the expanded region
  against the actual sibling gap and prove it does not overlap — a too-large tap area on a media
  thumbnail deletes the *next* photo.
- **Modal sheet trap:** a `presentation:"modal"` screen is presented above the root `BottomSheetModalProvider`, so any sheet it mounts portals *behind* it. `app/(tabs)/products/new.tsx` is the only modal screen; a sheet added there needs its own local provider and must use `useSafeAreaInsets().bottom`, not `useDockClearance()`.
- **Gates:** `npm test` + `npm run check-types` in `apps/mobile-admin` (baseline 67 suites / 462 tests, tsc clean) and in `packages/mobile-shared` (95 vitest).
- **Commit style:** conventional commits, single-line messages, no signature, no co-author trailer.

---

## File Structure

**Created**

| File | Responsibility |
|---|---|
| `components/ui/CollapsingHeader.tsx` | Scroll-driven serif header collapse |
| `components/ui/SwipeRow.tsx` | Horizontal swipe container with leading/trailing action sets |
| `components/ui/ActionSheet.tsx` | Long-press action menu on a Paper sheet |
| `components/dashboard/RevenueChart.tsx` | Area chart — fill, gridlines, emphasised endpoint |
| `components/dashboard/QueueRow.tsx` | One typed "needs you" row |
| `lib/queue.ts` | Pure queue composition, ordering, capping |
| `components/ui/IconButton.tsx` | The 41-site icon-button idiom, extracted |

**Modified**

| File | Change |
|---|---|
| `components/ui/PressableRow.tsx` | Widen API: `disabled`, `accessibilityState`, `accessibilityRole`, `accessibilityHint`, `ripple` |
| `components/ui/Thumb.tsx` | `accessible={false}` on both branches |
| `components/ui/BackHeader.tsx` | `height` → `minHeight` |
| `app/(tabs)/index.tsx` | Rewritten as the action queue |
| `app/(tabs)/orders/index.tsx` | Collapsing header, pill filters, scroll-revealed search, swipe, long-press sheet |
| `app/login.tsx` | Google/Apple buttons → icon row |
| ~10 layout containers | Screen gutter 16 → 20 |

---

### Task 1: Carry-over fixes from the increment 1 review

The final whole-branch review returned "merge with fixes". These are those fixes, plus they unblock later tasks — `PressableRow`'s narrow API is what forces duplicated layout and costs an a11y state today, and increment 2 adds ~15 more call sites.

**Files:** `components/ui/PressableRow.tsx`, `components/ui/Thumb.tsx`, `components/ui/BackHeader.tsx`, `components/StoreSelector.tsx`, `app/(tabs)/more/settings/team/index.tsx`, `app/(tabs)/more/index.tsx`, `components/ui/SearchField.tsx`, `components/ui/SegmentedControl.tsx`, `components/ui/StatusBadge.tsx`, `components/products/MediaGrid.tsx`, plus ~10 screen layout containers.

**Interfaces produced:**

```ts
export interface PressableRowProps {
  children: ReactNode;
  onPress: () => void;
  onLongPress?: () => void;
  lines?: 1 | 2;
  style?: StyleProp<ViewStyle>;
  accessibilityLabel: string;
  accessibilityRole?: AccessibilityRole;   // default "button"
  accessibilityState?: AccessibilityState;
  accessibilityHint?: string;
  disabled?: boolean;
  ripple?: { color: string };              // default theme.press.rippleInk
  testID?: string;
}
```

- [ ] **Step 1: Widen `PressableRow`.** Add the five props above. `disabled` must set both `Pressable`'s `disabled` and `accessibilityState.disabled`, and suppress press feedback. Keep the array style — never a function.
- [ ] **Step 2: Restore `StoreSelector`'s selected state.** `components/StoreSelector.tsx` lost `accessibilityState={{ selected: isActive }}` in the migration; pass it through the new prop.
- [ ] **Step 3: Delete the duplicated row block.** `app/(tabs)/more/settings/team/index.tsx` hand-mirrors `PressableRow`'s entire base style because there was no `disabled` prop. Replace with `PressableRow disabled`.
- [ ] **Step 4: Fix the link roles.** `app/(tabs)/more/index.tsx` — Privacy Policy and Terms call `Linking.openURL()` and leave the app but announce as `button`. Pass `accessibilityRole="link"`.
- [ ] **Step 5: `Thumb` a11y.** Set `accessible={false}` on **both** the image and placeholder branches, and drop the now-redundant `accessibilityLabel` at `components/ProductRow.tsx` — the enclosing `PressableRow` already announces the full row, so the thumb label is duplication that takes its own TalkBack focus.
- [ ] **Step 6: `BackHeader` overflow.** `components/ui/BackHeader.tsx` has a fixed `height: 48` now holding 40pt of content (12/16 eyebrow + 17/24 title). iOS is fine; Android's `includeFontPadding` adds ~4-6pt per Text node. Change to `minHeight: 48`. 35 screens pass an eyebrow to this.
- [ ] **Step 7: Sweep the orphaned `fontSize` literals.** Seven survived the re-scale, all aligned to the *old* scale: `SearchField.tsx` (14 — the app-wide search input, so typed text renders a step smaller than the rows beneath it), `SegmentedControl.tsx` (13 ×2), `StatusBadge.tsx` (11), `MediaGrid.tsx` (12). Replace with the matching `theme.text` preset or `Text preset`. Leave the two badge-counter values (10, 9) — those are deliberately sub-caption.
- [ ] **Step 8: Screen gutter 16 → 20.** Spec §1.2 requires it and increment 1's plan had no task for it. `theme.row.paddingH` is already 20, so list rows currently inset 4pt further than the `PageHeader` above them. Update the ~10 screen-level layout containers using `paddingHorizontal: theme.spacing.lg` for page content. Do **not** change `theme.spacing.lg` itself — it is used for non-gutter spacing throughout.
- [ ] **Step 9: Gates + screenshot.** `npm test`, `npm run check-types`, and a simulator screenshot of Products (gutter alignment + search field size) and one `BackHeader` screen.
- [ ] **Step 10: Commit.**

```bash
git commit -m "fix(mobile-admin): widen PressableRow API, fix a11y states, sweep orphaned type literals"
```

---

### Task 2: `IconButton` primitive

The icon-button idiom — `Pressable` + `hitSlop` + iOS opacity + ripple — is copy-pasted across 41 sites in 29 files. Increment 2 adds more. Extracting it also closes ~10 touch targets currently under 44pt.

**Files:** Create `components/ui/IconButton.tsx`; modify `components/ui/index.ts`; migrate call sites.

**Interface produced:**

```ts
export interface IconButtonProps {
  children: ReactNode;                 // the glyph
  onPress: () => void;
  accessibilityLabel: string;
  accessibilityRole?: AccessibilityRole;  // default "button"
  tone?: "ink" | "onDark" | "danger" | "accent";  // picks the ripple token
  disabled?: boolean;
  style?: StyleProp<ViewStyle>;
  testID?: string;
}
```

- [ ] **Step 1:** Write failing tests — renders children, fires `onPress`, enforces `minWidth`/`minHeight` of `theme.touchTarget`, passes a plain array style (never a function), maps each `tone` to the right `theme.press.ripple*` token.
- [ ] **Step 2:** Run, confirm they fail.
- [ ] **Step 3:** Implement. Minimum hit area is `theme.touchTarget` (44) via `minWidth`/`minHeight` on the pressable itself rather than `hitSlop`, so the target is real rather than an invisible overlay that can overlap siblings.
- [ ] **Step 4:** Migrate the icon-button sites. Preserve each site's existing glyph, size and label exactly.
- [ ] **Step 5:** Gates + screenshot of a header with icon buttons (e.g. Dashboard's notification bell).
- [ ] **Step 6:** Commit.

---

### Task 3: `CollapsingHeader`

**Files:** Create `components/ui/CollapsingHeader.tsx`; test `__tests__/collapsing-header.test.tsx`.

**Interface produced:**

```ts
export interface CollapsingHeaderProps {
  eyebrow?: string;
  title: string;            // serif, h1 expanded / h3 collapsed
  subtitle?: string;
  rightSlot?: ReactNode;
  scrollY: SharedValue<number>;
}
```

Behaviour:
- **Expanded** (offset 0): eyebrow, serif `h1` title, optional subtitle, right slot.
- **Collapsed** (offset ≥ 64): compact bar — serif `h3` title, `caption` subtitle beneath, right slot, hairline bottom rule.
- Driven by `useAnimatedScrollHandler` + `interpolate` on a shared value, **not** by re-render.
- Surface is solid Paper with a hairline. **No blur.**
- When `useReducedMotion()` is true, render the collapsed state immediately at any non-zero offset with no interpolation.

`PageHeader` is retained for non-scrolling and modal screens — do not delete it.

- [ ] **Step 1:** Failing tests — collapsed at offset ≥ 64, expanded at 0, collapsed immediately when reduced motion is on, title renders in both states.
- [ ] **Step 2:** Run, confirm failure.
- [ ] **Step 3:** Implement.
- [ ] **Step 4:** Gates.
- [ ] **Step 5:** Wire it into one screen temporarily to screenshot both states (scrolled and unscrolled). Revert the wiring if that screen isn't Task 7/8's target.
- [ ] **Step 6:** Commit.

---

### Task 4: `SwipeRow`

**Files:** Create `components/ui/SwipeRow.tsx`; test `__tests__/swipe-row.test.tsx`.

**Interface produced:**

```ts
export interface SwipeAction {
  key: string;
  label: string;
  icon: ReactNode;
  tone: "accent" | "danger" | "neutral";
  onPress: () => void;
}
export interface SwipeRowProps {
  children: ReactNode;
  leadingActions?: SwipeAction[];   // revealed by dragging RIGHT — constructive
  trailingActions?: SwipeAction[];  // revealed by dragging LEFT — destructive
  enabled?: boolean;                // default true
}
```

Behaviour:
- Built on gesture-handler's `Gesture.Pan()` + reanimated shared values. Threshold at **40% of row width**.
- `adminHaptics.swipeThreshold()` fires **once per crossing**, not on every frame.
- Spring back to rest on release below threshold.
- Owns **no business logic** — the caller supplies every handler. This is what makes it reusable across orders, reviews, tickets, coupons and products in increment 3.
- Gates on `useReducedMotion()`: when set, actions are still reachable but without the spring.

- [ ] **Step 1:** Failing tests — action fires past threshold, does not fire below it, springs back on release, threshold haptic fires exactly once per crossing, `enabled={false}` disables the gesture.
- [ ] **Step 2:** Run, confirm failure.
- [ ] **Step 3:** Implement.
- [ ] **Step 4:** Gates.
- [ ] **Step 5:** Screenshot a mid-swipe state if reachable; if not, say so — you cannot drive a pan gesture from `simctl`.
- [ ] **Step 6:** Commit.

---

### Task 5: `ActionSheet`

**Files:** Create `components/ui/ActionSheet.tsx`; test `__tests__/action-sheet.test.tsx`.

Wraps `@gorhom/bottom-sheet` (already installed) as the long-press action menu. This is the zero-dependency answer to a native context menu, and it matches the design system anyway — solid Paper surface, hairline rules, **no blur**.

**Interface produced:**

```ts
export interface ActionSheetItem {
  key: string;
  label: string;
  icon?: ReactNode;
  tone?: "default" | "danger";
  onPress: () => void;
}
export interface ActionSheetProps {
  title?: string;
  items: ActionSheetItem[];
  visible: boolean;
  onDismiss: () => void;
}
```

`adminHaptics.menuOpen()` fires on open. Rows are `PressableRow`-height (64pt) so they're comfortable one-thumb targets.

- [ ] **Step 1:** Failing tests — renders each item, fires the right handler, `danger` tone renders in `theme.colors.danger`, dismisses.
- [ ] **Step 2–6:** As above — run, implement, gates, screenshot, commit.

---

### Task 6: `RevenueChart`

**Files:** Create `components/dashboard/RevenueChart.tsx`; test `__tests__/revenue-chart.test.tsx`. Delete `components/dashboard/Sparkline.tsx` once nothing references it.

A **104pt** area chart on `react-native-svg` (already installed), replacing the 74×20 sparkline. Moss-tint fill, moss 2.25pt stroke, three faint gridlines, emphasised endpoint dot with a soft halo. This is the one thing on Home worth looking at, and moss here is one of its two permitted uses on that screen.

**Interface produced:**

```ts
export interface RevenueChartProps {
  data: number[];          // revenue_trend from the dashboard payload
  height?: number;         // default 104
  accessibilityLabel: string;
}
```

Edge cases that must be handled explicitly, since real stores hit all three: empty array, single point, and all-zero data (a flat line, not a divide-by-zero).

- [ ] **Step 1:** Failing tests covering all three edge cases plus a normal series.
- [ ] **Step 2–6:** Run, implement, gates, screenshot, commit.

---

### Task 7: `lib/queue.ts` + `QueueRow`

**Files:** Create `lib/queue.ts`, `components/dashboard/QueueRow.tsx`; tests for both.

`buildQueue` is **pure** — it takes already-fetched payloads and returns the sorted, capped, typed list. Keeping it separate from the screen makes the ordering and capping rules unit-testable without rendering, and it is the piece most likely to need tuning after you see it on device.

**Interface produced:**

```ts
export type QueueItemType = "order" | "review" | "stock" | "ticket";
export interface QueueItem {
  id: string;
  type: QueueItemType;
  primary: string;
  secondary: string;
  amount?: string;
  imageUrl?: string;
  badgeTone: "amber" | "moss" | "mute" | "blood";
  onPressRoute: string;
}
export function buildQueue(sources: QueueSources): QueueItem[];
```

Ordering — urgency then recency: **pending orders** (money waiting) → **low stock** (sales at risk) → **unanswered tickets** → **pending reviews**. Each type capped at **3**; total capped at **12**. When a type exceeds its cap, append a "See all N" row for that type, where `N` uses the authoritative count where one exists (`stats.orders_pending`, `stats.pending_reviews`) and otherwise renders "See all" with no number rather than a number that might be wrong.

`QueueRow` renders: 60pt `Thumb` (or a customer monogram disc where no product image applies), 17pt primary, 13pt secondary, right column with serif amount and typed badge. Wrapped in `SwipeRow` by the caller, not internally.

- [ ] **Step 1:** Failing tests for `buildQueue` — ordering, per-type cap, total cap, "See all" only when a type overflows, empty input → empty output, missing `customer_name` falls back to `customer_email`, missing `image_url` falls back to the monogram.
- [ ] **Step 2–6:** Run, implement, gates, screenshot, commit.

---

### Task 8: Dashboard as an action queue

**Files:** Rewrite `app/(tabs)/index.tsx`.

- **Header:** store name as the serif title (it's the merchant's shop — name it), date as the caption above, tenant monogram avatar as `rightSlot` with the notification badge. Uses `CollapsingHeader`.
- **Metrics band:** one elevated white card on the Paper ground — `heroNumeral` month revenue, moss-tint `↗ %` badge, caption line with today/this-week, `RevenueChart`, hairline, then the four-up order strip at `h2` serif. **Exactly one card** — a deliberate, bounded exception to "hairline rules, not bordered cards". Do not generalise it into a card grid.
- **"Needs you" queue:** `buildQueue` output, each row `SwipeRow`-wrapped. Orders get Approve (leading) / Cancel (trailing); reviews Approve / Reject; tickets Close (trailing only); low stock has no swipe.
- **Empty state:** when the queue is empty, a serif "All clear" editorial moment — not blank paper, not an apology.
- **Data:** compose from the dashboard payload plus `reviews?status=pending&limit=4` and `tickets?status=open&limit=4`. Each source is an **independent** query: if one fails the other three still render, with a single muted row naming what failed and a retry. The screen must never fail wholesale because tickets timed out.
- Optimistic mutations with rollback in `onError`, `adminHaptics.actionSucceeded()` / `actionFailed()`.

- [ ] **Step 1:** Failing tests — queue renders, empty state renders, a failing source degrades to a single error row rather than taking the screen down.
- [ ] **Step 2–5:** Run, implement, gates.
- [ ] **Step 6: Screenshot Dashboard.** Compare against the approved mockup and note every difference you can see. This is the sign-off screen.
- [ ] **Step 7:** Commit.

---

### Task 9: Orders — gesture triage

**Files:** Rewrite `app/(tabs)/orders/index.tsx`.

- `CollapsingHeader` with the pending count in `rightSlot`.
- **Search** moves into the scroll content, revealed by pulling down — it stops consuming ~56pt of every screen. Implement as a list header with the FlatList's initial `contentOffset` set past it. *If it proves undiscoverable on device, pinning it back is a one-prop revert — say so rather than redesigning.*
- **Filters:** underlined `SegmentedControl` → 40pt pill chips in a horizontal `ScrollView`. `adminHaptics.selectionChanged()` on change.
- **Rows:** 88pt, order number + customer name at 17/600, status badge right; serif tabular total at `h3` and relative time on the second line.
- **Swipe:** drag right → Approve (moss); drag left → Cancel (danger).
- **Long-press:** `ActionSheet` with Fulfil, Email label, Refund, Cancel.

- [ ] **Step 1:** Failing tests — filter change refetches, swipe actions call the right mutations, long-press opens the sheet.
- [ ] **Step 2–5:** Run, implement, gates.
- [ ] **Step 6:** Screenshot Orders, scrolled and unscrolled.
- [ ] **Step 7:** Commit.

---

### Task 10: Login — Google/Apple icon row

**Files:** `app/login.tsx`.

Three stacked full-width buttons, two of them solid ink ("Sign in" and "Sign in with Apple"), means the primary action doesn't read as primary. Collapse the two providers into a centred icon row so "Sign in" keeps the weight.

**Brand compliance is not optional here:**
- Apple permits a **logo-only** Sign in with Apple button, but it must use Apple's official mark and be **no less prominent** than other providers. An equal-sized icon row satisfies that; making Apple smaller than Google does not.
- Google permits the icon-only **"G"** mark at specified minimum sizes.
- **lucide has no brand marks** — it dropped them. Inline the official SVG paths via `react-native-svg` (already installed). Do not approximate the marks with generic glyphs, and do not add an icon package.
- Each target is at least 44pt and keeps its full accessible label ("Continue with Google", "Sign in with Apple") even though the visible label is gone.

The app shipped to both stores in late July, so this touches a review-sensitive surface — follow the specs exactly rather than eyeballing.

- [ ] **Step 1:** Failing tests — both providers render with correct accessible labels, both meet the 44pt target, handlers fire.
- [ ] **Step 2–5:** Run, implement, gates.
- [ ] **Step 6:** Screenshot login.
- [ ] **Step 7:** Commit.

---

## Out of scope

Increment 3 (rollout): Products, Customers, Reviews, Tickets, Coupons, Gift cards, Campaigns, Segments, Order detail, More/Account/Settings. Specified in `docs/superpowers/specs/2026-07-27-mobile-admin-native-ux-design.md` §3.

Also deliberately deferred: the pre-existing top-products `gofmt` misalignment; `has_first_order` schema drift; `theme.text` being unread by the render path (its own cleanup task).
