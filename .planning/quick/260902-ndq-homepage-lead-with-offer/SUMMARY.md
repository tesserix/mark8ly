---
type: quick
slug: homepage-lead-with-offer
branch: feat/homepage-lead-with-offer
commit: 2a59230c
files-modified:
  - apps/onboarding/app/page.tsx
completed: 2026-09-02
---

# Quick Summary: Homepage leads with the ninety-day offer

The ninety-day free trial and the 0% platform fee now lead the homepage and
its SERP entry instead of sitting as tertiary fine print below the CTAs. The
serif `<h1>` is untouched; a sans deck line beneath it carries the offer, and
the fine print below the CTAs keeps price detail only, so the offer is stated
exactly once.

## What changed

**`apps/onboarding/app/page.tsx` — `Hero()`**

New deck line between the `<h1>` and the descriptive paragraph:

```
mt-6 max-w-xl text-xl leading-[1.2] text-foreground
"Ninety days free, no card. And we never take a cut of what you sell."
```

Every class was already in use in this file (`text-xl` at :972, `leading-[1.2]`
at :972, `mt-6`, `text-foreground`) — no new colours, sizes, or spacing values.
Sans rather than serif: `.impeccable.md` reserves Source Serif 4 for display,
and a serif deck under a serif headline would compete rather than support. The
descriptive paragraph moved `mt-8` → `mt-6` so the headline/deck/body read as
one block, with the CTAs' `mt-10` still opening the next beat.

The Hero header comment claimed "Headline carries the offer" — the drift the
task named. Rewritten to describe what the code now does.

**Copy, before → after**

| Slot | Before | After |
| --- | --- | --- |
| Deck (new) | — | Ninety days free, no card. And we never take a cut of what you sell. |
| Below CTAs | Free for ninety days. No card required. Three clear plans after that, from $15 a month, billed yearly. | Three clear plans after that, from $15 a month, billed yearly. |
| `title.absolute` | Mark8ly — quiet commerce for people who make things (51 ch) | Ninety days free, 0% platform fees — Mark8ly commerce (53 ch) |
| `description` | Mark8ly is an editorial commerce platform for independent merchants. Open a storefront in an afternoon, keep every sale — no platform transaction fees — and ship a store that looks considered from day one. Ninety days free, no card required. (238 ch) | Ninety days free, no card, and 0% platform fees on your sales. An editorial commerce platform for independent merchants — open a store in an afternoon. (151 ch) |

Title and description lengths were measured with `node -e`, not estimated: 53
and 151, inside the ~60 / ~155 render budgets. The brand name stays in the
title, so the description spends its budget on the offer and the category
rather than repeating "Mark8ly". "0% **platform** fees" rather than a bare
"0% fees" — the merchant still pays their payment processor's standard rate,
and the unqualified phrasing would be a claim the repo does not support.

## Claims audit

Every claim was already committed elsewhere in the repo before this change:

- ninety days free, no card — `app/terms/page.tsx:87`, `app/refunds/page.tsx:21`
- no platform cut on sales — homepage FAQ (`page.tsx:1128`, "No added
  transaction fees from Mark8ly, ever") and `page.tsx:412`
- $15 a month billed yearly — existing USD annual equivalent, unchanged

No new claims invented. No migration/importer claim made anywhere.

## Not changed (deliberately)

- **`lib/seo/site-json-ld.ts`** — left alone. Editing `SITE_DESCRIPTION` is
  safe (the CSP hash is computed from `SITE_JSON_LD` at module load in
  `middleware.ts:31`, so it follows the content), but that string is the
  *site-wide* description consumed by the Organization and WebSite graphs on
  every route, not the homepage description. Rewriting it to lead with a
  trial offer would push page-specific marketing copy into site-wide
  structured data. No consistency gain worth that.
- **The `<h1>`** — byte-identical, per the task and to keep the existing e2e
  assertion `getByRole("heading", { name: /a storefront worth opening/i })`
  in `tests/e2e/golden-path.spec.ts:25` passing.
- **`app/opengraph-image.tsx:14`** — its `alt` still reads "Mark8ly — quiet
  commerce for people who make things" because it describes the OG image,
  which visually renders that tagline. Changing the alt without regenerating
  the image would make the alt wrong.

## Verification

| Command | Result |
| --- | --- |
| `npm run check-types -w @mark8ly/onboarding` (`tsc --noEmit`) | Pass, no output |
| `npm run lint -w @mark8ly/onboarding` | Runs, but it is a **stub** — the script body is `echo 'lint stub — migrate to ESLint flat config (next lint removed in Next 16)'`. It lints nothing. |
| `node -e` character counts on title/description | 53 and 151 |
| `git diff --diff-filter=D HEAD~1 HEAD` | No deletions |

No `npm install` was run and none was needed.

Not run: `next build` and the Playwright e2e suite. The e2e specs need a dev
server, and the only homepage assertion is on the unchanged `<h1>`. The change
is JSX text and two string literals in an already-typechecking file.

## Self-Check: PASSED

- `apps/onboarding/app/page.tsx` — exists, modified
- commit `2a59230c` — present in `git log`
