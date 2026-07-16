# Design Audit — Slice 1: Dashboard + Catalog

Scope: `app/(tabs)/index.tsx`, `app/(tabs)/products/index.tsx`, `components/ProductRow.tsx`, `components/DashboardStats.tsx`, measured against `lib/theme.ts` and the Paper·Ink·Moss editorial system. Read-only audit — nothing fixed.

---

## app/(tabs)/index.tsx

**[P0]** app/(tabs)/index.tsx:68,78,98 — `theme.colors.textTertiary` (`rgba(14,14,12,0.5)`, defined `lib/theme.ts:68`) is used for the ListRow secondary line, the chevron icon, and every Section eyebrow title. Blended over the Paper background (`#F7F6F2`) this resolves to roughly a mid-gray with a contrast ratio of ~3.6:1 against the ~4.5:1 required for 11–12pt text (caption is 12px, eyebrow is 11px — neither qualifies as "large text"). This is a systemic AA text-contrast failure everywhere `textTertiary` is used for actual copy (as opposed to genuinely decorative/disabled elements). — Fix: darken the token for text usage, e.g. add a `textTertiaryText` (or raise `textTertiary` itself) to ≥`rgba(14,14,12,0.62)` and re-check against `#F7F6F2`; keep a separate lighter value for non-text decorative strokes if needed.

**[P2]** app/(tabs)/index.tsx:114 — `Section` wraps each list group (Recent Orders / Low Stock / Top Products) in `<Card padding={0}>`, which defaults to `variant="outline"` — a full hairline border box (`Card.tsx:39-41`) around each section rather than a plain surface separated by hairline rules between sections, per the "hairline rules between sections, not bordered cards" principle. Reads as three stacked bordered cards down the page instead of one continuous editorial list. — Fix: use `<Card variant="ghost" padding={0}>` (or a plain `View` on `theme.colors.background`) and rely on `Hairline` only between/around sections, not a 4-sided border.

**[P3]** app/(tabs)/index.tsx:145,147 — `EmptyState` (loading and error state) centers its title/message (`EmptyState.tsx:16,20` use `align="center"`), which is a conventional pattern for empty/loading states but technically conflicts with the brand's "asymmetric, never centered" rule. Low-priority given it's a non-hero utility state.

**[P3]** app/(tabs)/index.tsx:136-140 — `PageHeader`'s serif title (the screen's true heading) has no `accessibilityRole="header"`, so screen-reader users can't jump directly to it via heading navigation. — Fix: add `accessibilityRole="header"` to the title `Text` in `PageHeader.tsx`.

---

## components/DashboardStats.tsx

**[P0]** components/DashboardStats.tsx:32,60 — Same `textTertiary` contrast failure (~3.6:1) applied to every stat-card eyebrow label ("Today", "This week", "This month", "Customers") and every compact-stat label ("Today", "Pending", "Fulfilled", "Cancelled") — the labels for literally every number on the dashboard are sub-AA contrast. — Fix: same as above — use a darkened text-safe tertiary token.

**[P1]** components/DashboardStats.tsx:41,45 — The revenue trend indicator (`ArrowUpRight` + percentage) uses `theme.colors.accent` (moss) for the positive case. This is a second live moss element at rest on the same screen as the "See all" link in `app/(tabs)/index.tsx:108` — a one-accent-per-view breach (moss is meant to be spent once, on the single primary action/link, not also doubled as a decorative positive-delta indicator). — Fix: render the positive trend in `theme.colors.text` (ink) or `textSecondary`, reserving moss for the one intentional accent moment on a given screen (or vice-versa: drop the "See all" moss and keep the trend, but not both).

**[P1]** components/DashboardStats.tsx:70-117 (styles `row`:95, `card`:96, `compactStrip`:104-111) — The stats block is two rows of equal-`flex:1` cards plus a fully symmetric 4-column compact strip, all centered internally (`compactCard` uses `alignItems:"center"`). This is exactly the "same-sized card grid" + centered pattern the brand explicitly calls an AI-slop tell, and it dominates the top of the dashboard. — Fix: break the grid — vary card widths/weights (e.g. make "This month" the wide/hero stat with the others secondary), left-align stat values instead of centering the compact strip, and vary vertical rhythm between the two groups instead of two identical rows.

**[P2]** components/DashboardStats.tsx:31,104-111 — `StatCard`'s `Card` (outline variant, hairline border on all 4 sides) plus the explicitly bordered `compactStrip` (`borderWidth: theme.hairline` at line 107) stack three-plus bordered boxes in one screenful — reads closer to a dense enterprise-dashboard stat grid than the editorial hairline system. — Fix: drop the border on `compactStrip` and use `Card variant="ghost"` for stat cards, relying on whitespace/hairline dividers instead of boxed cards.

**[P2]** components/DashboardStats.tsx:96 — `minHeight: 92` is a hardcoded magic number not present anywhere in `theme.spacing` (4/8/12/16/20/24/32/48). — Fix: derive from spacing tokens (e.g. `theme.spacing.xxxl * 2 + something` or add an explicit `theme.spacing` step) rather than a bespoke literal.

**[P3]** components/DashboardStats.tsx:30-51 — Each `StatCard`/`CompactStat` is three separate `Text` nodes with no `accessible`/composed `accessibilityLabel`, so VoiceOver/TalkBack reads label, value, and trend as three disconnected announcements instead of one coherent phrase ("Today, $482, up 4.2 percent"). — Fix: wrap each card in `accessible accessibilityLabel="…"` combining label + value + trend.

---

## app/(tabs)/products/index.tsx

**[P0]** app/(tabs)/products/index.tsx:74-85 (component sources: `SegmentedControl.tsx:39`, `SearchField.tsx:26`) — Both the inactive segmented-control tab label and the search placeholder use `textTertiary`, inheriting the same ~3.6:1 contrast failure — on this screen it affects the filter tabs (All/Active/Draft) and the search box, both primary interaction affordances. — Fix: same token fix as above; re-verify inactive tab and placeholder legibility once corrected.

**[P1]** app/(tabs)/products/index.tsx:81-85 (`SegmentedControl.tsx:62`) — `SegmentedControl` tabs have `minHeight: 36`, below the 44pt minimum touch target used everywhere else in this app (`theme.touchTarget = 44`). The filter row (All/Active/Draft) is a frequently-tapped control on this exact screen. — Fix: raise `tab` to `minHeight: theme.touchTarget` (44) in `SegmentedControl.tsx:62`.

**[P1]** app/(tabs)/products/index.tsx:74-79 (`SearchField.tsx:59`) — `SearchField`'s wrapper is a fixed `height: 40`, also below the 44pt minimum touch target, on the screen's primary search entry point. — Fix: raise to `height: theme.touchTarget` (44) in `SearchField.tsx:59`.

**[P2]** app/(tabs)/products/index.tsx:157,161 — The FAB's shadow uses a hardcoded `shadowColor: "#000"` and a bare `elevation: 6`, neither sourced from `theme.ts` (which has no shadow/elevation scale at all). — Fix: introduce a shadow token in `theme.ts` (or reuse `theme.colors.text`/ink as the shadow tint) rather than a raw hex, and give elevation a named constant.

**[P2]** app/(tabs)/products/index.tsx:88-90 — The loading `ActivityIndicator` has no `accessibilityLabel`/live-region announcement, so screen-reader users get silence while the catalog loads — inconsistent with the dashboard's own textual "Loading…" `EmptyState` pattern (`app/(tabs)/index.tsx:145`). — Fix: wrap in a `View` with `accessibilityLiveRegion="polite"` and `accessibilityLabel="Loading products"`, or reuse the same `EmptyState` loading pattern used on the dashboard for consistency.

**[P3]** app/(tabs)/products/index.tsx:137 — `paddingBottom: 96` in `styles.list` is a hardcoded value not derived from `theme.spacing` (closest token, `huge`, is 48). — Fix: express as `theme.spacing.huge * 2` or add the value to the spacing scale if it's a recurring need.

---

## components/ProductRow.tsx

**[P0]** components/ProductRow.tsx:43,55-57 — `textTertiary` is used for the placeholder-thumbnail icon color and for the "N in stock" caption whenever stock isn't low — the same ~3.6:1 contrast failure, here affecting the stock count on the majority of rows in the catalog list (only low-stock rows get the higher-contrast danger color). — Fix: same token fix as above.

**[P1]** components/ProductRow.tsx:61-64 — `StatusBadge` is rendered with `tone="success"` (solid `theme.colors.accent` / moss fill) for every `active` product. This is a decorative, at-rest use of the single moss accent as a routine status label rather than for a link/focus/primary CTA — and because it repeats once per active row, a catalog screen with several active products shows moss in half a dozen places simultaneously, alongside the screen's own moss FAB (`app/(tabs)/products/index.tsx:154`). This is the most severe one-accent breach in the slice. — Fix: give "Active" a neutral/ink or outline treatment (e.g. `tone="muted"` with an ink dot, or a new non-moss "active" tone) and reserve moss exclusively for the FAB (this screen's one legitimate primary-action accent).

**[P2]** components/ProductRow.tsx:33,39 — The row's `TouchableOpacity` already carries a full composed `accessibilityLabel` (title, price, stock, status), but the inner `Image` also sets its own `accessibilityLabel="{title} thumbnail"` — nested accessible elements can cause VoiceOver to double-announce the thumbnail. — Fix: drop the `Image`'s own label (or mark it `accessibilityElementsHidden`/`importantForAccessibility="no"`) since the parent touchable already speaks for the whole row.

**[P3]** components/ProductRow.tsx:52-57 — Price and stock count — arguably the row's key numerals — are rendered only in the sans `caption` preset, never serif, even though the brand's numerals guidance calls out stat/price numerals as a place the serif should carry the brand. Likely a deliberate density trade-off for a dense list, but worth a conscious call rather than default.

---

**Counts:** P0: 4 · P1: 5 · P2: 6 · P3: 5
