# mobile-admin Native UX — Increment 3 (The Rollout) Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Apply increment 2's proven pattern — collapsing header, 88pt rows, native press feedback, and a swipe where there is an obvious primary action — to every remaining screen in `apps/mobile-admin`, per spec §3.

**Architecture:** No new screens and no new features. One small rollout kit (a scroll hook, a busy-guard hook, a shared invariant-test helper, plus two ADDITIVE props on `CollapsingHeader`), then eight list screens wired onto the existing `CollapsingHeader` / `SwipeRow` / `ActionSheet` / `FilterChips` / `PressableRow` primitives, then the three items in §3 that are not list work: Order detail's sticky bottom action bar, the More/Account/Settings grouped inset lists, and the product editor/create density pass.

**Tech Stack:** React Native 0.85.3 · Expo SDK 56 · expo-router 56 · NativeWind 4.2.5 · reanimated 4.3 · gesture-handler 2.31 · @gorhom/bottom-sheet 5.2 · react-native-svg 15.15 · @tanstack/react-query 5.83

**Baseline at HEAD `7d62aa78`:** 900 jest / 89 suites in `apps/mobile-admin`, 95 vitest in `packages/mobile-shared`, `tsc --noEmit` clean in both. Tree clean, branch `main`, direct commits.

---

## Status of increments 1 and 2

Complete and on `main`. Everything below already exists, is reviewed, and is device-verified — **use it, do not rebuild it**:

`CollapsingHeader` · `SwipeRow` · `ActionSheet` · `RevenueChart` · `FilterChips` · `PressableRow` · `IconButton` · `Thumb` · `Monogram` · `StatusBadge` · `EmptyState` · `Text` · `Hairline` · `Screen` · `BackHeader` · `SegmentedControl` · `lib/queue.ts` · `lib/customer-identity.ts` · `adminHaptics` · `useDockClearance`.

**Already shipped early — do NOT re-plan these:**

- **Pill `FilterChips` rolled out to all eight list-filter screens** (commits `fa8a0dc5` + `975d0ae2`): Orders, products, customers, reviews, tickets, gift-cards, campaigns, coupons. Segments is the one list screen with no chips and needs none. `app/(tabs)/products/new.tsx` deliberately KEEPS `SegmentedControl` — it is a form field for product Status inside a Card, not a list filter.
- **`EmptyState` alignment normalised across 26 call sites** (commits `7c715717` + `4e4ff4e5`): every full-canvas list empty AND error state now passes `align="left"`. Three sites are deliberately left centred with a comment at each (`CategoryPickerSheet`, `TenantGate` ×2). All eight list screens in this plan already pass `align="left"` on both states and already carry the `errorSlot: { flex: 1 }` wrapper.
- **Rows are already 88pt.** Every list row in this plan (`ProductRow`, `CustomerRow`, `ReviewRow`, the inline `TicketRow`, `CouponRow`, `GiftCardRow`, `CampaignRow`, `SegmentRow`) is already `PressableRow lines={2}` at `theme.row.minHeightDouble` (88) with `paddingH` 20. **The density half of §3 is done for the list screens.** What is missing on them is the collapsing header, the swipes, and the long-press sheets.
- **Press feedback is done app-wide** (increment 1). `__tests__/no-touchable-opacity.test.ts` guards it.

---

## Global Constraints

Every task's requirements implicitly include this section. Each line here cost real debugging.

### Rendering and the device gate

- **🔴 NEVER pass a FUNCTION as a `style` prop to `Pressable`.** Under NativeWind 4.2.5's JSX interop a function style is not resolved like a plain array and **the styles are silently dropped at runtime**. This shipped once and left 24 call sites rendering with no padding and `flexDirection` falling back to `column` — with every unit test green, because RNTL renders without the NativeWind runtime. Use a plain array plus explicit `useState` press tracking driven by `onPressIn`/`onPressOut`, exactly as `components/ui/PressableRow.tsx` does. In `PressableRow` the array order is `[base, lines, style, pressed]` and `pressed` **must** stay last.
- **🔴 Metro silently serves stale bundles.** It has reported "1 module" rebuilds after a 30-file `git checkout`, and an entire Critical review finding turned out to be stale-bundle evidence from a pre-increment bundle. **Start every device session with `npx expo start --dev-client --clear --port 8082`** (8081 is held by Docker) and check the reported bundle/module count is plausible for what changed. A "1 module" rebuild after a large checkout is a LIE. Any device claim made without a cleared cache is suspect.
- **Fast Refresh silently fails to apply edits.** Verify against the screenshot, not against having saved the file.
- **🔴 `xcrun simctl ui content_size <size>` does NOT force a re-layout.** Changing it with the app running produces text that is clipped AND vertically overlapping — a stale layout, not a real state. It would lead you to "fix" a `lineHeight` bug that does not exist (RN's `RCTTextAttributes.mm:139` does scale `lineHeight` by the multiplier). **Terminate and relaunch after every size change.** Soundness check: with the method correct, `accessibility-large` and `AX3XL` come out PIXEL-IDENTICAL, which is exactly what the `Text` cap of 2 must produce.
- **Bundle id is `com.mark8ly.admin`.** A reviewer once burned a whole cycle launching `app.tesserix.admin` (a different, stale app) and concluded the app wouldn't boot.
- **`idb` IS installed** at `~/Library/Python/3.9/bin/idb` (just off `PATH`). Real tap AND swipe injection works. `idb ui swipe` needs `--delta` or it teleports. This is how you exercise a swipe on device.
- Launch order matters: `xcrun simctl launch booted com.mark8ly.admin` **before** any `xcrun simctl openurl` deep link; `openurl` alone leaves the simulator on the iOS home screen.
- **🔴 Never run a device-driving agent concurrently with another agent.** They share one simulator. This once caused stray coordinate taps that opened two Refund sheets and a Cancel-order sheet on real demo orders while another agent's Fast Refresh kept resetting navigation.
- **Every task ends with a device screenshot.** `xcrun simctl io booted screenshot <path>`. Green tests are not sufficient evidence — increment 2's device gate caught eight bugs that a fully green suite missed, including a hard red-screen crash.

### Dynamic Type

- `components/ui/Text.tsx` caps at `MAX_FONT_SCALE = 2` globally, applied as a default at the chokepoint so an explicit prop still wins.
- **`accessibility-medium` is 1.79×** (RN's `RCTFont` constant 1.786), so the cap is a provable **NO-OP there and proves nothing**. The cap first binds at `accessibility-LARGE` (AX2, 2.35×). **Every device gate in this plan must include one screenshot at `accessibility-large` or above.**
- **Fixed heights holding scalable text have caused SIX silent-clipping bugs** in this programme: `BackHeader` `height:48`, `CollapsingHeader` `height` 96/56, `ActionSheet`'s non-scrolling clamp, `RevenueChart`'s halo `PAD_Y`, `TenantMonogram`'s 40pt disc, and the four-up strip. Any new fixed box holding text is guilty until measured.
- Height contracts that must be honoured: `headerHeightsFor(fontScale, hasSubtitle, collapsedTitleLines)` in `CollapsingHeader.tsx:269`; `chipHeightsFor(fontScale)` in `FilterChips.tsx`. If a header slot contains text, cap it at the header's `MAX_FONT_SCALE` — the container height is computed from that multiplier.

### Gestures and destructive actions

- **🔴 `SwipeRow` never fires on swipe alone.** Past-threshold release **settles the row OPEN** and fires nothing; the user then taps the revealed action. Auto-fire is opt-in per action (`autoFireOnFullSwipe`) at ~85% travel, and **`tone: "danger"` is hard-blocked from auto-fire entirely**, flag or no flag. **This app has no undo.** Do not opt any action in.
- **Swipe convention, app-wide:** dragging a row **RIGHT** reveals the **constructive** action at the **leading** edge; dragging **LEFT** reveals the **destructive or dismissive** action at the **trailing** edge. Trailing is a POSITION, not a tone — a dismissive action (ticket Close, product Set to draft) sits trailing with `tone: "neutral"`.
- **🔴 The swipe-convention invariant test is mandatory on every new `SwipeRow` screen.** `SwipeRow` gives leading and trailing buttons the SAME testID pattern (`${testID}-action-${key}`), so swapping the two props leaves an entire suite green while putting the destructive action under the constructive gesture. Task 1 extracts the Orders/Dashboard version into `test-utils/swipe-convention.tsx`; every swipe screen calls it.
- **`SwipeRow` must stay a named function export.** The invariant test's finder keys on `(n.type as {name?: string}).name === "SwipeRow"`. Wrapping it in `memo`/an arrow makes every one of those tests silently find zero rows — which is why they all assert `rows.length > 0` first.
- **🔴 Long-press menus need the SAME busy guard as swipes.** `SwipeRow.enabled` does NOT gate the child row's `onLongPress`. Fulfil happens to be backend-idempotent; **Archive, Delete, Disable, Block and Close are not**, and they are exactly what §3's sheets contain. Gate `onLongPress` on the same per-row busy set: `onLongPress={isBusy(item.id) ? undefined : openMenu}`.
- **`ActionSheet` item count must stay constant per screen.** `snapPoints` memoises on `items.length`, so dropping an illegal item resizes the sheet under the merchant's thumb — and for a lazily-fetched precondition, resizes it AFTER it has opened. Use the additive `disabled` prop instead.
- **`ActionSheet` is controlled** (`visible` + `onDismiss`). The reason/refund sheets are **imperative** (`ref.present()` / `ref.dismiss()` + `onDismiss`). Do not mix the idioms on one sheet.
- **Sheet targets must clear on dismiss-without-submit.** Orders shipped a bug where `cancelTarget`/`refundTarget` survived a backed-out sheet, so dismissing on order A then opening Refund on order B rendered A's carrier warning on B. Every `xTarget` state gets `onDismiss={() => setXTarget(null)}`.
- **Sheet errors are LOCAL state, never `mutation.error`.** react-query never resets a mutation error, so binding a sheet to it means one failed action greets the merchant on every subsequent row's sheet. Clear on present.

### Design system

- **Shared primitives take ADDITIVE props, never changed defaults.** One changed default (`Eyebrow`'s gutter `lg`→`xl`) once rippled into ~15 call sites including five screens a report claimed were untouched. Where this plan needs a primitive to change, it says so explicitly and enumerates the blast radius.
- **One accent per view.** Moss `#2D4A2B` is never decorative. On a list screen it is spent on the constructive swipe and nothing else. The active `FilterChips` pill is deliberately **ink**, not moss.
- **Badges use `StatusBadge`; all four tones are tints.** Success `#E8EEE2`/`#2D4A2B`; warning `#7A4A0F` on `#F4E6CB`; danger `#8B2E20` on `#F6E4E1` (6.82:1); muted `#5C5953` on `#ECEAE3` (5.80:1).
- **Banned text colours:** `rgba(14, 14, 12, 0.5)` and `#7A766E`. Tertiary is `#5C5953`. A guard test enforces this against both token sources.
- **Tokens only, never raw hex.** Two token sources must agree: `lib/theme.ts` and `tailwind.config.js`.
- **Eyebrow + title + rows share ONE left edge**, gutter `theme.spacing.xl` (20).
- **44pt minimum touch target as a real `minWidth`/`minHeight` box**, not `hitSlop`. Exception: a small badge **overlaying** another element keeps its visible size and uses `hitSlop`, with the expanded region proven against the real sibling gap.
- **No glassmorphism. No centered heroes.** Solid Paper with hairlines; left-aligned, asymmetric.
- **Reduced motion:** every animation gates on `useReducedMotion()`.
- **`EmptyState align="left"` is silently defeated by a wrapping `styles.centered`** — a shrink-wrapping centred container re-centres the block even with the prop set. Use `errorSlot: { flex: 1 }`, as all eight list screens already do.
- **After adding a Tailwind class, restart Metro / clear the NativeWind CSS cache and verify on device.** (The earlier "a single-use class is never emitted" rule was DISPROVEN — a fresh `npx tailwindcss` build emits all 31 single-file classes. The real cause was a stale CSS cache. Do NOT convert working `className` usage to `StyleSheet` on the strength of the old rule.)

### Testing

- **Guard every fix with a test that fails when the fix is removed.** Several increment-2 fixes were later found deletable with a fully green suite (`EmptyState`'s `align` default, `Thumb`'s ring, `queue.ts`'s `kind` discriminant, `RevenueChart`'s paint order). Prove red→green by reverting, running, and restoring.
- **🔴 Choose the fixture that exposes the bug.** All three customer-identity defects passed against a customer who had a name. A fixture that cannot fail is not a test.
- **`components/ui/index.ts` has transitively broken unrelated suites twice** (once adding a reanimated importer, once adding a gorhom importer). If a new module in that barrel pulls in a native-heavy dependency, expect to add a `moduleNameMapper` in `jest.config.js` — and verify the mapping masks nothing (that no suite ever exercised the real module).
- **react-query invalidation is PREFIX matching** (`exact: false` by default in v5). `invalidateQueries({queryKey: ["products"]})` reaches `["products", status, search]`.
- **Gates, run in both packages:** `npm test` and `npm run check-types` in `apps/mobile-admin` (baseline 900 tests / 89 suites, tsc clean) and `npx vitest run` + `npm run check-types` in `packages/mobile-shared` (95 vitest).

### Process

- **Zero new npm dependencies.** The root lockfile cannot be regenerated locally — a plain `npm install` collapses the deliberate multi-version tree and breaks mobile-admin. `package.json` and `package-lock.json` must be byte-identical to base at the end of this increment. **If a task appears to need a dependency, that task is wrong.**
- **Modal sheet trap:** `app/(tabs)/products/new.tsx` is the only `presentation:"modal"` screen. It is presented above the root `BottomSheetModalProvider`, so it keeps its own local provider (`new.tsx:159`) and must use `useSafeAreaInsets().bottom`, **not** `useDockClearance()`.
- **Commit style:** conventional commits, single-line messages, no body, no signature, no co-author trailer. Direct to `main`.
- **Update `.superpowers/sdd/progress.md`** at the end of each task with what shipped and what the device gate caught.

---

## What §3 asks for that the backend cannot do

Read before planning any action. Every destructive action in §3 was checked against its actual Go handler in `services/marketplace-api`.

**These endpoints DO NOT EXIST and their §3 menu items are CUT:**

| §3 item | Finding |
|---|---|
| Reviews → **Report** | No report/flag concept anywhere in `internal/review`. |
| Tickets → **Assign** | No assignee field and no route. |
| Gift cards → **Enable / Disable** | `giftcard.Service` has no disable method at all; the `disabled` status in `giftcard/models.go:20` is unreachable from the admin API. Admin surface is list / get / issue only. |
| Gift cards → **Delete** | Does not exist. |
| Coupons · Campaigns · Segments → **Duplicate** | No duplicate endpoint for any of the three. |
| Products → **Duplicate** | `POST /:id/copy` exists but **rejects `target_store_id == source`** (`service_copy.go:67`), so a product cannot be duplicated inside its own store — and there is **no mobile route** for it at all. |
| Products → **Delete** | Web-only (`RoleOwner`); **no mobile route**. Archive is the mobile-reachable equivalent. |
| Customers → **Email / Call** | No server endpoints — these are client-side `mailto:` / `tel:` links only, which is fine and is how they are planned. |

**These exist, and here is exactly how each must be invoked:**

| Action | Endpoint | Fires directly, or opens a sheet/confirm? | Why |
|---|---|---|---|
| Product **activate** | `PATCH /mobile/admin/stores/:sid/products/:id` `{status:"active"}` | **Fires directly.** | Plain field write, idempotent, no gating (`service.go:385-392`). Reversible. |
| Product **set to draft** | same, `{status:"draft"}` | **Fires directly.** | Idempotent, reversible, no gating. Dismissive not destructive → `tone: "neutral"`. |
| Product **archive** | same, `{status:"archived"}` | **Confirm `Alert.alert` first.** | Idempotent and technically reversible — **but only if the merchant can still find the product.** The mobile list's chips are all/active/draft, so an archived product currently vanishes from every filter. **Task 2 therefore adds an "Archived" chip**; without it, Archive is irreversible from mobile and must not ship. |
| Product **edit price / adjust stock** | `PATCH .../products/:id/variants/:variantId` | **CUT from scope** — see "Recommended cuts". | Needs variant selection and a numeric-entry sheet. That is new feature work, not applying a pattern. |
| Customer **block** | `POST /mobile/admin/.../customers/:id/block` | **🔴 MUST open a sheet.** `BlockCustomerRequest.Reason` is `binding:"required"` (`customers_dto.go:72`) — an empty string fails binding. | Same shape as order cancel. |
| Customer **unblock** | `POST .../customers/:id/unblock` | **Fires directly.** | No body read at all, idempotent. |
| Review **approve** | `POST .../reviews/:id/approve` | **Fires directly.** | No body. **Explicitly idempotent** — already-approved returns 200 with the review (`reviews.go:110-113`). |
| Review **reject** | `POST .../reviews/:id/reject` | **Fires directly.** | No body. Explicitly idempotent (`reviews.go:144-147`). |
| Review **reply** | `POST .../reviews/:id/reply` | **Navigates to the review detail screen.** | `Content` required, max 5000, and appends a NEW reply every call (not idempotent). A multiline compose belongs on the detail screen, which already has one. |
| Ticket **close** | `PATCH .../tickets/:id` `{status:"closed"}` | **Fires directly, but ONLY when legal.** | **NOT idempotent — same-status returns HTTP 409** `invalid_transition` (`ticket/models.go:112-114`), and `closed` is **terminal: it cannot be reopened** (`:129`). Gate the action off `status !== "closed"`, and confirm with an `Alert` because it is one-way. |
| Ticket **reply** | `POST .../tickets/:id/reply` | **Navigates to the ticket detail screen.** | `Content` required; closed tickets reject replies with 409; replying to a `resolved` ticket auto-reopens it. |
| Coupon **enable / disable** | `PATCH .../coupons/:id` `{status:"active"\|"disabled"}` | **Fires directly.** | Idempotent, reversible. Only `active`/`disabled` accepted; `expired` is system-managed and rejected. |
| Coupon **delete** | `DELETE .../coupons/:id` | **CUT.** | It is not a delete: it sets `status='disabled'`, returns `200 {"message":"coupon disabled"}`, and the row **stays in the list**. Labelling it "Delete" in the UI is a lie, and "Disable" is already offered by the PATCH above. |
| Campaign **delete** | `DELETE .../campaigns/:id` | **Confirm `Alert.alert`.** | Irreversible (second call 404s) and **only legal for `status === "draft"`** — anything else returns HTTP 409 `campaign_not_draft` (`campaign/service.go:197-205`). Disable the item for non-draft campaigns. |
| Segment **delete** | `DELETE .../segments/:id` | **Confirm `Alert.alert` naming the segment.** | **Hard delete**, irreversible, second call 404s. **Blocked with `409 segment_in_use`** when campaigns still reference it (`campaign/service.go:273-286`, `campaign/repository.go:125-141`); `ApiError.message` already states how many campaigns are blocking it. On that rejection the sheet must surface that reason instead of a generic error. |
| Order **cancel** | `POST .../orders/:id/cancel` | **MUST open `CancelReasonSheet`** (already wired on Orders list and detail). | `Reason` is `binding:"required"`. Terminal — second call 409s. Cancelling a paid order auto-fires a full refund and cancels the carrier shipment. |
| Order **refund** | `POST .../orders/:id/refund` | **MUST open `RefundSheet`.** | `refund_request_id` manually enforced (422 otherwise). Idempotent per request id; a DIFFERENT id triggers a second real gateway refund. |
| Order **fulfil** | `POST .../orders/:id/fulfill` | **Fires directly when legal.** | No body. NOT idempotent — `fulfilled` is terminal, second call 409s. Legal only from `confirmed`. |
| Order **confirm** | `POST .../orders/:id/confirm` | **Fires directly when legal** (`status === "pending"`). | Already wired via `useConfirmOrder`. |

---

## Optimistic state — settled here, per screen

The Dashboard's optimistic hide took FOUR rounds. The rule that finally worked (`useExpireHidesOnFreshAnswer`, commit `13a1deae`): a mutation *settling* is not fresh data; clear the hide on a **per-source `dataUpdatedAt` watermark**, and only when `dataUpdatedAt` advances PAST it.

**None of that machinery is needed in this increment. The verified reason:**

| Screen | List queryKey | Mutation invalidates | Prefix match? | Optimistic hiding needed? |
|---|---|---|---|---|
| Products | `["products", status, search]` | `["products"]` + `["product", id]` (`product-crud.ts:42-43`) | ✅ | **No** |
| Customers | `["customers", search, status]` | `["customers"]` + `["customer", id]` (`customer-actions.ts:14-15,28-29`) | ✅ | **No** |
| Reviews | `["reviews", status]` | `["reviews"]`, `["review", id]`, `["dashboard"]` (`review-actions.ts:23-25`) | ✅ | **No** |
| Tickets | `["tickets","list",status]` | `["tickets"]` (`ticket-actions.ts:15`) | ✅ | **No** |
| Coupons | `["coupons","list",status,search]` | `["coupons"]` (`coupon-actions.ts:20`) | ✅ | **No** |
| Gift cards | `["gift-cards","list",status]` | n/a — no mutations exist | — | **No** |
| Campaigns | `["campaigns","list",status]` | `["campaigns"]` (`campaign-actions.ts:19`) | ✅ | **No** |
| Segments | `["segments","list"]` | `["segments"]` + `["campaigns"]` (`segment-actions.ts:19,22`) | ✅ | **No** |

Every mutation invalidates a strict PREFIX of its own list's key, so **each list refetches itself** — exactly the situation Orders was correctly built for ("no optimistic hide; `useOrderMutation` invalidates `["orders"]` and `useOrders` is keyed `["orders","list",…]`"). The Dashboard's `["dashboard"]` asymmetry — the thing that made the hide necessary and then broke it three times — **does not exist on any screen in this increment.**

**🔴 Two consequences an implementer must not "fix":**

1. **Do not add an optimistic hide to any list screen in this plan.** The row disappears (or re-badges) when the refetch lands, which is the correct, already-proven behaviour.
2. **Tickets deliberately do NOT invalidate `["dashboard"]`,** and that exclusion is PINNED by an existing test — the dashboard schema has no ticket field. Closing a ticket from the Tickets screen will not update the Dashboard's queue row until the dashboard refetches on its own. That is correct. Leave it.

The **only** state a list screen keeps is the per-row busy set (`useBusyIds`, Task 1), which suppresses the gesture and the long-press while that row's own request is open. It makes no claim about the data.

---

## Fixtures — what the demo store cannot exercise

Known facts about the demo tenant (Bondi, `demo@`, tenant `8c302556-b647-4824-8ce4-73f547ca456e`, store `8b69eea9-2537-4d36-9d99-bafcbad02dbc`): **231 products**, **$0 revenue**, **exactly one customer, who has no name** (email only). A handful of demo orders exist and must not be actioned.

Create fixtures **through the app's own UI or the web admin**, never by seeding the database directly — a prior session correctly declined a DB seed because it bypasses production business logic.

| Screen | What exists | What you must create before the device gate |
|---|---|---|
| Products (Task 2) | 231 products, mixed active/draft | One product you are willing to archive and un-archive. Verify the new **Archived** chip finds it again — that is the whole point of the chip. |
| Customers (Task 3) | **One customer, no name, almost certainly no phone, not blocked** | (a) A customer **with** a first/last name — the no-name path is the default here and a named customer is the one that can fail. (b) A customer **with a phone number**, to see Call enabled; the no-phone customer proves the disabled path. (c) Block one, so the **Unblock** path and the `blocked` chip are reachable. |
| Reviews (Task 4) | Some reviews exist (a store screenshot used them) | At least one **pending** review — approve/reject are only offered on pending. Storefront-side: submit a review as a customer if none is pending. |
| Tickets (Task 5) | Unknown | One **open** ticket (Close legal) **and** one **closed** ticket (Close must be disabled and must NOT be swipeable). Without the closed one, the 409-avoidance gating is untested. |
| Coupons (Task 6) | Unknown | One **active** and one **disabled** coupon, so both swipe edges are reachable on real rows. |
| Gift cards (Task 1) | Unknown | At least one issued gift card, so the list is not empty behind the new header. Issue one via the existing Issue flow. |
| Campaigns (Task 7) | Unknown | 🔴 One **draft** campaign (Delete legal) **and** one **non-draft** (sent/scheduled/paused — Delete must be disabled). The non-draft is the fixture that exposes a missing gate; a draft-only store passes against broken code. |
| Segments (Task 7) | Unknown | One segment you will delete, and one **referenced by a campaign** — the backend refuses to delete the referenced one with a `409 segment_in_use`, and the delete fixture must exercise that blocked path, not just the confirm copy. |
| Order detail (Task 8) | A few demo orders | 🔴 You need one order in **each** of `pending`, `confirmed`, `fulfilled`, `cancelled` to prove the sticky bar's primary-slot gating. **Do not action the existing demo orders to produce them** — place new storefront orders. |
| More/Account/Settings (Task 9) | Real | Nothing extra. Team screen wants ≥2 members plus a pending invitation to show both row shapes. |
| Product editor/create (Task 10) | Real | One single-variant and one multi-variant product. |

---

## File Structure

**Created**

| File | Responsibility |
|---|---|
| `lib/use-collapsing-scroll.ts` | The `scrollY` shared value + `useAnimatedScrollHandler` pair every collapsing screen needs |
| `lib/use-busy-ids.ts` | Per-row in-flight guard: `isBusy` / `markBusy` / `clearBusy` / `settleCallbacks` |
| `test-utils/swipe-convention.tsx` | The leading-is-constructive invariant, as a callable assertion |
| `components/ui/StickyActionBar.tsx` | Bottom action bar: full-width, hairline-topped, Paper |
| `components/ui/GroupedList.tsx` | Sectioned inset list — eyebrow label + `Card padding={0}` + inset hairlines |
| `components/ui/GroupedRow.tsx` | One grouped row: icon slot, label, optional value/badge/trailing slot, optional chevron |
| `components/customers/BlockReasonSheet.tsx` | Required-reason sheet for customer block |
| `lib/admin-api/product-status.ts` | `useSetProductStatus` — the one mutation this increment adds |

**Modified**

| File | Change |
|---|---|
| `components/ui/CollapsingHeader.tsx` | **ADDITIVE** `onBack?: () => void` and `leadingSlot?: ReactNode` |
| `components/ui/index.ts` | Export the new primitives |
| `app/(tabs)/index.tsx`, `app/(tabs)/orders/index.tsx` | Migrate their inline scroll handler to `useCollapsingScroll`; Orders migrates to `useBusyIds` |
| `app/(tabs)/more/marketing/gift-cards/index.tsx` | CollapsingHeader with back |
| `app/(tabs)/products/index.tsx` | CollapsingHeader, Archived chip, swipe, long-press sheet |
| `app/(tabs)/customers/index.tsx` | CollapsingHeader, long-press sheet, block-reason sheet |
| `app/(tabs)/customers/reviews/index.tsx` | CollapsingHeader with back, swipe, long-press sheet |
| `app/(tabs)/more/settings/tickets/index.tsx` | CollapsingHeader with back, trailing swipe, long-press sheet |
| `app/(tabs)/more/marketing/coupons/index.tsx` | CollapsingHeader with back, swipe, long-press sheet |
| `app/(tabs)/more/marketing/campaigns/index.tsx` | CollapsingHeader with back, long-press sheet |
| `app/(tabs)/more/marketing/segments/index.tsx` | CollapsingHeader with back, long-press sheet |
| `app/(tabs)/orders/[id].tsx` | Actions move into `StickyActionBar` + overflow `ActionSheet` |
| `app/(tabs)/products/new.tsx` | Its hand-rolled footer becomes `StickyActionBar`; density pass |
| `app/(tabs)/products/[id].tsx` | Density and type pass |
| `app/(tabs)/more/index.tsx`, `more/account.tsx`, `more/security.tsx`, `more/settings/notification-settings.tsx`, `more/settings/team/index.tsx` | Grouped inset lists |

---

### Task 1: The rollout kit, proved on Gift cards

Six of the eight list screens are **nested routes that use `BackHeader`** — `CollapsingHeader` has no back affordance, so §3's "every screen gets a collapsing header" is impossible without an additive prop. That is settled here, and proved on Gift cards: the one list screen with **zero backend actions** (no enable, no disable, no delete — see the cut table), so it isolates the header work from any mutation risk.

**Files:**
- Create: `lib/use-collapsing-scroll.ts`, `lib/use-busy-ids.ts`, `test-utils/swipe-convention.tsx`
- Modify: `components/ui/CollapsingHeader.tsx`, `components/ui/index.ts`, `app/(tabs)/more/marketing/gift-cards/index.tsx`, `app/(tabs)/index.tsx`, `app/(tabs)/orders/index.tsx`
- Test: `__tests__/collapsing-header.test.tsx` (extend), `__tests__/use-busy-ids.test.tsx` (new), `__tests__/gift-cards-screen.test.tsx` (new or extend)

**Interfaces produced:**

```ts
// lib/use-collapsing-scroll.ts
import { useAnimatedScrollHandler, useSharedValue } from "react-native-reanimated";
import type { SharedValue } from "react-native-reanimated";

export interface CollapsingScroll {
  /** Pass to `CollapsingHeader`'s `scrollY`. */
  scrollY: SharedValue<number>;
  /** Spread onto the Animated scroll view: `onScroll={onScroll}` + `scrollEventThrottle={16}`. */
  onScroll: ReturnType<typeof useAnimatedScrollHandler>;
}

export function useCollapsingScroll(): CollapsingScroll {
  const scrollY = useSharedValue(0);
  const onScroll = useAnimatedScrollHandler((event) => {
    scrollY.value = event.contentOffset.y;
  });
  return { scrollY, onScroll };
}
```

```ts
// lib/use-busy-ids.ts
export interface BusyIds {
  /** True while THIS row's own request is open. */
  isBusy: (id: string) => boolean;
  markBusy: (id: string) => void;
  clearBusy: (id: string) => void;
  /**
   * react-query callbacks for a direct (non-sheet) mutation on one row:
   * releases that row's guard by id and reports the outcome in the hand.
   * There is nothing to roll back — no local state ever claimed the row changed.
   */
  settleCallbacks: (id: string) => { onSuccess: () => void; onError: () => void };
}

export function useBusyIds(): BusyIds;
```

```tsx
// test-utils/swipe-convention.tsx
export const CONSTRUCTIVE_TONE = "accent";
export const DESTRUCTIVE_TONE = "danger";

export type Root = ReturnType<typeof render>["UNSAFE_root"];
export interface RowActions {
  leadingActions?: { key: string; tone: string; autoFireOnFullSwipe?: boolean }[];
  trailingActions?: { key: string; tone: string; autoFireOnFullSwipe?: boolean }[];
}

export function swipeRows(root: Root): ReturnType<Root["findAll"]>;
export function swipeRow(root: Root, testID: string): ReturnType<Root["findAll"]>[number] | undefined;
/** Asserts the app-wide side/tone invariant over EVERY mounted SwipeRow. Fails if there are none. */
export function assertSwipeConvention(root: Root): void;
/** Asserts no action on any row opts into full-swipe auto-fire. Fails if there are none. */
export function assertNoAutoFire(root: Root): void;
```

```ts
// components/ui/CollapsingHeader.tsx — ADDITIVE only
export interface CollapsingHeaderProps {
  // ... every existing prop unchanged ...
  /**
   * Renders a back chevron at the leading edge in BOTH states, vertically
   * centred like `rightSlot`. ADDITIVE: omitted on the two tab-root screens
   * that already use this primitive, so their rendering is bit-identical.
   */
  onBack?: () => void;
  /** Escape hatch for a non-back leading control. Ignored when `onBack` is set. */
  leadingSlot?: ReactNode;
}
```

- [ ] **Step 1: Write the failing tests for `useBusyIds`.**

```tsx
// __tests__/use-busy-ids.test.tsx
import { renderHook, act } from "@testing-library/react-native";
import { useBusyIds } from "@/lib/use-busy-ids";

describe("useBusyIds", () => {
  it("guards only the row that was marked", () => {
    const { result } = renderHook(() => useBusyIds());
    act(() => result.current.markBusy("a"));
    expect(result.current.isBusy("a")).toBe(true);
    expect(result.current.isBusy("b")).toBe(false);
  });

  // The bug this replaces: Orders' first version used ONE slot, so marking B
  // overwrote A's guard and A's onSuccess then cleared it outright, re-arming
  // B while B's own request was still open.
  it("keeps A guarded while B is settling", () => {
    const { result } = renderHook(() => useBusyIds());
    act(() => {
      result.current.markBusy("a");
      result.current.markBusy("b");
    });
    act(() => result.current.settleCallbacks("b").onSuccess());
    expect(result.current.isBusy("a")).toBe(true);
    expect(result.current.isBusy("b")).toBe(false);
  });

  it("clears the guard on error as well as success", () => {
    const { result } = renderHook(() => useBusyIds());
    act(() => result.current.markBusy("a"));
    act(() => result.current.settleCallbacks("a").onError());
    expect(result.current.isBusy("a")).toBe(false);
  });

  // Without a new identity, a FlatList renderItem memoised on `isBusy` never
  // re-runs and a guarded row keeps rendering as swipeable.
  it("returns a NEW isBusy identity whenever the set changes", () => {
    const { result } = renderHook(() => useBusyIds());
    const before = result.current.isBusy;
    act(() => result.current.markBusy("a"));
    expect(result.current.isBusy).not.toBe(before);
  });
});
```

- [ ] **Step 2: Run and confirm failure.** `cd apps/mobile-admin && npx jest __tests__/use-busy-ids.test.tsx` — expect "Cannot find module '@/lib/use-busy-ids'".

- [ ] **Step 3: Implement `useBusyIds`.**

```ts
// lib/use-busy-ids.ts
import { useCallback, useMemo, useState } from "react";

export interface BusyIds {
  isBusy: (id: string) => boolean;
  markBusy: (id: string) => void;
  clearBusy: (id: string) => void;
  settleCallbacks: (id: string) => { onSuccess: () => void; onError: () => void };
}

export function useBusyIds(): BusyIds {
  // A SET, not one slot: triage is a queue and a merchant fires the next row
  // long before the previous one's request comes back. Replaced immutably so
  // React sees a new identity and memoised renderItems actually re-run.
  const [ids, setIds] = useState<ReadonlySet<string>>(() => new Set());

  const markBusy = useCallback((id: string) => {
    setIds((prev) => (prev.has(id) ? prev : new Set(prev).add(id)));
  }, []);

  const clearBusy = useCallback((id: string) => {
    setIds((prev) => {
      if (!prev.has(id)) return prev;
      const next = new Set(prev);
      next.delete(id);
      return next;
    });
  }, []);

  const isBusy = useCallback((id: string) => ids.has(id), [ids]);

  const settleCallbacks = useCallback(
    (id: string) => ({
      onSuccess: () => {
        clearBusy(id);
        void adminHaptics.actionSucceeded();
      },
      onError: () => {
        clearBusy(id);
        void adminHaptics.actionFailed();
      },
    }),
    [clearBusy],
  );

  return useMemo(
    () => ({ isBusy, markBusy, clearBusy, settleCallbacks }),
    [isBusy, markBusy, clearBusy, settleCallbacks],
  );
}
```

Import `adminHaptics` from `@repo/mobile-shared/haptics/feedback`. Note the haptics assertions belong in this suite too — add one test that `settleCallbacks(id).onSuccess()` calls `actionSucceeded` and `onError()` calls `actionFailed`, with the module mocked.

- [ ] **Step 4: Run and confirm pass.**

- [ ] **Step 5: Implement `useCollapsingScroll` and migrate the two existing callers.**

Write the module exactly as in the Interfaces block. Then replace the inline `useSharedValue` + `useAnimatedScrollHandler` pair in `app/(tabs)/index.tsx` and `app/(tabs)/orders/index.tsx` with `const { scrollY, onScroll } = useCollapsingScroll();`. **Behaviour must be bit-identical** — these are the two sign-off screens. Their existing suites are the guard; run them specifically and confirm no assertion changed.

- [ ] **Step 6: Migrate Orders to `useBusyIds`.**

`app/(tabs)/orders/index.tsx` currently declares `busyOrderIds`, `markBusy`, `clearBusy` and `settleCallbacks` inline (lines ~170-276). Replace with `const busy = useBusyIds();` and rewrite the two call sites: `enabled={!busy.isBusy(item.id)}` and `onLongPress={busy.isBusy(item.id) ? undefined : setMenuOrder}`. `renderItem`'s dependency array changes from `busyOrderIds` to `busy.isBusy`. Its existing tests must stay green **without editing them**; if one needs editing, you have changed behaviour — stop and say so.

- [ ] **Step 7: Write the failing test for `CollapsingHeader`'s `onBack`.**

```tsx
// append to __tests__/collapsing-header.test.tsx
describe("CollapsingHeader — back affordance", () => {
  it("renders no back control by default (bit-identical for existing callers)", () => {
    const { queryByLabelText } = render(
      <CollapsingHeader title="Inbox" scrollY={makeScrollY(0)} />,
    );
    expect(queryByLabelText("Go back")).toBeNull();
  });

  it("renders a back control in BOTH states when onBack is supplied", () => {
    const onBack = jest.fn();
    const expanded = render(<CollapsingHeader title="Coupons" onBack={onBack} scrollY={makeScrollY(0)} />);
    expect(expanded.getByLabelText("Go back")).toBeTruthy();
    expanded.unmount();
    const collapsed = render(<CollapsingHeader title="Coupons" onBack={onBack} scrollY={makeScrollY(200)} />);
    expect(collapsed.getByLabelText("Go back")).toBeTruthy();
  });

  it("fires onBack", () => {
    const onBack = jest.fn();
    const { getByLabelText } = render(
      <CollapsingHeader title="Coupons" onBack={onBack} scrollY={makeScrollY(0)} />,
    );
    fireEvent.press(getByLabelText("Go back"));
    expect(onBack).toHaveBeenCalledTimes(1);
  });

  // The height contract is computed from MAX_FONT_SCALE; a leading control
  // that grows the header without headerHeightsFor knowing about it is the
  // sixth silent-clipping bug waiting to happen.
  it("does not change headerHeightsFor", () => {
    expect(headerHeightsFor(1, false, 1)).toEqual(headerHeightsFor(1, false, 1));
    // and the title still renders at both states with a back control present
    const { getByText } = render(
      <CollapsingHeader title="Coupons" onBack={jest.fn()} scrollY={makeScrollY(200)} />,
    );
    expect(getByText("Coupons")).toBeTruthy();
  });
});
```

Reuse the file's existing `makeScrollY` helper if there is one; otherwise write it as `{ value: n } as SharedValue<number>` matching how the existing tests drive the primitive.

- [ ] **Step 8: Run, confirm failure, implement `onBack` / `leadingSlot`.**

Render the back control as an `IconButton` with a `ChevronLeft` lucide glyph, `accessibilityLabel="Go back"`, `tone="ink"`. It is a **fixed-size 44pt control containing no text**, so it does not participate in `headerHeightsFor` — the collapsed bar is 56pt at 1× and 112pt at the cap, both of which contain 44 comfortably. The title block's horizontal space shrinks by `44 + theme.spacing.sm`; the title already uses `adjustsFontSizeToFit` with a 13pt floor on both layers, so a long title shrinks rather than clipping. `headerHeightsFor` is **not** modified.

- [ ] **Step 9: Write `test-utils/swipe-convention.tsx`.**

Port the assertions verbatim from `__tests__/orders-screen.test.tsx:211-227` and `:432-493`, keeping the rationale comment. The finder:

```tsx
export function swipeRows(root: Root) {
  return root.findAll(
    (n) => typeof n.type !== "string" && (n.type as { name?: string }).name === "SwipeRow",
  );
}

export function assertSwipeConvention(root: Root) {
  const rows = swipeRows(root);
  // Fails loudly if the finder found nothing — SwipeRow must stay a NAMED
  // function export or every one of these assertions vacuously passes.
  expect(rows.length).toBeGreaterThan(0);
  for (const row of rows) {
    const { leadingActions = [], trailingActions = [] } = row.props as RowActions;
    for (const action of leadingActions) expect(action.tone).not.toBe(DESTRUCTIVE_TONE);
    for (const action of trailingActions) expect(action.tone).not.toBe(CONSTRUCTIVE_TONE);
  }
}

export function assertNoAutoFire(root: Root) {
  const rows = swipeRows(root);
  expect(rows.length).toBeGreaterThan(0);
  for (const row of rows) {
    const { leadingActions = [], trailingActions = [] } = row.props as RowActions;
    for (const a of [...leadingActions, ...trailingActions]) {
      expect(a.autoFireOnFullSwipe).toBeFalsy();
    }
  }
}
```

**🔴 `test-utils/` must not be picked up by jest's `testMatch`.** Verify: run the full suite and confirm the suite count is 89 + only your new suites, with no "Your test suite must contain at least one test" failure. If jest does pick it up, add `testPathIgnorePatterns: ["<rootDir>/test-utils/"]` to `jest.config.js`.

- [ ] **Step 10: Migrate Orders' and the Dashboard's copies onto the helper.**

`__tests__/orders-screen.test.tsx` and `__tests__/dashboard-screen.test.tsx` each carry a hand-copied version. Replace the invariant and auto-fire cases with `assertSwipeConvention(UNSAFE_root)` / `assertNoAutoFire(UNSAFE_root)`. **Keep** each screen's positive per-action assertions (`approve` is leading + accent, `cancel` is trailing + danger, ticket `close` is trailing + neutral, and the paint assertions) — those are screen-specific and the helper deliberately does not cover them. Prove the helper still bites: temporarily swap `leadingActions`/`trailingActions` in `orders/index.tsx`, run, see red, restore.

- [ ] **Step 11: Wire Gift cards onto `CollapsingHeader`.**

`app/(tabs)/more/marketing/gift-cards/index.tsx`. Replace `BackHeader` (line ~62) with:

```tsx
const router = useRouter();
const { scrollY, onScroll } = useCollapsingScroll();
const dockPad = useDockClearance();
// ...
<Screen>
  <CollapsingHeader
    eyebrow="MARKETING"
    title="Gift cards"
    onBack={() => router.back()}
    scrollY={scrollY}
  />
  <FilterChips chips={CHIPS} value={status} onChange={setStatus} />
  <Animated.FlatList
    data={items}
    onScroll={onScroll}
    scrollEventThrottle={16}
    contentContainerStyle={[styles.list, { paddingBottom: dockPad }]}
    renderItem={renderItem}
    keyExtractor={(item) => item.id}
    ListEmptyComponent={<EmptyState align="left" title="No gift cards" message="…" />}
    // ...existing infinite-scroll + RefreshControl props unchanged
  />
</Screen>
```

`Animated` is `react-native-reanimated`'s default export. `FilterChips` stays **outside** the FlatList (it is pinned, matching Orders after the search revert) and **above** the list, below the header. Do **not** move it into `ListHeaderComponent` — the chips block owns its own vertical rhythm and a hugging wrapper, and putting it in the list re-introduces the ~110pt-of-dead-paper stretch bug.

**Gift cards get NO swipe and NO long-press sheet.** The backend has no enable, disable or delete — see the cut table. Row press continues to navigate to the detail screen.

- [ ] **Step 12: Gates.** `npm test` and `npm run check-types` in `apps/mobile-admin`; `npx vitest run` and `npm run check-types` in `packages/mobile-shared`. `git diff --stat package.json package-lock.json` must be empty.

- [ ] **Step 13: Device gate.**

1. `cd apps/mobile-admin && npx expo start --dev-client --clear --port 8082`. Confirm the bundle count is plausible for the files you changed.
2. `xcrun simctl launch booted com.mark8ly.admin`, navigate More → Marketing → Gift cards.
3. Screenshot **expanded** (offset 0) and **collapsed** (scrolled past 64). Verify: the back chevron is present and vertically centred in BOTH states; the title does not clip; eyebrow, title and rows share ONE left edge at gutter 20; the chips strip sits between header and rows with no dead paper.
4. Tap the back chevron. Verify it navigates back, in both the expanded and collapsed state.
5. `xcrun simctl ui booted content_size accessibility-large`, then **terminate and relaunch**, then screenshot again. Verify no clipping and no mid-word breaks.
6. Return to `large`, terminate, relaunch.
7. Regression screenshot of **Dashboard** and **Orders** (both scrolled and unscrolled) — Steps 5, 6 and 10 touched all three of the increment's sign-off screens.

- [ ] **Step 14: Commit.**

```bash
git commit -m "feat(mobile-admin): add rollout kit and give CollapsingHeader an additive back affordance"
```

---

### Task 2: Products list

**Files:**
- Create: `lib/admin-api/product-status.ts`
- Modify: `app/(tabs)/products/index.tsx`
- Test: `__tests__/products-screen.test.tsx` (new or extend)

**Interfaces consumed:** `useCollapsingScroll`, `useBusyIds`, `assertSwipeConvention`, `assertNoAutoFire` (Task 1); `CollapsingHeader` with `onBack` (not used here — Products is a tab root).

**Interface produced:**

```ts
// lib/admin-api/product-status.ts
import type { ProductStatus } from "@repo/mobile-shared/api/schemas/product";

/**
 * The one mutation this increment adds. A thin, single-purpose wrapper over
 * the same PATCH `useUpdateProduct` uses, so it inherits the invalidation
 * that already makes the list refetch itself:
 *   invalidate ["products"]  → reaches ["products", status, search]
 *   invalidate ["product", id] → the detail screen, which is NOT under that prefix
 */
export function useSetProductStatus(): UseMutationResult<
  Product,
  unknown,
  { id: string; status: ProductStatus }
>;
```

**Actions.** `PATCH .../products/:id {status}` is idempotent with no gating, so both swipes fire directly.

| Gesture / item | Action | Tone | Shown when | Fires directly? |
|---|---|---|---|---|
| Leading swipe (drag right) | **Activate** | `accent` | `status !== "active"` | ✅ direct |
| Trailing swipe (drag left) | **Set to draft** | `neutral` | `status === "active"` | ✅ direct — dismissive, not destructive |
| Long-press item 1 | **Edit** → `router.push("/(tabs)/products/[id]")` | default | always | navigation |
| Long-press item 2 | **Activate** | default | always, `disabled` when already active | ✅ direct |
| Long-press item 3 | **Set to draft** | default | always, `disabled` when already draft | ✅ direct |
| Long-press item 4 | **Archive** | `danger` | always, `disabled` when already archived | **Confirm `Alert.alert` first** |

The sheet always renders **four** items — `snapPoints` memoises on `items.length`, so illegal ones are `disabled`, never dropped.

**🔴 The Archived chip is part of this task and is not optional.** The chips today are `all / active / draft`. If `all` does not include archived products (verify on device — issue a request with no status filter and check an archived product comes back), an archived product vanishes from every mobile filter and Archive becomes irreversible from the phone. Add a fourth chip `{ key: "archived", label: "Archived" }` mapping to `status=archived`, exactly as the seven-screen chip rollout mapped every other key to its literal status value. **Do not ship the Archive action without it.**

- [ ] **Step 1: Write the failing tests.**

```tsx
// __tests__/products-screen.test.tsx
import { assertSwipeConvention, assertNoAutoFire, swipeRow, type RowActions }
  from "@/test-utils/swipe-convention";

describe("Products — swipe", () => {
  it("offers Activate on the LEADING edge for a draft product, in the accent tone", () => {
    const { UNSAFE_root } = render(<ProductsScreen />);
    const row = swipeRow(UNSAFE_root, "swipe-p-draft")?.props as RowActions;
    expect(row.leadingActions).toHaveLength(1);
    expect(row.leadingActions?.[0]).toMatchObject({ key: "activate", tone: "accent" });
  });

  it("offers Set to draft on the TRAILING edge for an active product, in the NEUTRAL tone", () => {
    const { UNSAFE_root } = render(<ProductsScreen />);
    const row = swipeRow(UNSAFE_root, "swipe-p-active")?.props as RowActions;
    expect(row.trailingActions?.[0]).toMatchObject({ key: "draft", tone: "neutral" });
  });

  it("offers no Activate on an already-active product", () => {
    const { UNSAFE_root } = render(<ProductsScreen />);
    const row = swipeRow(UNSAFE_root, "swipe-p-active")?.props as RowActions;
    expect(row.leadingActions ?? []).toHaveLength(0);
  });

  it("holds the app-wide swipe convention on every row", () => {
    const { UNSAFE_root } = render(<ProductsScreen />);
    assertSwipeConvention(UNSAFE_root);
  });

  it("never opts any action into full-swipe auto-fire", () => {
    const { UNSAFE_root } = render(<ProductsScreen />);
    assertNoAutoFire(UNSAFE_root);
  });
});

describe("Products — long-press menu", () => {
  it("always renders four items so the sheet never resizes", () => {
    // open the menu on an active product and on an archived one
    // expect both to render exactly 4 ActionSheet items
  });

  it("disables Activate on an already-active product rather than dropping it", () => { /* … */ });

  it("puts Archive behind a confirm and does not fire the mutation until it is accepted", () => {
    const alertSpy = jest.spyOn(Alert, "alert");
    // open menu, press Archive
    expect(setStatus).not.toHaveBeenCalled();
    // invoke the destructive button's onPress from the spy's captured args
    expect(setStatus).toHaveBeenCalledWith({ id: "p1", status: "archived" });
  });

  it("suppresses BOTH the swipe and the long-press while that row's request is open", () => {
    // fire Activate on p1, then assert:
    //   swipeRow(root, "swipe-p1").props.enabled === false
    //   the ProductRow's onLongPress prop is undefined
    // and that a SECOND row is still enabled and still long-pressable
  });

  it("offers an Archived filter chip", () => {
    const { getByTestId } = render(<ProductsScreen />);
    expect(getByTestId("filter-chip-archived")).toBeTruthy();
  });
});
```

Fixtures: three products — one `draft`, one `active`, one `archived`. **The archived one is the fixture that exposes a missing `disabled` gate;** a two-status fixture passes against broken code.

- [ ] **Step 2: Run, confirm failure.**

- [ ] **Step 3: Implement `useSetProductStatus`.**

Model it on `useUpdateProduct` (`lib/admin-api/product-crud.ts:33-45`) — same API call, same two invalidations (`["products"]` and `["product", id]`). Do not add a third invalidation; `["dashboard"]` has no product-status field.

- [ ] **Step 4: Wire the screen.**

Replace `PageHeader` (line ~77) with `CollapsingHeader eyebrow="CATALOGUE" title="Products"` and the pending/total count in `rightSlot` if one is cheaply available; the existing `SearchField` stays **pinned** above the chips (the scroll-revealed variant was proven undiscoverable on device and reverted on Orders — do not reintroduce it). Wrap the list in `Animated.FlatList` with `onScroll`/`scrollEventThrottle={16}`. Add the Archived chip. Then:

```tsx
const busy = useBusyIds();
const setStatus = useSetProductStatus();

const actionsFor = useCallback(
  (p: Product): { leading?: SwipeAction[]; trailing?: SwipeAction[] } | null => {
    const leading: SwipeAction[] =
      p.status === "active"
        ? []
        : [{
            key: "activate",
            label: "Activate",
            tone: "accent",
            icon: <Check size={ICON_SIZE} color={theme.colors.inverse} strokeWidth={2} />,
            onPress: () => {
              busy.markBusy(p.id);
              setStatus.mutate({ id: p.id, status: "active" }, busy.settleCallbacks(p.id));
            },
          }];

    const trailing: SwipeAction[] =
      p.status === "active"
        ? [{
            key: "draft",
            // Dismissive, not destructive: reversible, idempotent, and the
            // trailing edge is a POSITION not a tone (see the ticket Close row
            // on the Dashboard, which proves the same point).
            label: "Draft",
            tone: "neutral",
            icon: <FileText size={ICON_SIZE} color={theme.colors.text} strokeWidth={2} />,
            onPress: () => {
              busy.markBusy(p.id);
              setStatus.mutate({ id: p.id, status: "draft" }, busy.settleCallbacks(p.id));
            },
          }]
        : [];

    // An archived product has neither gesture — an armed gesture that can only
    // no-op is worse than no gesture. Return null so no SwipeRow is mounted.
    if (leading.length === 0 && trailing.length === 0) return null;
    return { leading, trailing };
  },
  [busy, setStatus],
);
```

`renderItem` mirrors Orders exactly: `<Hairline inset={theme.spacing.xl} />` for `index > 0`, `SwipeRow` only when `actionsFor` returns non-null, `enabled={!busy.isBusy(item.id)}`, and `onLongPress={busy.isBusy(item.id) ? undefined : setMenuProduct}`.

Archive's confirm:

```tsx
const confirmArchive = useCallback((p: Product) => {
  Alert.alert(
    "Archive product?",
    `"${p.title}" will be removed from your storefront. You can find it again under the Archived filter and re-activate it.`,
    [
      { text: "Cancel", style: "cancel" },
      {
        text: "Archive",
        style: "destructive",
        onPress: () => {
          busy.markBusy(p.id);
          setStatus.mutate({ id: p.id, status: "archived" }, busy.settleCallbacks(p.id));
        },
      },
    ],
  );
}, [busy, setStatus]);
```

- [ ] **Step 5: Run, gates.**

- [ ] **Step 6: Device gate — destructive verification.**

1. `npx expo start --dev-client --clear --port 8082`; confirm the bundle count.
2. Screenshot Products expanded and collapsed.
3. **Swipe with `idb`** (`idb ui swipe … --delta`): drag a draft row RIGHT — verify it **settles open** and fires nothing until you tap Activate. Then tap it. Verify the row's badge changes to Active after the refetch, with no optimistic hiding involved.
4. Drag an active row LEFT — verify Draft appears in the **neutral** (sink) tone, not danger, and again settles open rather than firing.
5. 🔴 **Archive:** long-press an active product → tap Archive → verify the confirm Alert appears and **nothing happens if you Cancel**. Accept it. Then **switch to the Archived chip and confirm the product is there**, and re-activate it from the sheet. If the product cannot be found again, the chip is wrong and Archive must not ship.
6. 🔴 Long-press an **archived** product and verify Archive is present-but-greyed, not missing — and that the sheet is the **same height** as on an active product.
7. Fire Activate and immediately try to swipe and long-press the same row: both must be inert; a neighbouring row must still respond.
8. `content_size accessibility-large`, terminate, relaunch, screenshot.

- [ ] **Step 7: Commit.**

```bash
git commit -m "feat(mobile-admin): add collapsing header, status swipes and long-press menu to Products"
```

---

### Task 3: Customers list

Customers has **no swipe** per §3 — and the backend agrees: the only state-changing action is Block, whose reason is mandatory, so it can never be a fire-on-tap gesture.

**Files:**
- Create: `components/customers/BlockReasonSheet.tsx`
- Modify: `app/(tabs)/customers/index.tsx`
- Test: `__tests__/customers-screen.test.tsx`, `__tests__/block-reason-sheet.test.tsx`

**Interface produced:**

```ts
// components/customers/BlockReasonSheet.tsx
export interface BlockReasonSheetHandle {
  present: () => void;
  dismiss: () => void;
}
export interface BlockReasonSheetProps {
  /** Named in the sheet copy so the merchant can see who they are blocking. */
  customerLabel?: string;
  onSubmit: (reason: string) => void;
  isSubmitting: boolean;
  /** Local error string, never `mutation.error` — react-query never resets one. */
  error: string | null;
  /** Fires on EVERY close, including back-out, so the target can be released. */
  onDismiss: () => void;
}
```

Model it verbatim on `components/orders/CancelReasonSheet.tsx`: `BottomSheetModal`, `snapPoints={["52%"]}`, `enableDynamicSizing={false}`, imperative ref handle, does not self-dismiss on submit, `BottomSheetBackdrop` with `pressBehavior="close"` and a flat ink scrim (never a blur). **Submit is disabled until the reason is non-empty** — the backend's `binding:"required"` rejects an empty string, and a 400 the merchant could have been shown inline is a failure of the client.

**Long-press menu — four items, always four:**

| Item | Action | Tone | `disabled` when |
|---|---|---|---|
| Email | `Linking.openURL(\`mailto:${email}\`)` | default | never (every customer has an email) |
| Call | `Linking.openURL(\`tel:${phone}\`)` | default | **no phone on the customer** |
| Block | opens `BlockReasonSheet` | `danger` | `status === "blocked"` |
| Unblock | `useUnblockCustomer` — fires directly | default | `status !== "blocked"` |

§3 lists three items; Unblock is added because Block is only reversible from mobile if it is there, and the same argument that forced the Archived chip applies. Four items, constant length.

- [ ] **Step 1: Write the failing tests.**

Fixtures — **all four of these, because the demo store's only customer is the no-name/no-phone one and it passes against almost any bug**:
- `c-named` — has `first_name`/`last_name` and a phone
- `c-noname` — email only, no phone (the demo store's real shape)
- `c-blocked` — `status: "blocked"`
- `c-nophone` — has a name, no phone

```tsx
describe("Customers — long-press menu", () => {
  it("renders exactly four items on every customer shape", () => { /* c-named, c-noname, c-blocked */ });

  it("disables Call when the customer has no phone, rather than dropping it", () => { /* … */ });

  it("opens tel: with the phone number when Call is enabled", () => { /* … */ });

  it("disables Block on an already-blocked customer and enables Unblock", () => { /* … */ });

  // The action with a REQUIRED reason must never reach the API without one.
  it("opens the reason sheet for Block and does NOT call the mutation on tap", () => {
    // press Block; expect blockCustomer.mutate not called; expect sheet present()ed
  });

  it("fires Unblock directly (no body, idempotent)", () => { /* … */ });

  it("mounts no SwipeRow on any row", () => {
    const { UNSAFE_root } = render(<CustomersScreen />);
    expect(swipeRows(UNSAFE_root)).toHaveLength(0);
  });

  it("suppresses long-press while that customer's own request is open", () => { /* … */ });

  it("titles the sheet with the customer identity, not a duplicated email", () => {
    // uses lib/customer-identity.ts — for c-noname the title IS the email and
    // there is no subtitle; asserting against c-named alone would pass against
    // the exact bug customer-identity was written to fix.
  });
});

describe("BlockReasonSheet", () => {
  it("keeps submit disabled while the reason is empty", () => { /* … */ });
  it("passes the trimmed reason to onSubmit", () => { /* … */ });
  it("renders the error prop and does not read mutation.error", () => { /* … */ });
  it("fires onDismiss on backdrop tap as well as on submit", () => { /* … */ });
});
```

- [ ] **Step 2: Run, confirm failure.**

- [ ] **Step 3: Implement `BlockReasonSheet`.**

- [ ] **Step 4: Wire the screen.**

`PageHeader` (line ~89) → `CollapsingHeader eyebrow="PEOPLE" title="Customers"`. `SearchField` stays pinned. `Animated.FlatList` + `onScroll`. Add `useBusyIds`, the `ActionSheet`, `menuCustomer` state, `blockTarget` state, `blockError` local state, and the sheet ref. **`onDismiss={() => setBlockTarget(null)}`** on the sheet — the Orders bug where a backed-out sheet left the target pinned and leaked into the next row's sheet is exactly reproducible here.

Use `customerIdentity(customer)` from `lib/customer-identity.ts` for the sheet title. Do not write a fourth private copy of "who is this customer".

- [ ] **Step 5: Gates.**

- [ ] **Step 6: Device gate — destructive verification.**

1. Cleared-cache start; screenshot Customers expanded and collapsed.
2. Long-press the **no-name** customer: verify the sheet title shows the email once, not twice, and that Call is greyed out.
3. Long-press the **named, phoned** customer: verify Call is enabled; tap it and confirm the dialler intent fires (a simulator will show the confirm sheet; that is enough).
4. 🔴 **Block:** long-press → Block → verify the **reason sheet opens and nothing is sent**. Verify Submit is disabled with an empty field. Enter a reason, submit, and confirm the row's badge flips to Blocked after the list refetches — **with no optimistic hiding**.
5. 🔴 Dismiss a Block sheet **without submitting** on customer A, then long-press customer B and open Block. Verify B's sheet names **B**, not A. (This is the exact class of bug Orders shipped.)
6. Long-press the blocked customer: verify Block is greyed, Unblock is enabled, and the sheet is the same height. Unblock and verify the badge clears.
7. `content_size accessibility-large`, terminate, relaunch, screenshot — the sheet copy holds a customer email, the string that broke mid-token on the customer detail screen.

- [ ] **Step 7: Commit.**

```bash
git commit -m "feat(mobile-admin): add collapsing header and long-press actions to Customers"
```

---

### Task 4: Reviews

The cleanest swipe screen in the increment: approve and reject take **no body** and are **explicitly idempotent** on the server (`reviews.go:110-113`, `:144-147`), so both fire directly with no confirm.

**Files:** Modify `app/(tabs)/customers/reviews/index.tsx`. Test `__tests__/reviews-screen.test.tsx`.

**Actions:**

| Gesture / item | Action | Tone | Shown when | Fires directly? |
|---|---|---|---|---|
| Leading swipe | **Approve** | `accent` | `status === "pending"` | ✅ direct |
| Trailing swipe | **Reject** | `danger` | `status === "pending"` | ✅ direct |
| Long-press 1 | **Reply** → `router.push` to the review detail | default | always | navigation |
| Long-press 2 | **Approve** | default | `disabled` when `status === "approved"` | ✅ direct |
| Long-press 3 | **Reject** | `danger` | `disabled` when `status === "rejected"` | ✅ direct |

**Report is CUT** — no endpoint exists. Three items, constant length.

A non-pending review gets **no `SwipeRow` at all** (return `null` from `actionsFor`), matching Orders' terminal-status rule: an armed gesture that can only be a no-op is worse than no gesture.

**Reject fires directly despite the danger tone** — it is idempotent, and it is reversible by approving. It is `danger` because it is the trailing/negative outcome for the customer's review, and the tone drives the paint assertion in the invariant test.

- [ ] **Step 1: Write the failing tests.**

Fixtures: one `pending`, one `approved`, one `rejected`. Include the paint assertions:

```tsx
it("paints Approve moss and Reject danger", () => {
  const { getByTestId } = render(<ReviewsScreen />);
  const approve = StyleSheet.flatten(getByTestId("swipe-rv1-action-approve").props.style);
  const reject = StyleSheet.flatten(getByTestId("swipe-rv1-action-reject").props.style);
  expect(approve.backgroundColor).toBe(theme.colors.accent);
  expect(reject.backgroundColor).toBe(theme.colors.danger);
});

it("holds the app-wide swipe convention on every row", () => {
  assertSwipeConvention(render(<ReviewsScreen />).UNSAFE_root);
});

it("never opts any action into full-swipe auto-fire", () => {
  assertNoAutoFire(render(<ReviewsScreen />).UNSAFE_root);
});

it("mounts no SwipeRow on an already-approved review", () => { /* … */ });
```

- [ ] **Step 2-4: Run, implement, run.**

Header: `BackHeader` (line ~63) → `CollapsingHeader eyebrow="CUSTOMERS" title="Reviews" onBack={() => router.back()}`. Chips stay. `Animated.FlatList` + `onScroll`. `useBusyIds` gating both the swipe and the long-press. Mutations are `useApproveReview` / `useRejectReview` (`lib/admin-api/review-actions.ts`) — they already invalidate `["reviews"]`, `["review", id]` **and** `["dashboard"]`, so the Dashboard's pending-review queue rows and its `stats.pending_reviews` "See all N" row both update. Do not add or remove an invalidation.

- [ ] **Step 5: Gates.**

- [ ] **Step 6: Device gate — destructive verification.**

1. Cleared-cache start; screenshot expanded and collapsed.
2. `idb ui swipe` a pending review RIGHT: verify it **settles open**, fires nothing, then tap Approve. Verify the badge flips.
3. Swipe LEFT: verify Reject is danger-toned and **also settles open** — a full swipe must not fire it.
4. 🔴 Approve a review, then **switch to the Dashboard** and confirm the queue no longer lists it and the "See all N pending reviews" count has dropped. This is the one cross-screen invalidation in the increment and it is only visible on device.
5. Verify an approved review has no swipe at all and that its sheet greys Approve.
6. `accessibility-large`, terminate, relaunch, screenshot — review rows hold free-text customer content.

- [ ] **Step 7: Commit.**

```bash
git commit -m "feat(mobile-admin): add collapsing header, approve/reject swipes and long-press menu to Reviews"
```

---

### Task 5: Tickets

**🔴 The one screen where the action is NOT idempotent and the gate is load-bearing.** `PATCH .../tickets/:id {status:"closed"}` on an already-closed ticket returns **HTTP 409** (`CanTransitionTo` returns false for same-status, `ticket/models.go:112-114`), and **`closed` is terminal — it cannot be reopened** (`:129`).

**Files:** Modify `app/(tabs)/more/settings/tickets/index.tsx`. Test `__tests__/tickets-screen.test.tsx`.

**Actions:**

| Gesture / item | Action | Tone | Shown when | Fires directly? |
|---|---|---|---|---|
| Leading swipe | — | — | never | §3 gives tickets no constructive swipe |
| Trailing swipe | **Close** | `neutral` | `status !== "closed"` | **Confirm `Alert.alert`** — one-way |
| Long-press 1 | **Reply** → `router.push` to ticket detail | default | always | navigation |
| Long-press 2 | **Close** | `danger` | `disabled` when `status === "closed"` | **Confirm `Alert.alert`** |

**Assign is CUT** — no assignee model and no route. Two items.

**Close's tone is `neutral` on the swipe and `danger` in the sheet, deliberately.** Closing a resolved ticket is a normal outcome, not a destruction — this is the same reasoning the Dashboard's ticket row already uses, and it is the row that proves the invariant test isn't merely "trailing means danger". In the sheet, where there is no side to carry meaning, `danger` marks it as the one-way action. Both tones are asserted.

**Close is confirmed even though it is trailing-swipe-revealed-then-tapped.** Everywhere else in this plan a revealed tap is enough, because the action is reversible. Close is not: there is no reopen endpoint. One extra tap is the correct price.

- [ ] **Step 1: Write the failing tests.**

Fixtures: **one `open` and one `closed` ticket — the closed one is mandatory.** Without it the 409-avoidance gate is untested and a store with only open tickets passes against completely broken code.

```tsx
it("mounts NO SwipeRow on a closed ticket", () => {
  const { UNSAFE_root } = render(<TicketsScreen />);
  expect(swipeRow(UNSAFE_root, "swipe-t-closed")).toBeUndefined();
});

it("puts Close on the trailing edge in the NEUTRAL tone, with nothing leading", () => {
  const row = swipeRow(render(<TicketsScreen />).UNSAFE_root, "swipe-t-open")?.props as RowActions;
  expect(row.leadingActions ?? []).toHaveLength(0);
  expect(row.trailingActions?.[0]).toMatchObject({ key: "close", tone: "neutral" });
});

it("does not fire the mutation until the confirm is accepted", () => { /* Alert spy, as in Task 2 */ });

it("disables Close in the sheet on a closed ticket rather than dropping it", () => { /* … */ });

it("holds the app-wide swipe convention", () => {
  assertSwipeConvention(render(<TicketsScreen />).UNSAFE_root);
});

it("never opts any action into full-swipe auto-fire", () => {
  assertNoAutoFire(render(<TicketsScreen />).UNSAFE_root);
});

// The exclusion is deliberate and already pinned elsewhere; pin it here too so
// nobody "fixes" it while wiring this screen.
it("does not invalidate the dashboard (its schema has no ticket field)", () => { /* … */ });
```

- [ ] **Step 2-4: Run, implement, run.**

Header: `BackHeader` (line ~96) → `CollapsingHeader eyebrow="SUPPORT" title="Tickets" onBack={() => router.back()}` and keep the existing `IconButton` Plus as `rightSlot` (it currently lives on `BackHeader.rightSlot`). Chips stay. `useUpdateTicketStatus` from `lib/admin-api/ticket-actions.ts`. `useBusyIds` gating both controls. The inline `TicketRow` (line ~38) is already 88pt and needs no density work.

- [ ] **Step 5: Gates.**

- [ ] **Step 6: Device gate — destructive verification.**

1. Cleared-cache start; screenshot expanded and collapsed.
2. 🔴 Verify a **closed** ticket cannot be swiped at all (`idb ui swipe` left on it and confirm nothing is revealed), and that long-pressing it greys Close while keeping the sheet the same height.
3. 🔴 Swipe an **open** ticket left → verify Close is the neutral sink tone, settles open, and **shows a confirm Alert on tap**. Cancel it and confirm the ticket is still open.
4. Accept the confirm. Verify the row's badge becomes Closed after the refetch, and that the row is now un-swipeable. **Do not re-close it** — there is no reopen.
5. `accessibility-large`, terminate, relaunch, screenshot — ticket subjects are free text and the row is two-line.

- [ ] **Step 7: Commit.**

```bash
git commit -m "feat(mobile-admin): add collapsing header, gated close swipe and long-press menu to Tickets"
```

---

### Task 6: Coupons

**Files:** Modify `app/(tabs)/more/marketing/coupons/index.tsx`. Test `__tests__/coupons-screen.test.tsx`.

**Actions.** `PATCH .../coupons/:id {status}` accepts only `active` and `disabled` (`expired` is system-managed and rejected by the service), is idempotent, and is fully reversible.

| Gesture / item | Action | Tone | Shown when | Fires directly? |
|---|---|---|---|---|
| Leading swipe | **Enable** | `accent` | `status === "disabled"` | ✅ direct |
| Trailing swipe | **Disable** | `neutral` | `status === "active"` | ✅ direct — reversible, dismissive |
| Long-press 1 | **Edit** → `router.push` to the coupon detail | default | always | navigation |
| Long-press 2 | **Enable** | default | `disabled` unless `status === "disabled"` | ✅ direct |
| Long-press 3 | **Disable** | default | `disabled` unless `status === "active"` | ✅ direct |

**§3's "Delete" is CUT and this is not a scope decision, it is a correctness one.** `DELETE /coupons/:id` does not delete: it sets `status='disabled'`, returns `200 {"message":"coupon disabled"}`, logs the audit action as `coupon.deactivated`, and **leaves the row in the list**. A menu item labelled "Delete" that leaves the coupon visible is a lie to the merchant, and "Disable" is already offered above it by the honest endpoint. **Duplicate is CUT** — no endpoint.

An `expired` or `scheduled` coupon gets **no `SwipeRow`** — neither transition is legal.

- [ ] **Step 1: Write the failing tests.**

Fixtures: one `active`, one `disabled`, one `expired`. The expired one is the fixture that exposes a missing legality gate.

Include `assertSwipeConvention` and `assertNoAutoFire`, the paint assertions (Enable = `theme.colors.accent`, Disable = `theme.colors.sink`), the constant-item-count assertion, and the busy-guard-on-both-controls assertion. Add one explicitly:

```tsx
it("offers no Delete item (the endpoint only disables and the row would stay)", () => {
  // open the menu; expect exactly 3 items and none labelled /delete/i
});
```

- [ ] **Step 2-4: Run, implement, run.**

Header: `BackHeader` (line ~66) → `CollapsingHeader eyebrow="MARKETING" title="Coupons" onBack={() => router.back()}`. Chips stay. Use `usePatchCoupon` (`lib/admin-api/coupon-actions.ts:29`) — there is no dedicated toggle hook and none is needed. `useBusyIds` on both controls.

- [ ] **Step 5: Gates.**

- [ ] **Step 6: Device gate.**

1. Cleared-cache start; screenshot expanded and collapsed.
2. Swipe a disabled coupon RIGHT → settles open → tap Enable → badge flips after the refetch.
3. Swipe an active coupon LEFT → verify Disable is the **neutral** tone, not danger, and settles open.
4. Verify an expired coupon has no swipe and greys both toggles in the sheet at constant sheet height.
5. Verify no Delete item is present anywhere.
6. `accessibility-large`, terminate, relaunch, screenshot — coupon codes are tabular-nums monospace and are a known latent mid-token-break risk.

- [ ] **Step 7: Commit.**

```bash
git commit -m "feat(mobile-admin): add collapsing header, enable/disable swipes and long-press menu to Coupons"
```

---

### Task 7: Campaigns and Segments

Grouped because they share one shape — **no swipe, one long-press sheet whose only real action is an irreversible Delete** — and because they share `segment-actions.ts`'s cross-invalidation.

**Files:** Modify `app/(tabs)/more/marketing/campaigns/index.tsx`, `app/(tabs)/more/marketing/segments/index.tsx`. Test `__tests__/campaigns-screen.test.tsx`, `__tests__/segments-screen.test.tsx`.

**Campaigns:**

| Item | Action | Tone | `disabled` when | Fires directly? |
|---|---|---|---|---|
| Edit | `router.push` to the campaign detail | default | never | navigation |
| Delete | `useDeleteCampaign` | `danger` | 🔴 **`status !== "draft"`** | **Confirm `Alert.alert`** |

`DELETE /campaigns/:id` returns **HTTP 409 `campaign_not_draft`** for anything that is not a draft (`campaign/service.go:197-205`), and 404s on a second call. Duplicate is CUT — no endpoint. Two items.

**Segments:**

| Item | Action | Tone | `disabled` when | Fires directly? |
|---|---|---|---|---|
| Edit | `router.push` to the segment detail | default | never | navigation |
| Delete | `useDeleteSegment` | `danger` | never client-side — the list has no campaign-linkage field to gate on | **Confirm `Alert.alert` naming the segment**; on rejection, surface the server's reason |

`DELETE /segments/:id` is a **hard delete**, but `campaigns.segment_id` is a plain FK the service now enforces *before* deleting: a segment still referenced by a campaign is refused with **`409 segment_in_use`** (`campaign/service.go:273-286`, `campaign/repository.go:125-141`), and `ApiError.message` is already the server's prose — e.g. *"segment is still used by 2 campaigns and cannot be deleted"* — so there is nothing to orphan and no count to re-derive client-side. The confirm copy only needs to warn that the delete is permanent; the mutation's `onError` is what tells the merchant *why* it didn't happen:

```tsx
Alert.alert(
  "Delete segment?",
  `"${segment.name}" will be permanently deleted. This cannot be undone.`,
  [
    { text: "Cancel", style: "cancel" },
    { text: "Delete", style: "destructive", onPress: () => { /* markBusy + mutate */ } },
  ],
);

// useDeleteSegment's onError. ApiError doesn't carry `details`, but for
// segment_in_use the message text already states the blocking campaign
// count — surface it verbatim rather than inventing generic copy.
function onDeleteSegmentError(err: unknown, segment: { name: string }) {
  const reason =
    err instanceof ApiError && err.code === "segment_in_use"
      ? err.message
      : "Something went wrong deleting the segment.";
  Alert.alert("Can't delete segment", reason);
}
```

Segments is the **only list screen with no `FilterChips`** and needs none — the resource has no status axis. Do not add one.

- [ ] **Step 1: Write the failing tests.**

Campaign fixtures: 🔴 **one `draft` AND one non-draft (`sent` or `scheduled`)**. A draft-only store passes against a missing gate.
Segment fixtures: two segments, one of which your device fixture has a campaign pointing at.

```tsx
// campaigns
it("disables Delete on a non-draft campaign rather than dropping it", () => { /* … */ });
it("enables Delete on a draft campaign", () => { /* … */ });
it("does not fire the mutation until the confirm is accepted", () => { /* Alert spy */ });
it("mounts no SwipeRow on any row", () => {
  expect(swipeRows(render(<CampaignsScreen />).UNSAFE_root)).toHaveLength(0);
});
it("offers no Duplicate item (no endpoint exists)", () => { /* … */ });

// segments
it("names the segment in the confirm and states the delete is permanent", () => {
  const spy = jest.spyOn(Alert, "alert");
  // press Delete
  expect(spy.mock.calls[0][1]).toContain("permanently deleted");
  expect(spy.mock.calls[0][1]).toContain("Summer buyers"); // the fixture's name
});
it("surfaces the server's reason when delete is blocked with 409 segment_in_use", () => {
  // mock the mutation rejecting with
  // new ApiError(409, "segment_in_use", "segment is still used by 1 campaign and cannot be deleted")
  const spy = jest.spyOn(Alert, "alert");
  // accept the confirm; the mutation rejects
  expect(spy.mock.calls[1][0]).toBe("Can't delete segment");
  expect(spy.mock.calls[1][1]).toContain("still used by 1 campaign");
});
it("invalidates both segments and campaigns on a successful delete", () => { /* the existing hook does; pin it */ });
```

- [ ] **Step 2-4: Run, implement, run.**

Both headers: `BackHeader` → `CollapsingHeader eyebrow="MARKETING" title="Campaigns" | "Segments" onBack={() => router.back()}`. Campaigns keeps its chips; segments has none. Both get `Animated.FlatList` + `onScroll`, `useBusyIds` on the long-press only (no swipe to gate).

Segments currently uses a plain query with no infinite scroll — leave the pagination as it is; this task is header + press + menu.

- [ ] **Step 5: Gates.**

- [ ] **Step 6: Device gate — destructive verification.**

1. Cleared-cache start; screenshot both screens, expanded and collapsed.
2. 🔴 **Campaigns:** long-press the **non-draft** campaign and verify Delete is greyed and the sheet is the same height as on the draft. Long-press the draft, tap Delete, **Cancel** the confirm, verify the campaign is still there. Accept it, verify it disappears after the refetch.
3. 🔴 **Segments:** long-press the segment a campaign targets, tap Delete, and accept the confirm — verify the delete is refused with a **"Can't delete segment"** alert naming why (referenced by its campaign) and that the segment is still in the list afterward. The 409 means this is safe to actually attempt; nothing is destroyed. Then delete the **other**, unreferenced segment and verify it disappears.
4. Verify neither screen mounts a swipe (drag a row and confirm nothing is revealed).
5. `accessibility-large`, terminate, relaunch, screenshot both.

- [ ] **Step 7: Commit.**

```bash
git commit -m "feat(mobile-admin): add collapsing headers and gated delete menus to Campaigns and Segments"
```

---

### Task 8: Order detail — sticky bottom action bar

Not a list rollout. Today every action is a stack of full-width buttons at the **bottom of the scroll content** (`orders/[id].tsx:428-467`), so on a long order the merchant scrolls past line items, totals, addresses, shipping and documents before reaching "Confirm order". §3 moves them into a bar that is always in the thumb.

**Files:**
- Create: `components/ui/StickyActionBar.tsx`
- Modify: `app/(tabs)/orders/[id].tsx`, `app/(tabs)/products/new.tsx` (adopt the primitive), `components/ui/index.ts`
- Test: `__tests__/sticky-action-bar.test.tsx`, `__tests__/order-detail-screen.test.tsx` (extend)

**Interface produced:**

```ts
// components/ui/StickyActionBar.tsx
export interface StickyActionBarProps {
  children: ReactNode;
  /**
   * Distance from the bottom of the SCREEN to the bottom of the bar.
   * The caller owns this because the two callers sit in different worlds:
   *   - a (tabs) screen must clear the floating dock  → BAR_BOTTOM below
   *   - a presentation:"modal" screen covers the dock → useSafeAreaInsets().bottom
   * Getting this wrong is the modal sheet trap in a different costume.
   */
  bottom: number;
  testID?: string;
}
/** The bar's own height excluding `bottom`, so callers can size scroll padding. */
export const STICKY_BAR_HEIGHT: number;
```

Extracted verbatim from the one existing sticky footer in the app, `products/new.tsx:318-329`: `position:"absolute", left:0, right:0`, `flexDirection:"row"`, `gap: theme.spacing.sm`, `paddingHorizontal: theme.spacing.lg`, `paddingVertical: theme.spacing.md`, `backgroundColor: theme.colors.background`, `borderTopWidth: theme.hairline`, `borderTopColor: theme.colors.hairline`. Full-bleed with a hairline top rule — **not** a floating pill, and **no blur**.

**Dock interaction, settled here.** The dock is `position:"absolute", left:12, right:12`, 64pt tall, at `bottom: insets.bottom + DOCK_BOTTOM_GAP`. Order detail is inside `(tabs)`, so the dock is visible. The bar sits **above** it:

```ts
const insets = useSafeAreaInsets();
const BAR_BOTTOM = insets.bottom + DOCK_BOTTOM_GAP + DOCK_HEIGHT + theme.spacing.xs;
// scroll padding must clear BOTH:
contentContainerStyle={[styles.scroll, { paddingBottom: BAR_BOTTOM + STICKY_BAR_HEIGHT + theme.spacing.md }]}
```

`useDockClearance()` is **not** used on this screen any more — it computes clearance for content, and the content now has to clear the bar as well. Import `DOCK_HEIGHT` / `DOCK_BOTTOM_GAP` from `@/components/navigation/dock-metrics` directly.

**Bar contents — constant height at every order state.** Two slots:

| Slot | Content |
|---|---|
| Primary (flex:1) | The single highest-priority legal action: **"Confirm order"** when `status === "pending"`; **"Mark fulfilled"** when `status === "confirmed"`; otherwise a **non-interactive `bodyEmphasis` caption** stating the terminal state ("Order fulfilled" / "Order cancelled"), in the same 48pt box |
| Overflow (44pt `IconButton`) | `MoreHorizontal` glyph → opens `ActionSheet` |

The caption fallback exists so the **bar height and the scroll padding never change with order state**. A bar that appears and disappears reflows the whole screen under the merchant's thumb, and a variable `paddingBottom` is the same class as the six silent-clipping bugs.

**Overflow `ActionSheet` — always four items, `disabled` per legality** (identical construction to the Orders list menu, so `snapPoints` never resizes):

| Item | Tone | `disabled` when | Behaviour |
|---|---|---|---|
| Refund | default | `payment_status ∉ {paid, partially_refunded}` | opens the existing `RefundSheet` |
| Email invoice | default | never (server 422s when there is no customer email; the existing handler already Alerts that) | fires `handleEmailInvoice` |
| Email receipt | default | never (server 409s before delivery; the existing handler already Alerts that) | fires `handleEmailReceipt` |
| Cancel order | `danger` | `status ∈ {cancelled, fulfilled}` | opens the existing `CancelReasonSheet` |

**Do not touch `ShippingPanel`.** Its seven actions stay inside the panel where they have context. Moving them would double the scope and lose the shipment they act on.

**Do not touch the two Documents buttons' behaviour** — but their *placement* moves into the overflow sheet (they are the Email invoice / Email receipt items above), and the Documents card keeps only its caption disclaimer.

**`useConfirmOrder`'s three-way Alert stays.** `handleConfirm` (`:168`) offers Cancel / "Confirm" / "Confirm & mark paid" — that is a real business distinction and belongs on the primary action, not in a sheet.

- [ ] **Step 1: Write the failing tests.**

```tsx
// __tests__/sticky-action-bar.test.tsx
it("renders children in a full-width bar with a hairline top rule", () => { /* … */ });
it("honours the caller's `bottom` rather than computing its own", () => { /* … */ });
it("passes a plain ARRAY style, never a function", () => {
  // the NativeWind interop trap; assert typeof style !== "function"
});

// __tests__/order-detail-screen.test.tsx
it("shows Confirm order as the primary on a pending order", () => { /* … */ });
it("shows Mark fulfilled as the primary on a confirmed order", () => { /* … */ });
it("shows a non-interactive terminal caption on a cancelled order", () => {
  // and asserts it is NOT pressable — a disabled button announces differently
});
it("keeps the bar's height identical across pending / confirmed / fulfilled / cancelled", () => {
  // measure the rendered bar style height in all four states
});
it("renders exactly four overflow items in every order state", () => { /* … */ });
it("disables Refund on an unpaid order rather than dropping it", () => { /* … */ });
it("disables Cancel on a fulfilled order", () => { /* … */ });
it("opens CancelReasonSheet rather than firing cancel (reason is REQUIRED)", () => { /* … */ });
it("opens RefundSheet rather than firing refund (refund_request_id is enforced)", () => { /* … */ });
it("no longer renders the inline action button stack", () => {
  // the old styles.actions block must be gone, or both exist and the screen
  // has two Confirm buttons
});
```

Fixtures: four orders — `pending` unpaid, `confirmed` paid, `fulfilled` paid, `cancelled`.

- [ ] **Step 2: Run, confirm failure.**

- [ ] **Step 3: Implement `StickyActionBar` and export it from the barrel.**

It imports nothing native-heavy, so no `jest.config.js` mapping is needed — but run the full suite immediately after adding it to `components/ui/index.ts`, because that barrel has transitively broken unrelated suites twice.

- [ ] **Step 4: Migrate `products/new.tsx` onto it.** Its hand-rolled footer becomes `<StickyActionBar bottom={insets.bottom}>`. **Keep `useSafeAreaInsets()`, not `useDockClearance()`** — it is the modal screen and the modal covers the dock. Its existing tests (`__tests__/new-product.test.tsx`) must stay green unedited.

- [ ] **Step 5: Rewrite Order detail's action region.** Delete `styles.actions` and the four stacked `ActionButton`s; add the bar, the overflow `ActionSheet`, and the new scroll padding. Keep both existing sheets mounted and add `onDismiss` handlers clearing `cancelError` / `refundError` (there is only one order here so there is no cross-order leak, but the stale-error class is the same).

- [ ] **Step 6: Gates.**

- [ ] **Step 7: Device gate — destructive verification.**

1. Cleared-cache start.
2. Open the **pending** order. Screenshot at scroll top and scrolled to the bottom: 🔴 verify the bar stays put in both, that it sits **above the dock with a visible gap and no overlap**, and that the last row of scroll content is fully reachable above the bar.
3. Open each of the four order states in turn and screenshot: verify the bar's height and the content's bottom padding are **visually identical** across all four, and that the terminal caption reads as a caption, not a dead button.
4. 🔴 **Cancel:** open the overflow → Cancel order → verify `CancelReasonSheet` opens and **nothing is sent**. Verify Submit is blocked with an empty reason. **Back out without submitting** and confirm the order is untouched. *Do not actually cancel a demo order — use one of the orders you created for this task.*
5. 🔴 **Refund:** on the paid `confirmed` order, open the overflow → Refund → verify `RefundSheet` opens with the correct refundable amount and that the carrier-shipment warning appears when there is a shipment. Back out. *Do not submit a real refund.*
6. Verify Refund is **greyed** (not missing) on the unpaid pending order, and Cancel is greyed on the fulfilled one, with the sheet the same height in both.
7. Fire **Confirm order** on the pending fixture and verify the primary slot becomes "Mark fulfilled" without the bar moving.
8. `accessibility-large`, terminate, relaunch, screenshot the bar in all four states — a 48pt primary box holding "Mark fulfilled" at 2× is exactly the shape that has clipped five times in this programme.

- [ ] **Step 8: Commit.**

```bash
git commit -m "feat(mobile-admin): move Order detail actions into a sticky bottom bar with an overflow menu"
```

---

### Task 9: More / Account / Settings — grouped inset lists

The other §3 item that is not a list rollout. Today these screens are inconsistent: `more/index.tsx` is already a proper grouped card list; `account.tsx` and `security.tsx` use `Card` with hand-rolled inner rows at a different height; `notification-settings.tsx` and `team/index.tsx` have **no `Card` at all**. Gutters are split 16 vs 20.

**Files:**
- Create: `components/ui/GroupedList.tsx`, `components/ui/GroupedRow.tsx`
- Modify: `components/ui/index.ts`, `app/(tabs)/more/index.tsx`, `app/(tabs)/more/account.tsx`, `app/(tabs)/more/security.tsx`, `app/(tabs)/more/settings/notification-settings.tsx`, `app/(tabs)/more/settings/team/index.tsx`
- Test: `__tests__/grouped-list.test.tsx`, plus extensions to each screen's existing suite

**Interfaces produced:**

```ts
// components/ui/GroupedList.tsx
export interface GroupedListSection {
  key: string;
  /** Rendered as an eyebrow above the card. Omit for an unlabelled group. */
  label?: string;
  rows: ReactNode[];
  /** Explanatory caption below the card, e.g. notification-settings' intro copy. */
  footer?: string;
}
export interface GroupedListProps {
  sections: GroupedListSection[];
}
```

```ts
// components/ui/GroupedRow.tsx
export interface GroupedRowProps {
  label: string;
  /** lucide glyph, 18px / strokeWidth 1.75, in a 22pt slot. */
  icon?: ReactNode;
  /** Right-hand caption, e.g. account.tsx's field values. */
  value?: string;
  /** Right-hand control, e.g. a Switch. Mutually exclusive with `value`. */
  trailing?: ReactNode;
  /** Crimson count pill, e.g. more/index.tsx's unread badge. */
  badge?: string;
  /**
   * Omit for a NON-INTERACTIVE information row. That renders a plain View
   * with identical metrics — NOT a disabled PressableRow, which announces as
   * a disabled button and is wrong for a value the merchant is only reading.
   */
  onPress?: () => void;
  accessibilityLabel?: string;
  accessibilityRole?: AccessibilityRole;
  /** Trailing chevron. Defaults to `true` when `onPress` is set. */
  chevron?: boolean;
  testID?: string;
}
```

**The construction is `more/index.tsx`'s, promoted — not invented.** `GroupedList` renders per section: `Text preset="eyebrow" color="textTertiary"` at `paddingHorizontal: theme.spacing.xs` inside a `theme.spacing.xl` gutter, then `<Card padding={0}>` whose children are joined by `<Hairline inset={theme.spacing.huge + theme.spacing.xs} />` (52pt, aligning under the label column), then an optional `caption` footer. Section gap `theme.spacing.lg`. `GroupedRow` is `PressableRow lines={1}` (64pt — native row height, exactly what §3 asks for) with `backgroundColor: theme.colors.elevated`, a 22pt icon slot, `bodyEmphasis` label, and a 16pt tertiary `ChevronRight`.

**Apply to:**

| Screen | Change |
|---|---|
| `more/index.tsx` | Its local `Row` (`:42-58`) and hand-built section loop are **deleted** and replaced by `GroupedList`/`GroupedRow`. The `SECTIONS` data and the hand-written Legal group stay; Legal's `accessibilityRole="link"` is threaded through. This screen's rendering must be **visually unchanged** — it is the reference the primitive was extracted from, and any diff you can see in a screenshot is a bug in the extraction. |
| `more/account.tsx` | Profile becomes an unlabelled-icon `GroupedList` of **non-interactive** `GroupedRow`s (`value` set, no `onPress`) replacing the local `InfoRow`. Store becomes a one-row group. Sign Out and the Danger zone stay as they are — they are not list items. 🔴 **This screen has no ScrollView at all today**; add one with `paddingBottom: useDockClearance()`, since grouped lists and the danger zone together will overflow at raised text sizes. |
| `more/security.tsx` | Its hand-rolled rows (`minHeight: 56`, `paddingHorizontal: md`) become `GroupedRow` at 64/20, closing a real 8pt/4pt inconsistency with every other row in the app. |
| `more/settings/notification-settings.tsx` | The master push toggle and the five `PREFERENCE_TYPES` rows become two `GroupedList` sections of `GroupedRow` with the `Switch` in `trailing`. Its intro caption becomes the first section's `footer`. Keep the optimistic local mirror at `:120` exactly as it is. |
| `more/settings/team/index.tsx` | Member rows and invitation rows become `GroupedRow`s in two labelled sections. Members keep their role `StatusBadge` (pass it as `trailing`); the owner/pending rows keep their non-interactive treatment — pass **no `onPress`**, which now produces a genuinely non-interactive row rather than a disabled button. Invitations keep their revoke `IconButton` in `trailing`. |

**Deliberately NOT applied — these are forms, not lists:** `more/settings/branding.tsx`, `more/settings/team/invite.tsx`, `more/settings/tickets/new.tsx`, `more/settings/tickets/[id].tsx`. **Also not applied:** `more/settings/audit-logs.tsx` and `more/settings/tickets/index.tsx` are `FlatList`s with edge-to-edge rows and stay that way (tickets/index is Task 5's screen).

**Gutter normalisation:** every screen touched here moves to `theme.spacing.xl` (20) so eyebrow, card and rows share one left edge. Do **not** change `theme.spacing.lg` itself.

- [ ] **Step 1: Write the failing tests.**

```tsx
// __tests__/grouped-list.test.tsx
it("renders a labelled section as eyebrow + card + inset hairlines between rows", () => { /* … */ });
it("renders no hairline above the first row or below the last", () => { /* … */ });
it("renders an unlabelled section without an empty eyebrow box", () => { /* … */ });

// GroupedRow
it("renders an interactive row at the 64pt single-line height", () => { /* … */ });
it("renders a row with NO onPress as a non-interactive View, not a disabled button", () => {
  // assert: no accessibilityRole="button", no accessibilityState.disabled,
  // and pressing it does nothing. A disabled button announces "dimmed" to
  // VoiceOver, which is wrong for a value the merchant is only reading.
});
it("defaults the chevron on when onPress is set and off when it is not", () => { /* … */ });
it("passes a plain ARRAY style, never a function", () => { /* NativeWind interop */ });
```

Plus, on each migrated screen, one assertion that survives the migration: `more/index.tsx`'s unread badge and its `accessibilityRole="link"` on the two Legal rows; `account.tsx`'s profile values; `notification-settings.tsx`'s toggle behaviour; `team/index.tsx`'s owner row being non-interactive.

- [ ] **Step 2-4: Run, implement, run.** Add both to the barrel and immediately run the **full** suite.

- [ ] **Step 5: Gates.**

- [ ] **Step 6: Device gate.**

1. Cleared-cache start.
2. 🔴 Screenshot `more/index.tsx` **before and after** the extraction and compare: it must be pixel-identical apart from anti-aliasing. If it is not, the primitive does not match what it was extracted from.
3. Screenshot Account, Security, Notification settings and Team. Verify on each: one left edge shared by eyebrow, card and rows; 64pt rows; hairlines inset to 52 and absent at the card's top and bottom edges; no double-gutter (the `Card` margin plus a row's `paddingH` stacking, which produced a 36pt indent on Team once before).
4. Verify Account **scrolls** and that the Danger zone is fully reachable above the dock.
5. On Notification settings, toggle a preference and confirm the switch responds immediately (the local optimistic mirror) and does not jump back.
6. `accessibility-large`, terminate, relaunch, screenshot all five. Row labels like "Notification settings" and "Tesserix Support" at 2× in a 64pt box are the exact shape that has clipped before.

- [ ] **Step 7: Commit.**

```bash
git commit -m "feat(mobile-admin): promote grouped inset lists into a primitive and roll it across More, Account and Settings"
```

---

### Task 10: Product editor and create — density and type

§3's lightest item: "Density and type only; sheet behaviour unchanged." No new actions, no gestures, no sheets.

**Files:** Modify `app/(tabs)/products/[id].tsx`, `app/(tabs)/products/new.tsx`, `components/ui/FieldInput.tsx`, `components/products/VariantRow.tsx`. Test: extend `__tests__/product-detail-sections.test.tsx`, `__tests__/new-product.test.tsx`.

**🔴 MEASURE BEFORE CHANGING.** The four-up strip taught this programme that half of a suspected defect was already fixed. Start by screenshotting both screens at `large` and at `accessibility-large` and **listing what is actually off the baseline**. Only change what you measured.

**The known deltas, from the code read:**

| Surface | Now | Target | Note |
|---|---|---|---|
| `products/[id].tsx` Card `marginHorizontal` | `theme.spacing.lg` (16) | `theme.spacing.xl` (20) | The rest of the app is at 20; this screen's cards inset 4pt less than its own header |
| `products/[id].tsx` Eyebrow gutter | default (lg) | explicit `xl` | One left edge. Set it **per screen**, never by changing `Eyebrow`'s default — that is the change that rippled into ~15 call sites |
| `products/new.tsx` Card `marginHorizontal` | `lg` | `xl` | same |
| `VariantRow` `minHeight` | 56 | `theme.row.minHeightSingle` (64) | and `paddingHorizontal` to `theme.row.paddingH` (20) |
| `products/[id].tsx` `switchRow` | ad-hoc | `minHeight: theme.row.minHeightSingle`, `paddingVertical: theme.row.paddingV` | it is a row |
| `FieldInput` | `minHeight: 44`, `multiline: 96`, `paddingHorizontal: sm` | measure first | see the warning below |

**🔴 `FieldInput` is shared and a default change there is a deliberate exception to the additive-props rule.** Its call sites are `products/[id]`, `products/new`, `more/account` (the DELETE confirmation), `more/settings/branding` (13 inputs), `team/invite`, `tickets/new`, `tickets/[id]`. If — and only if — measurement shows its input text is below the 17pt `body` baseline or its box below 44pt:

1. Change it once, at the primitive.
2. **Enumerate every call site in the commit message.**
3. **Screenshot every one of the seven screens** in the device gate, at `large` and at `accessibility-large`.

If measurement shows it is already on the baseline, change nothing and record that — this is exactly the "already fixed by the cap" outcome, and recording it prevents the next pass from re-litigating it.

**Do not touch:**
- `products/new.tsx`'s `SegmentedControl` — it is a form field for Status inside a Card, not a list filter, and the earlier ruling explicitly kept it.
- `products/new.tsx`'s local `BottomSheetModalProvider` (`:159`) or its `useSafeAreaInsets()` — the modal sheet trap.
- The header band's 72pt thumb, `MediaGrid`, `OptionsEditor`, `CategoryField`, or any sheet's behaviour.
- Its sticky footer — Task 8 already migrated it to `StickyActionBar`.

**🔴 `__tests__/product-detail-sections.test.tsx:85` asserts `products/[id].tsx` stays under 500 lines.** It is currently 476. A density pass should not add lines; if it does, extract rather than raise the ceiling.

- [ ] **Step 1: Measure.** Screenshot both screens at `large` and `accessibility-large` (terminate + relaunch between size changes). Write the measured list of deltas into `.superpowers/sdd/progress.md` before changing anything.

- [ ] **Step 2: Write the failing tests** for whatever you measured — a gutter assertion on both screens, `VariantRow`'s 64pt height, the switch row's height, and (if applicable) `FieldInput`'s box.

- [ ] **Step 3: Run, confirm failure.**

- [ ] **Step 4: Implement.**

- [ ] **Step 5: Run, gates.** Confirm `products/[id].tsx` is still under 500 lines.

- [ ] **Step 6: Device gate.**

1. Cleared-cache start.
2. Screenshot the editor on a **single-variant** and a **multi-variant** product, and the create modal. Verify one left edge on each; verify variant rows are 64pt.
3. If `FieldInput` changed: screenshot **all seven** of its call sites.
4. Verify the create modal's sheets (category picker) still portal **above** the modal, not behind it — that is the trap this screen exists to remember.
5. `accessibility-large`, terminate, relaunch, screenshot all of the above again.

- [ ] **Step 7: Commit.**

```bash
git commit -m "refactor(mobile-admin): bring product editor and create onto the native density and type scale"
```

---

### Task 11: Rollout sweep and final gates

The increment's own guard. Every previous task could pass individually while the rollout as a whole drifts — one screen that quietly omits the invariant test, one screen still on `PageHeader`, one `EmptyState` re-centred by a wrapper.

**Files:** Create `__tests__/rollout-invariants.test.ts`. Modify whatever it catches.

- [ ] **Step 1: Write the meta-test.**

```ts
// __tests__/rollout-invariants.test.ts
// Corpus-based, in the shape of the existing banned-colour and
// no-touchable-opacity guards. Each assertion must be proven to bite by
// temporarily breaking one screen and watching it go red.

const SWIPE_SCREENS = [
  "app/(tabs)/orders/index.tsx",
  "app/(tabs)/index.tsx",
  "app/(tabs)/products/index.tsx",
  "app/(tabs)/customers/reviews/index.tsx",
  "app/(tabs)/more/settings/tickets/index.tsx",
  "app/(tabs)/more/marketing/coupons/index.tsx",
];

const COLLAPSING_SCREENS = [
  ...SWIPE_SCREENS,
  "app/(tabs)/customers/index.tsx",
  "app/(tabs)/more/marketing/gift-cards/index.tsx",
  "app/(tabs)/more/marketing/campaigns/index.tsx",
  "app/(tabs)/more/marketing/segments/index.tsx",
];

it("every screen that mounts a SwipeRow has a suite asserting the swipe convention", () => {
  // for each SWIPE_SCREENS entry, find its __tests__ suite and assert it
  // imports assertSwipeConvention from test-utils/swipe-convention
});

it("every rolled-out list screen uses CollapsingHeader, not PageHeader or BackHeader", () => {
  // grep each COLLAPSING_SCREENS file
});

it("no rolled-out screen wraps an EmptyState in a re-centring container", () => {
  // the styles.centered trap: align="left" is silently defeated by a
  // shrink-wrapping centred parent. Assert each uses errorSlot: { flex: 1 }.
});

it("no action anywhere opts into full-swipe auto-fire", () => {
  // grep the whole app corpus for autoFireOnFullSwipe: true
});

it("no screen added an optimistic hide", () => {
  // every list in this increment refetches itself; a hide here would be the
  // Dashboard's four-round bug re-imported into a screen that never needed it
});
```

- [ ] **Step 2: Run it. Fix whatever it catches.** Prove each assertion bites by breaking one screen, running, and restoring.

- [ ] **Step 3: Full gates.**

```bash
cd apps/mobile-admin && npm test && npm run check-types
cd ../../packages/mobile-shared && npx vitest run && npm run check-types
cd ../.. && git diff --stat package.json package-lock.json   # must be empty
git log --format='%B' <base>..HEAD | grep -E 'Co-Authored|Signed-off' # must be empty
git log --format='%s%n%b' <base>..HEAD                                # single lines only
```

- [ ] **Step 4: Whole-increment device pass, on ONE cleared bundle.**

`npx expo start --dev-client --clear --port 8082`, then walk every rolled-out screen in one session and screenshot each **expanded and collapsed**: Dashboard, Orders, Order detail, Products, Product editor, Product create, Customers, Reviews, Tickets, Coupons, Gift cards, Campaigns, Segments, More, Account, Security, Notification settings, Team.

Check on each: one left edge · 88pt rows · exactly one moss accent (and none on a screen whose only moss was a swipe that is not currently revealed) · badges are tints · no clipping · the dock is clear of content.

- [ ] **Step 5: Repeat the whole walk at `accessibility-large`.** `xcrun simctl ui booted content_size accessibility-large`, **terminate, relaunch**, walk again. Then restore to `large`, terminate, relaunch, and confirm.

- [ ] **Step 6: Update `.superpowers/sdd/progress.md`** with the increment's outcome, every defect the device gate caught, and anything carried forward.

- [ ] **Step 7: Commit.**

```bash
git commit -m "test(mobile-admin): add rollout invariants guarding the increment 3 pattern"
```

---

## Out of scope

**Cut because the backend does not support it** (each detailed in the cut table above): review report · ticket assign · gift card enable/disable/delete · coupon delete · coupon/campaign/segment duplicate · product duplicate · product delete from mobile.

**Cut as feature work, not pattern rollout:** §3's "Edit price · Adjust stock" items on the Products long-press menu. Both need variant selection (a product has N variants and the endpoint is per-variant) plus a numeric-entry sheet that does not exist. That is a new capability, and §3's own framing for this increment is "collapsing header, deeper rows, native press feedback, and — where there is an obvious primary action — a swipe". The Edit item navigates to the editor, where both fields already live. **Recommend a follow-up task if merchants ask for it.**

**Deliberately deferred (pre-existing, recorded, not introduced here):**
- The dead `user_name` context key in `marketplace-api` — review replies persist the literal "Store Owner" and **ticket replies persist raw email addresses as the author name, which customers see**. Real production-data bug, its own task.
- `product_media.position` / top-products subselect tiebreak asymmetry.
- `order-actions.ts`'s "do not `return` the invalidation" invariant, which is load-bearing across files and has no test.
- The `has_first_order` schema drift.
- `FieldInput`'s `aria`/label parity with `DangerZone`.
- `RevenueChart`'s all-zero series rendering as a half-height block, and its missing paint-order regression guard.
