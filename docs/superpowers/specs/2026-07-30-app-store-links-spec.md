# Mobile-admin store links on web surfaces — spec

**Status: BLOCKED ON iOS APPROVAL — deliberately not implemented yet (user decision
2026-07-30).** Nothing ships until App Store 1.0.0 is approved, then all three
surfaces ship together with both badges.

## Why it is blocked, with evidence

| Platform | Linkable today? | Evidence |
|---|---|---|
| Google Play | ✅ live | `HTTP 200` on `https://play.google.com/store/apps/details?id=com.mark8ly.admin` |
| App Store | ❌ does not exist | `itunes.apple.com/lookup?bundleId=com.mark8ly.admin` → `resultCount: 0` |

iOS 1.0.0 / buildNumber 10 is an **initial** release still in review.
`apps.apple.com` product URLs do not resolve until first release, so an App
Store badge shipped now would be a dead link on the highest-intent page in the
funnel. The user chose to hold rather than ship Android-only.

### Getting the App Store URL at approval time

Do not hand-copy it out of App Store Connect. Once the app is live:

```bash
curl -s "https://itunes.apple.com/lookup?bundleId=com.mark8ly.admin" \
  | python3 -c "import json,sys; r=json.load(sys.stdin)['results'][0]; print(r['trackId'], r['trackViewUrl'])"
```

`resultCount: 0` still means **not live yet** — that is the go/no-go signal for
this whole piece of work. Use the canonical
`https://apps.apple.com/app/apple-store/id<trackId>` form, not a
locale-prefixed URL, so it resolves for merchants in every market.

## Decisions locked 2026-07-30

1. **Sequencing** — wait for iOS approval; ship both badges together.
2. **Treatment** — official Apple and Google badge artwork (not a restrained
   text row, not QR).
3. **Surfaces** — all three: onboarding `/welcome`, a persistent quiet entry in
   the admin, and a dismissible dashboard prompt.

## Surfaces and exact targets

| # | Surface | File | Treatment |
|---|---|---|---|
| 1 | Onboarding success | `apps/onboarding/app/welcome/page.tsx` | Hairline-separated block, badges. Highest intent — merchant just created a store and is signed in. |
| 2 | Admin persistent entry | `apps/admin/app/(admin)/settings/page.tsx` | Quiet, permanent, findable. No dismissal state. |
| 3 | Admin dashboard prompt | `apps/admin/app/(admin)/dashboard/page.tsx` | One-time, dismissible, must never become a nag. |

Surface 1 already ends in a hairline-ruled `<dl>` grid inside
`PostSubmitShell`, so an install block drops into the existing editorial rhythm
without inventing a new layout idiom.

**Storefront is deliberately excluded.** The mobile app is the *merchant admin*
app; the storefront audience is customers, for whom the link is noise.

## Badge artwork — the part most likely to be got wrong

Both companies license their badges under brand guidelines that constrain use.
The rules that matter here:

- Use the **official artwork**, downloaded from Apple's Marketing Resources and
  Google's Play badge generator. Do not redraw, recolour, or rebuild the badges
  as CSS/SVG by hand.
- **Self-host** the assets. Do not hotlink from Apple or Google.
- Do not modify, rotate, crop, add effects to, or change the proportions of a
  badge.
- Maintain the required **clear space** around each badge, and respect each
  badge's **minimum size**.
- When both appear together they must be **visually equal weight** — the same
  height, aligned on a shared baseline.
- Each badge must link to its own product page and nothing else.

🔴 **Verify the exact clear-space and minimum-size figures against the current
published guidelines when implementing.** Both documents change, and the
numbers should be read off the source rather than carried in from memory or
from this spec.

### The design tension, stated honestly

The badges are loud, branded, and full-colour — they work against the
`.impeccable.md` direction ("calm, premium, refined", "one accent per view",
"if it feels loud, it's wrong"). This was chosen with that trade-off understood,
for recognisability and conversion. Mitigations that do not violate either
company's guidelines:

- give the block generous surrounding whitespace and a hairline rule rather than
  a bordered card, so it reads as a composed section rather than an ad;
- keep the surrounding copy in the editorial voice (Source Serif 4 heading, no
  urgency language, no exclamation marks);
- do not add a moss accent anywhere in the block — the badges are already
  carrying the visual weight for that view.

## Implementation shape

**One source of truth for the URLs**, shared by all three surfaces — do not
inline the URLs three times:

```ts
export const MOBILE_ADMIN_APP_LINKS = {
  ios: "https://apps.apple.com/app/apple-store/id<trackId>",     // fill on approval
  android: "https://play.google.com/store/apps/details?id=com.mark8ly.admin",
} as const;
```

Render a badge only when its URL is non-empty. That keeps the "wait for iOS"
decision reversible at zero cost and means a future platform change is a
one-line edit rather than a hunt across three apps. `packages/ui` is the right
home if both apps consume it, per the Path C component strategy — app-local
until a second app needs it, then promote.

**Dismissal (surface 3 only)** needs persistence. Prefer a per-user server-side
preference if one already exists; otherwise `localStorage` keyed per tenant, so
dismissing on one store does not silently dismiss on another. Whichever is
chosen, the prompt must not reappear after dismissal — that is the whole point
of the affordance.

## Accessibility

- Badges are links containing images: each needs meaningful alt text
  ("Download Mark8ly Admin on the App Store"), not "App Store badge" and not
  empty alt.
- The dismiss control needs an accessible label and must be keyboard-reachable,
  with a visible moss focus ring.
- Do not rely on colour alone to convey anything in this block.
- WCAG 2.1 AA on the surrounding copy, per the project baseline.

## Test plan

- Unit: badge renders only when its URL is set; both render when both are set;
  neither renders when the config is empty. Prove the gating red by clearing a
  URL — a test that passes with the URLs hardcoded is not testing the gate.
- Unit: dismissal persists across remount and is scoped per tenant.
- A11y: alt text present and non-generic; dismiss control labelled and
  focusable.
- Manual, at approval time: both URLs resolve to the correct product page
  (open them, do not assume), on desktop and mobile.

## Do not

- Do not ship the App Store badge before `itunes.apple.com/lookup` returns a
  result — that is the entire reason this is blocked.
- Do not add a QR dependency. `qrcode.react` is **not** currently a dependency
  of `apps/onboarding` or `apps/admin`, and the root lockfile cannot be
  regenerated locally (a plain `npm install` collapses the deliberate multi-
  version tree). QR was considered and not chosen.
- Do not reuse anything from the **Pro app purchase** feature. Every existing
  "App Store"/"Google Play" string in the repo
  (`apps/admin/lib/copy/subscription.ts`, `.../subscription/appCredentials.ts`,
  etc.) is about merchants shipping *their own* white-label apps under their own
  developer accounts. Unrelated, and conflating the two would be confusing.

## Trigger to start

App Store 1.0.0 approved → `itunes.apple.com/lookup?bundleId=com.mark8ly.admin`
returns a `trackId` → fill `MOBILE_ADMIN_APP_LINKS.ios` → implement all three
surfaces → gates → ship.
