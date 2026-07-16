# Design audit — Slice 2: Orders & Customers (mobile-admin)

Scope: `app/(tabs)/orders/index.tsx`, `orders/[id].tsx`, `components/OrderRow.tsx`,
`app/(tabs)/customers/index.tsx`, `customers/[id].tsx`, `components/CustomerRow.tsx`.
Measured against `apps/mobile-admin/lib/theme.ts` tokens and the Paper·Ink·Moss editorial bar.
Read-only — nothing fixed.

---

## P0 — a11y / blocker

**[P0]** `components/ui/SegmentedControl.tsx:62` (`tab` style, `minHeight: 36`) — used directly by
`app/(tabs)/orders/index.tsx:79-83` for the status filter row. 36pt is below the required 44pt
minimum touch target (`theme.touchTarget = 44`), and every one of the 5 filter tabs (All/Pending/
Confirmed/Completed/Cancelled) inherits the undersized hit area. — Fix: change `tab.minHeight` to
`theme.touchTarget` (44) and add matching `paddingVertical` adjustment so label stays vertically centered.

**[P0]** `components/ui/StatusBadge.tsx:23` — the `warning` tone renders `inverse` (near-white,
`#F7F6F2`) text on an `amber` (`#B08A30`) background. Computed contrast ≈ **2.98:1**, below the
4.5:1 AA minimum for normal text (the 11px/600-weight badge label doesn't qualify as "large text").
This tone is live on every "pending" order — `components/OrderRow.tsx` (`STATUS_TONE.pending =
"warning"`) and `app/(tabs)/orders/[id].tsx:35`. — Fix: swap `fg` for `warning` to `theme.colors.text`
(ink) instead of `inverse`, or darken the amber background until inverse text clears 4.5:1.

**[P0]** `theme.colors.textTertiary` (`lib/theme.ts:69`, `rgba(14,14,12,0.5)` on `#F7F6F2` Paper) —
computed contrast ≈ **3.56:1**, below the 4.5:1 AA minimum, yet it's the default color for captions,
eyebrows, and secondary body text across this entire slice. Concrete hits: `OrderRow.tsx:66`
(relative time), `orders/[id].tsx:71` (InfoRow label), `:91` (variant name), `:244` (created date),
`:280-282` ("No shipping address."), `CustomerRow.tsx:50` (email), `:58` (order count),
`customers/[id].tsx:49` (StatTile label), `:136-142` (phone/joined), and `components/ui/EmptyState.tsx:20`
/ `PageHeader.tsx` eyebrow (both rendered on `orders/index.tsx` and `customers/index.tsx`). — Fix:
raise the alpha to ~0.62-0.65 (or define a dedicated AA-safe secondary-ink token) so it clears 4.5:1,
then re-verify `textMuted` (0.35 alpha) isn't used for any real text anywhere it's inherited.

---

## P1 — design-system violation

**[P1]** `components/CustomerRow.tsx:77-84` (avatar `backgroundColor: theme.colors.accent`) and
`app/(tabs)/customers/[id].tsx:196-204` (profile avatar, same) — moss is used as a decorative avatar
fill, repeated on **every row** in the customer list. Directly violates "one moss accent per view,
spent once" / "a second moss element at rest is a violation" — a list of 20 customers is 20 moss
circles. — Fix: use ink-on-paper or ink-on-`surfaceAlt` initials chip (e.g. `background: theme.colors.
surfaceAlt`, `color: theme.colors.text`), reserve moss for the one CTA per screen.

**[P1]** `components/ui/StatusBadge.tsx:22` (`success` tone = solid `theme.colors.accent` fill) —
mapped from `STATUS_TONE.confirmed = "success"` in both `OrderRow.tsx:13` and `orders/[id].tsx:36`.
Every "confirmed" order in the list shows a solid-moss badge, and on the order detail screen for a
confirmed order this badge (`[id].tsx:243`) is on-screen simultaneously with the moss-filled
"Mark Fulfilled" primary button (`[id].tsx:463-468`/`styles.btnPrimary`) — two live moss elements in
one view. — Fix: give status badges a neutral/ink or outline treatment instead of solid moss; reserve
the filled-moss treatment exclusively for the primary CTA.

**[P1]** `app/(tabs)/customers/[id].tsx:119-148` — the entire profile header (avatar, name `align=
"center"`, email `align="center"`, phone `align="center"`, "Joined" `align="center"`, blocked badge
row `alignItems: "center"`) is a centered hero. Directly contradicts "asymmetric, never centered —
centered heroes... are AI slop tells." — Fix: left-align the identity block (e.g. avatar + name/email
as a left-aligned byline row), keep stats/actions below in the existing asymmetric rhythm.

**[P1]** `app/(tabs)/customers/[id].tsx:150-154` / `styles.statsRow`, `styles.statTile:214-223` — three
equal-width, equal-height stat tiles (Orders/Spent/Avg) in a uniform row. The design bar explicitly
calls out "same-sized card grids" as an anti-pattern. — Fix: promote one stat (e.g. Spent, in serif
`h2`/`display`) as the visual lead with the other two as smaller secondary figures, breaking the
uniform grid.

**[P1]** `app/(tabs)/orders/[id].tsx:180-204` (Confirm Order / Cancel Order) and `customers/[id].tsx:
73-83` (Block/Unblock Customer) use native `Alert.alert` for the confirmation step on every mutating
action. This is unstyled OS chrome (system font/colors, no Paper·Ink·Moss) surfacing at the single
most consequential moment (destructive actions) in both flows. — Fix: replace with a themed
confirmation using the existing `InputModal`-style overlay (`Card`/`theme.colors.overlay`,
`theme.colors.danger` for destructive confirm) instead of the OS dialog.

---

## P2 — polish

**[P2]** `components/CustomerRow.tsx:80` (`borderRadius: 20`), `customers/[id].tsx:199`
(`borderRadius: 36` off a hardcoded `width/height: 72`) — avatar circles use ad hoc pixel radii
instead of the token scale. — Fix: use `theme.radii.pill` (999) so the circle is derived, not hand-computed.

**[P2]** `app/(tabs)/orders/[id].tsx:85` / `styles.lineThumb` — every line item renders a bare empty
gray box (`backgroundColor: theme.colors.background`, no image, no glyph, no label) as a permanent
placeholder. Reads as an unfinished element. — Fix: add a neutral product-icon glyph, or drop the box
and let the text carry the row until real thumbnails exist.

**[P2]** `app/(tabs)/orders/[id].tsx:504-515` (`modalInput`) — the tracking-number/refund-amount
`TextInput` has no focus treatment beyond the OS default; the design bar calls for a visible moss
focus ring across the system. — Fix: add `onFocus`/`onBlur` state toggling `borderColor: theme.colors.
accent`.

**[P2]** `app/(tabs)/customers/[id].tsx:91-93` — the "Failed to load customer" error state uses the
serif `h3` preset, which per the design bar is reserved for editorial titles/numerals, not
error/alert copy; the screen also offers no retry action, a dead end for the user. — Fix: use
`bodyEmphasis`/`body` with `color="danger"`, and add a retry button.

**[P2]** `components/ui/SearchField.tsx:36-43` — the clear ("×") button is a 16px icon with `hitSlop:
12`, giving an effective ~40pt hit target, just under the 44pt minimum. Used on both
`orders/index.tsx` and `customers/index.tsx` search bars. — Fix: raise `hitSlop` to 14+ or wrap in an
explicit 44×44 pressable.

**[P2]** Typographic inconsistency for the same "editorial numeral": order total renders as
`bodyEmphasis` (sans) in the list row (`OrderRow.tsx:63-65`) but as `h3` (serif) on the detail screen
(`orders/[id].tsx:263-265`). Likely intentional for list density, but worth a deliberate design call
since the design bar calls out "an order total" specifically as a serif moment.

---

## P3 — nit

**[P3]** `app/(tabs)/orders/[id].tsx:480` (`btnDanger`, `borderWidth: 1`) and `customers/[id].tsx:230`
(`blockBtn`, `borderWidth: 1`) — hardcoded `1` instead of a shared border-width constant; every other
border in these files uses `theme.hairline` (0.5). Inconsistent, not visually broken.

**[P3]** `ActivityIndicator` loading states have no `accessibilityLabel` (`orders/index.tsx:87`,
`customers/index.tsx:65`, `orders/[id].tsx:220-224`, `customers/[id].tsx:103-107`) — screen readers
get a generic "in progress" instead of "Loading orders", "Loading customer", etc.

**[P3]** `InfoRow` (`orders/[id].tsx:67-79`) and `StatTile` (`customers/[id].tsx:43-54`) render label
and value as two separate `Text` nodes rather than one grouped accessible element — VoiceOver/
TalkBack will announce "Name" then "John Doe" as two stops instead of one combined "Name: John Doe."

---

## Counts

- P0: 3
- P1: 4
- P2: 6
- P3: 3
