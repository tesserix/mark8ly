# Next-session prompt — Instagram outreach execution

Paste the contents of this file at the start of a new Claude session
when you're ready to execute the IG outreach playbook. The prompt is
self-contained — fresh agent will pick up cold and have everything
needed to coach you through warm-up comments, Post #1 photo direction,
and the @the_bondi_store grid captions.

---

I'm executing the Instagram outreach strategy for Mark8ly. The infrastructure is all built — I need help running the playbook.

## What Mark8ly is

A "calm, editorial" alternative to Shopify aimed at indie merchants who sell on Instagram without a website. Parent co: Tesserix (Australia). Brand voice: calm, considered, anti-loud-DTC, anti-SaaS-cliché. Target geos: AU first, India second.

## What's already shipped (don't change any of this)

**4 IG accounts reserved + bios set:**

- `@mahesh.sangawar` (founder, ~112 followers, runs cold DMs)
- `@mark8ly` (brand, dormant until first paying merchants)
- `@tesserix.app` (parent co, corporate brochure)
- `@the_bondi_store` (real-merchant persona for peer-DM angle)

**Demo store:** https://the-bondi-store.mark8ly.com

12 on-brand linen/cotton/lifestyle products, all inventory=0 (no orders possible), 24 images (1 product shot + 1 lifestyle per product), full policy footer, Bondi-coast hero with white text overlay via custom CSS. Reads as a real boutique. Use it as proof in outreach DMs.

**Leads system:** Tesserix-home leads page at `/admin/apps/mark8ly/leads` has 259 IG sellers (scraped, "no website" filter). Top 28 are flipped `is_starred=true` (16 AU + 12 IN). Filter UI: starred-only, country, status, followers band, posts band. Source CSV: `~/Downloads/insta-scrape/warmup_top30.csv`

## The playbook (don't reinvent — read first)

`docs/marketing/MARKETING-LAUNCH-TODO.md` (in `~/personal/tesserix-new/mark8ly`) is the 20-step plan. Phases 4-5 are the active outreach work. Steps 15-16 have the DM templates (founder voice + peer voice).

## Three things blocking outreach — all need ME (you coach)

1. **Warm-up comments (3 days, ~30 min/day)** — open the leads admin page, "Starred only" filter, leave one genuine comment per top-28 target on a recent post. NO pitch, NO link. Pre-warms the algorithm before any DM lands. Pace: 10 leads/day × 3 days.

2. **Post #1 on `@mahesh.sangawar` (this weekend, 30 min)** — caption ready in MARKETING-LAUNCH-TODO Step 7. I need help with the photo: composition, lighting, what to capture. I have a phone and a real desk/notebook/window.

3. **`@the_bondi_store` grid (next 2 weeks, trickle)** — 12 Higgsfield-generated lifestyle shots already live at `gs://tesseracthub-480811-mark8ly-media/tenants/8c302556-b647-4824-8ce4-73f547ca456e/products/media/<folder>/<handle>-lifestyle.png`. I need to post them to IG one every 2-3 days. Each post links to its product page on the storefront. Need per-post caption drafts in the same editorial voice as the storefront (Phoebe Philo / The Row register).

After all 3 above: cold DM phase opens (~15-25 DMs/day from each account).

## What I want from you

Help me actually execute. Specifically:

- Walk me through the warm-up: how to pick which post to comment on, what makes a "genuine" comment vs a sales-y one, the exact register
- Coach me through the Post #1 photo: composition options I can shoot in 10 min, lighting, what NOT to do
- Draft 12 captions for the `@the_bondi_store` grid (one per lifestyle shot, each tied to its product, editorial voice, no exclamation marks)
- Be honest as I report reply rates: what's working, what to iterate, when to escalate from comment → DM, when to write someone off

Don't propose rebuilds of the storefront / leads / accounts. Don't suggest new tools, new platforms, new strategies. We've decided the playbook — help me run it.

Start by reading `docs/marketing/MARKETING-LAUNCH-TODO.md`. Then ask me which of the three blockers I want to tackle first.
