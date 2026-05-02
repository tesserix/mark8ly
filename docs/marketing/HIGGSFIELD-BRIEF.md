# Mark8ly product tour video — Higgsfield brief

Drop-in brief for re-cutting the 60-second product tour using Higgsfield (or Runway, Pika, Luma, Sora — same brief works).

---

## Why we're using Higgsfield

The macOS-built v3 holdover (`apps/onboarding/public/video/tour.mp4`) is editorially correct but capped at:
- **Voice**: macOS Flo (en-UK), better than Karen, still detectably synthetic.
- **Motion**: static stills with crossfades. Calm, but no real cinematography.
- **Music**: synthesized A2/E3/A3 sine drone. Subtle but tonally flat.

Higgsfield will lift all three to production quality in one pass.

---

## Brand spec — paste this verbatim into any "system prompt" / brand field

> **Brand**: Mark8ly. Multi-tenant ecommerce platform — "a quiet, considered commerce platform for people who actually make things."
>
> **Tone**: Calm, premium, refined. Editorial in spirit — closer to a thoughtful independent magazine than a dev tool or DTC hype brand. Confident without shouting. Trust, craft, quiet competence — never urgency or hustle.
>
> **Palette**: Paper · Ink · Moss.
> - Paper background `#F7F6F2` (warm-neutral, never pure white)
> - Ink text `#0E0E0C` (near-black, almost no warmth)
> - Moss accent `#2D4A2B` (a single editorial green — Bottega/Phoebe Philo direction)
>
> **Type**: Source Serif 4 for headlines, Source Sans 3 for UI/body. Serif carries the brand.
>
> **Anti-references — explicitly avoid**: generic SaaS aesthetic (Stripe/Linear), loud DTC (Gymshark/Glossier), enterprise dashboards, Web3/AI maximalism (dark mode, glow, glassmorphism), 2018 indie ecommerce (terracotta + sage + cream artisanal). No autoplay urgency, no sparkle particles, no fake-browser chrome.
>
> **Voice (audio)**: warm female en-AU or en-UK, considered pace (~145 wpm), no hype, light breathy quality. Reference: Phoebe Philo's voiceover style or the Bottega holiday films.
>
> **Music**: subtle ambient pad, low frequency drone, no melody, no percussion. Volume: barely-there, frames the silence rather than filling it. Reference: Aphex Twin "Selected Ambient Works II" track 8, or any Stars of the Lid.

---

## Voiceover script (155 words, ~50–55s at 145 wpm)

```
A storefront should feel like a shop you walk into.
Real product detail. Quiet typography. Whitespace that does the work most templates can't.
Checkout that takes payments customers actually use — cards, UPI, wallets — without us taking a cut.
An admin you don't have to learn. Each screen does one thing, clearly.
Pick a layout. Change a colour. Your storefront updates instantly.
Ninety days free. No card. Open your shop this afternoon.
Mark8ly.
```

Pronounce "Mark8ly" as **"mark eight ly"**. Pronounce "UPI" as **"U P I"** (letters).

---

## Storyboard — 6 scenes

Each scene includes (a) the source image to upload, (b) intent, (c) recommended motion prompt for the AI camera.

### Scene 1 · Bondi storefront hero · 0:00 – 0:04
- **Image**: `apps/onboarding/public/screens/storefront.png` (or `/tmp/mark8ly-video/stills/scene1-hero.png`)
- **VO**: "A storefront should feel like a shop you walk into."
- **Intent**: cold open. The product looks finished and lived-in before any UI explanation.
- **Motion prompt**: *"Slow cinematic dolly-in, 6% zoom over 4 seconds. Camera holds steady. No vertical drift. Soft focus pull on the foreground palm tree."*

### Scene 2 · Product browse · 0:04 – 0:13
- **Image**: `/tmp/mark8ly-video/stills/scene2-browse-full.png` (a full-page tall shot of the product grid)
- **VO**: "Real product detail. Quiet typography. Whitespace that does the work most templates can't."
- **Intent**: Show the catalogue without it feeling like a feed.
- **Motion prompt**: *"Vertical pan-down through a long page, slow continuous scroll, 8.5 seconds. No bounce, no easing at edges, perfectly linear. Camera stays parallel to the page."*

### Scene 3 · PDP → checkout · 0:13 – 0:28
- **Images**: `scene3-pdp.png`, then `scene3-checkout.png`
- **VO**: "Checkout that takes payments customers actually use — cards, UPI, wallets — without us taking a cut."
- **Intent**: Show the journey, with the racket as the through-line.
- **Motion prompt** (PDP): *"Subtle 4% zoom-in on the product, 5 seconds. Hold steady."*
- **Motion prompt** (checkout): *"Cross-dissolve from PDP to checkout (0.8s). Then very slow vertical drift down the form (3% over 9 seconds), like a Muji catalogue page being read."*

### Scene 4 · Admin dashboard · 0:28 – 0:34
- **Image**: `scene4-admin-dashboard.png`
- **VO**: "An admin you don't have to learn. Each screen does one thing, clearly."
- **Intent**: Show the tool feels approachable, not enterprise.
- **Motion prompt**: *"Hold static for 2 seconds. Then a gentle 3% zoom-in toward the right column over 4 seconds. No camera shake."*

### Scene 5 · Theme gallery · 0:34 – 0:43
- **Image**: `scene5-theme-gallery.png`
- **VO**: "Pick a layout. Change a colour. Your storefront updates instantly."
- **Intent**: The "wow" moment — bring the brand back to admin.
- **Motion prompt**: *"Slow lateral pan-right across the layout grid for 4 seconds, then a 5% zoom-in on the highlighted Editorial layout for 4 seconds. Editorial pacing, like a Phaidon book unfolding."*

### Scene 6 · Closer card · 0:43 – 0:55
- **Image**: `scene6-closer.png` (paper background, serif "Open your shop this afternoon")
- **VO**: "Ninety days free. No card. Open your shop this afternoon. Mark8ly."
- **Intent**: Final brand moment.
- **Motion prompt**: *"Hold completely static. Camera locked. Slow fade to white at the very end (1.5s). Type stays crisp."*

---

## Source assets

All scene stills are at 1920×1080 (or taller for pan scenes). Located:

```
/tmp/mark8ly-video/stills/
  scene1-hero.png
  scene1-hero-full.png      (3035px tall — for pan)
  scene2-browse.png
  scene2-browse-full.png    (4895px tall — for pan)
  scene3-pdp.png
  scene3-checkout.png
  scene4-admin-dashboard.png
  scene5-theme-gallery.png
  scene6-closer.png         (HTML→PNG render of closer card)
```

> **If `/tmp` was cleaned**, regenerate via `node /tmp/mark8ly-video/.video-stills.mjs` (script preserved in repo at `apps/onboarding/scripts/`) — Playwright drives the live storefront + admin and re-captures.

---

## Production checklist

- [ ] Higgsfield account
- [ ] Upload all 7 source images
- [ ] Paste brand spec into project settings
- [ ] Paste VO script, set voice = en-AU or en-UK female, pace 145 wpm
- [ ] Music: search "ambient", filter "low key / drone / minimal", pick volume ≈ -22dB under VO
- [ ] Generate scene 1, review, iterate motion prompt if shaky
- [ ] Generate remaining scenes, then full edit
- [ ] Export 1920×1080 master + 1080×1920 vertical
- [ ] Drop into `apps/onboarding/public/video/` replacing `tour.mp4` and `tour-reels.mp4`

---

## Iteration tips

1. **If voice still feels TTS** — try ElevenLabs voice "Rachel" or OpenAI `tts-1-hd` voice "shimmer" via Higgsfield's BYO-voice option if available.
2. **If motion stutters** — Higgsfield defaults to 24fps; bump to 30fps and re-render.
3. **If music fights the VO** — lower music to -28dB, side-chain duck under speech.
4. **If output looks too "AI"** — turn motion intensity down to 20–30%, keep camera moves under 5%.

---

## Caption strategy for vertical/reels cut

Higgsfield generates captions automatically. Set them to:
- Position: lower-third, ~260px from bottom
- Style: paper-tinted card with soft shadow, ink text, Source Sans 3 medium
- Word-by-word reveal — NOT auto-progressing all-at-once subtitles
- One caption block per VO sentence (matches our v3 reels structure)

---

*Last updated: see git log on this file.*
