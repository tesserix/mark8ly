# Slice 4 — Shared Chrome + UI Primitives Design Audit

Scope: `components/navigation/Dock.tsx`, `components/StorePicker.tsx`, `components/StoreSelector.tsx`,
`components/TenantGate.tsx`, `components/ErrorBoundary.tsx`, `components/ui/*`, `lib/theme.ts`.
Read-only audit against Paper·Ink·Moss editorial-luxury system. Nothing fixed.

## Root cause behind most findings below

This app runs **two disconnected token systems** at once:

1. `lib/theme.ts` — a hand-authored JS object consumed via `theme.colors.*` / `theme.spacing.*` /
   `theme.radii.*` by every `StyleSheet.create` primitive (Dock, Card, StatusBadge, SearchField,
   SegmentedControl, EmptyState, Eyebrow, Hairline, FieldInput, StorePicker, StoreSelector, TenantGate).
2. `tailwind.config.js` (backed by `packages/ui/src/styles/mark8ly-tokens.css`, the stated canonical
   source) — consumed via NativeWind class names, and it's the **only** thing `components/ui/Text.tsx`
   actually reads for color and type scale.

The two were hand-copied once and have since drifted. `theme.ts`'s values differ from the canonical
CSS token file in several places (see P1 #1 below), and `theme.ts`'s entire `text` typographic scale
(`theme.text.display/h1/h2/…`, lines 111–176) is dead code — no component reads it; `Text.tsx` gets
font size/line-height/tracking from `tailwind.config.js` instead. Any new primitive built against
`theme.ts` values will silently diverge from what `Text.tsx`-based components render.

---

## Findings

**[P0]** `components/ui/Text.tsx:49-59` (`COLOR_CLASSES.textTertiary → 'text-ink-muted'`, resolving to
`tailwind.config.js:27` `ink.muted = #7A766E`) — this is the color actually rendered wherever a
component passes `color="textTertiary"` to `<Text>`. Contrast against the Paper background
(`#F7F6F2`) computes to ≈4.1:1 — under the 4.5:1 WCAG 2.1 AA baseline for normal text (and this is
*not* large/bold text at the sizes used). It repeats as real content text in `EmptyState.tsx:20`
(error/empty message copy), `Eyebrow.tsx:15` (every section header app-wide), `PageHeader.tsx:27`
(every screen's eyebrow), `BackHeader.tsx:33`, and `StorePicker.tsx:49` (store slug caption). It also
shows up again via a raw NativeWind class in `ErrorBoundary.tsx:32` (`text-ink-muted` on the crash
screen's "Restart the app to continue" message) — the one screen where legibility matters most.
Separately, `theme.ts:69` defines `textTertiary` as `rgba(14,14,12,0.5)`, a *different* value
(≈3.5:1 on Paper) used directly by icon-only consumers (Dock, SearchField) — icons only need 3:1 so
those pass, but neither definition clears 4.5:1 for text. — Fix: pick one canonical "tertiary" text
color that hits ≥4.5:1 on `#F7F6F2` (e.g. promote real content text to `ink.soft`/`textSecondary`,
~9:1, and reserve the muted tone for icons/decoration only, never for `<Text>` body/caption content).

**[P1]** `lib/theme.ts:44-55` — the hand-authored palette has drifted from the canonical
`packages/ui/src/styles/mark8ly-tokens.css` values that `tailwind.config.js` correctly mirrors:
`inkSoft #3A3A36` vs canonical `--ink-600 #45433E`; `mossSoft #3F6A3D` vs canonical `--moss-600
#3D5F38`; `crimson #8B2020` vs canonical `--danger #8B2E20`; `amber #B08A30` vs canonical `--warning
#B5751F` (a visibly different, more gold/tan hue than the specified "amber-bronze"); `bone #E5E4DF`
vs canonical `--ink-100 #E2DFD6`; `paperWarm #FAFAF6` vs canonical `--paper-100 #FAF8F2`. Every
StyleSheet-based primitive in this slice (Dock, Card, StatusBadge, SearchField, FieldInput, etc.)
renders these off-canonical values. — Fix: delete the duplicated hex literals in `theme.ts` and
generate/import them from the same source `tailwind.config.js` reads (or a shared JSON emitted once
from `mark8ly-tokens.css`), so there is exactly one palette.

**[P1]** `lib/theme.ts:93-100` vs `tailwind.config.js:76-83` — `theme.radii.md` = `6` but
NativeWind's `rounded-md` = `10px` (NativeWind's unsuffixed `rounded`/`DEFAULT` = `6px`, i.e. what
`theme.radii.md` actually means). Same token name, two different pixel values depending on which
half of the app a component is styled through — a landmine for anyone matching corner radii between
a `StyleSheet` primitive and a NativeWind-styled sibling. — Fix: rename one side so `md` means the
same radius in both systems, or generate both from one map.

**[P1]** `components/ui/StatusBadge.tsx:22` — `success` tone is a **solid Moss fill**
(`bg: theme.colors.accent`). Moss is the app's single accent, meant for links/focus/primary-hover;
flooding a badge background with it on every "success"/"active"/"paid"/"in stock" row invites the
one accent to become a decorative fill repeated down long lists (products, orders). The canonical
token file already defines `--moss-100` / `--accent-tint` (`mark8ly-tokens.css:61,89`) specifically
so success/accent states can read as a soft tint + moss text instead of a solid block. — Fix: use
`accent-tint` background with moss/ink text for `success`, save the solid moss fill for a true
primary-CTA-adjacent moment, not a routine list-row status chip.

**[P1]** `components/ui/SegmentedControl.tsx:59-65` — tab touch target is `paddingVertical: 8` +
`minHeight: 36`, below the 44pt minimum this system otherwise enforces (e.g. `FieldInput.tsx:40`
correctly uses `minHeight: 44`). This is a filter control likely reused on every list screen —
sub-44pt here repeats on every screen that filters. Also `accessibilityRole="button"` combined with
`accessibilityState={{ selected: active }}` (lines 32-34) is a semantic mismatch — `selected` isn't a
meaningful state on the `button` role for assistive tech; this should be `tab` (with a `tablist`
wrapper) or `radio`/`radiogroup`, matching the toggle-group pattern the rest of the system uses. —
Fix: bump `tab` to `minHeight: 44`, switch role to `tab`/`radio` with a matching group role on `row`.

**[P1]** `components/ui/SearchField.tsx:38-43,59` — the clear button's effective hit area is
`16px icon + 12px hitSlop×2 = 40×40`, and the field row itself is `height: 40` (`styles.wrapper`) —
both under the 44pt baseline used elsewhere in this same file set (`BackHeader.tsx:59-60` correctly
sizes its back button to 44×44). — Fix: raise `hitSlop` to 14 (or shrink padding) to clear 44×44, and
raise the row height to 44 to match the rest of the system's touch rhythm.

**[P1]** `lib/theme.ts` — no shadow/elevation token exists anywhere in the theme (`spacing`, `radii`,
`text` are all defined; there is no `shadow`/`elevation` object). `Dock.tsx:157-161` hardcodes
`shadowColor: "#000000"`, `shadowOpacity: 0.1`, `shadowRadius: 16`, `elevation: 10` inline instead of
referencing a token — the design bar calls for a "single-elevation shadow scale," but there's nowhere
in the token layer that captures what that single elevation *is*, so the next elevated surface will
either hand-roll its own (drifting) shadow or copy-paste this block. — Fix: add `theme.shadow.raised`
(or similar) once in `theme.ts` and have `Dock.tsx` reference it.

**[P1]** `lib/theme.ts:111-176` (`theme.text`) is unread dead code — no component in the app
references `theme.text.*` (confirmed via grep); the actual type scale rendered by
`components/ui/Text.tsx` comes entirely from `tailwind.config.js:65-74`'s `fontSize` scale via
NativeWind classes. The values happen to still match today (both scales list `display: 36/42/-0.5`,
etc.) but there is no mechanism keeping them in sync — a future edit to one will silently stop
applying anywhere. — Fix: delete `theme.text` (or make `Text.tsx` actually consume it) so there's one
type-scale source, matching the one-palette fix above.

**[P2]** `components/StoreSelector.tsx:65` — the store-switcher sheet uses React Native's built-in
`animationType="slide"` (OS default easing/duration) while `Dock.tsx:33` establishes a specific
app-standard ease-out-quart/220ms as "the motion reference." The two chrome-level transient surfaces
in this slice (Dock's active-tab pill, and this modal) now use two unrelated motion languages. — Fix:
drive the sheet's presentation with the same Reanimated easing/duration used in `Dock.tsx`, or at
minimum document that native modal transitions are an intentional exception.

**[P2]** `components/StoreSelector.tsx:145` (`storeRow` `minHeight: 56`) vs
`components/StorePicker.tsx:94` (`row` `minHeight: 64`) — both are "pick your store" list rows for
the same underlying data (`Store`), rendered a few taps apart in the same flow, at two different
row heights. — Fix: share one row height (or extract one `StoreRow` primitive) between the two.

**[P2]** `components/TenantGate.tsx:123` and `components/StoreSelector.tsx:85` — bare
`<ActivityIndicator>` with no `accessibilityLabel`. VoiceOver/TalkBack users get no announcement that
content is loading during tenant resolution or the store-switch fetch. — Fix: add
`accessibilityLabel="Loading"` (or more specific copy) to both.

**[P2]** `components/ErrorBoundary.tsx:26-39` — the crash screen has no recovery action (no retry/
reload button, no `accessibilityRole="alert"` / live region to announce the failure to screen reader
users) — just static text asking the user to restart the app manually. For the one screen users see
when something is actually broken, this is a weak trust moment relative to the "confident, calm"
brand direction. — Fix: at minimum mark the container as an accessibility live region announcing the
failure; consider a "Try again" action that resets boundary state.

**[P3]** `components/navigation/Dock.tsx:5` vs `:153` — the file header comment claims "solid Paper
surface" but `styles.bar.backgroundColor` is `theme.colors.elevated` (white), not
`theme.colors.background` (Paper). The implementation is arguably correct per the system's own rule
("white reserved for cards/popovers," and a floating shadowed dock reads as a popover) — but the
comment is stale/misleading against the code. — Fix: update the comment to say "Elevated" instead of
"Paper."

**[P3]** `components/ui/Card.tsx:44` — `ghost` variant sets `backgroundColor: "transparent"` as a bare
string literal rather than a named token/constant. Harmless today, but every other color in this file
is a token reference. — Fix: define `theme.colors.transparent` or similar for consistency, or leave a
comment noting it's intentionally not a palette color.

**[P3]** `components/ui/EmptyState.tsx:38` — icon wrapper uses a hardcoded `opacity: 0.5` rather than
a token. — Fix: pull from a shared "muted icon" opacity constant if one exists, or add one.

---

## Counts

- P0: 1
- P1: 7
- P2: 4
- P3: 3
