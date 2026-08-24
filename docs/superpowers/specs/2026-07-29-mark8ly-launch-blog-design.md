# Mark8ly launch blog post — design

**Date:** 2026-07-29
**Publishing target:** `blog.tesserix.app` (MongoDB-backed, see "Publishing" below)
**Status:** drafted and saved to the blog as DRAFT (2026-07-29), awaiting review

---

## 1. Goal

Announce the Mark8ly launch on the Tesserix blog with a post that earns its place
next to the blog's existing technical explainers, while remaining fully readable
by a non-technical merchant.

The post is an **engineering story, not an advertisement**. It follows one
merchant signup from an email address to a live storefront and explains, in plain
English, everything the platform had to do along the way. The launch is the
reason to write. The story is the content.

**Success looks like:** a founder with no engineering background reads the whole
thing, understands why running many shops from one system is hard, trusts that
Mark8ly does it properly, and clicks through to The Bondi Store or to onboarding.

---

## 2. Audience and voice

Primary reader: a merchant or founder evaluating where to put their store.
Secondary reader: an engineer who follows the Tesserix blog.

Voice rules (from the standing blog rules):

- Very simple, clear English. Strong marketing language, no hype.
- Every technical idea gets a plain-language stand-in **before** its real name.
  Permissions become "who holds keys to which room". The outbox becomes "the
  paperwork that follows the guest".
- Professional, clean, engaging, well structured.
- No AI tells. **No em dashes.** No "delve", "unleash", "in today's fast-paced
  world", "it's not just X, it's Y", "seamlessly", "robust".
- Reading time **12 to 15 minutes**, so a target of **~2,800 words**.

---

## 3. Title and slug

**Title:** We Built a Real Online Store in Ten Minutes. Here Is Everything That Had to Happen.

**Slug:** `we-built-a-real-online-store-in-ten-minutes`

The title states the concrete outcome first and the promise of explanation
second, matching the pattern of the blog's best-performing posts.

---

## 4. Narrative spine

One signup, followed end to end.

A merchant types an email at `mark8ly.com`. Ten minutes later **The Bondi Store**
is live at `the-bondi-store.mark8ly.com` with its own admin dashboard at
`the-bondi-store-admin.mark8ly.com`. The Bondi Store is a real demo tenant and is
publicly reachable, so every claim in the post is checkable by the reader.

This mirrors the device the blog's strongest post (Temporal) uses: open on one
concrete moment, then spend the article explaining the machinery behind it.

---

## 5. Section outline

Target roughly 350 words per section.

| # | Section | Content | Word budget |
|---|---------|---------|-------------|
| 1 | **The ten minutes** | Open on the signup and the finished store. State the promise and what the reader will learn. Show the storefront screenshot early. | 350 |
| 2 | **Proving you are you, without a password** | Magic-link verification. Why there is no password to steal. Why we store only a fingerprint (SHA-256 hash) of the link, never the link itself. | 330 |
| 3 | **Giving a store its own front door** | Slug selection, subdomain, custom domains. How one visit to `the-bondi-store.mark8ly.com` finds the right shop out of every shop on the platform. Figure 2 lands here. | 400 |
| 4 | **Who is allowed to touch what** | Permissions as keys and rooms. Why isolation is the product, not a feature. Relationship-based permissions (OpenFGA) named once, explained in plain words. | 400 |
| 5 | **The handoff that used to break** | The honest section. The store existed before its permissions did, so the very first login failed. The outbox, the drainer, the retry. Werner Vogels quote lands here. Figure 3 lands here. | 400 |
| 6 | **The rest of the shop** | What "30 of 31 features shipped" means to a merchant: products and variants, orders and refunds, payments in multiple countries, tax, shipping carriers, coupons, gift cards, loyalty, reviews. Admin screenshot lands here. | 400 |
| 7 | **What this costs us** | House pattern, the honest tradeoffs. Multi-tenancy means one bad query can touch everyone. Custom domains mean DNS you do not control. Payments mean regional rules. What we chose not to build. | 350 |
| 8 | **Open yours** | Short close. Link to The Bondi Store and to onboarding. No hard sell. | 170 |

---

## 6. Visuals

All assets hosted at `storage.googleapis.com/tesserix-blog-assets/blog/images/2026/07/`.
Palette is the Mark8ly system: paper `#F7F6F2`, ink `#0E0E0C`, moss `#2D4A2B`,
elevated white `#FFFFFF`. Headline type is Source Serif 4, body Source Sans 3,
mono JetBrains Mono.

| Asset | Type | Purpose |
|-------|------|---------|
| **Banner** | Hand-authored SVG | Prominent, unique, editorial. Sets the paper/ink/moss tone at the top of the post. Also used as `featuredImage`. |
| **Figure 1** | Hand-authored SVG | The ten-minute journey as a horizontal timeline: email, verify, store created, permissions granted, storefront live. |
| **Figure 2** | Hand-authored SVG | How one request to `the-bondi-store.mark8ly.com` reaches the right store. The multi-tenancy explainer. |
| **Figure 3** | Hand-authored SVG | The handoff: store record, outbox row, drainer, permissions, and the retry that closed the race. |
| **Screenshot A** | PNG, Playwright | The Bondi Store storefront, public, no auth needed. |
| **Screenshot B** | PNG, Playwright | The admin dashboard, captured with the demo login at `admin.mark8ly.com/login`. |

Diagrams are hand-authored SVG rather than generated images: they stay crisp,
stay editable, match the house style of recent posts, and carry no AI-image look.

---

## 7. Quote

Placed in section 5, where the post admits the first version broke:

> "Everything fails, all the time." — Werner Vogels, CTO, Amazon

Rendered with the house `.pullquote` treatment.

---

## 8. Publishing

`blog.tesserix.app` stores content in **MongoDB, not git**. There is no markdown
file to commit. Publishing means inserting a document into
`tesserix_blog.posts` on the `mongodb-blog` StatefulSet.

The `content` field is one self-contained HTML block following the house format
used by the strongest recent posts:

```
<style> scoped to #mk8-blog </style>
<section id="mk8-blog">
  <span class="kicker">…</span>
  <h1>…</h1>
  <p class="lede">…</p>
  <hr>
  <h2>1. …</h2>
  … .diagram / .caption / .example / .pullquote …
</section>
```

Post document fields to set:

| Field | Value |
|-------|-------|
| `title` | Locked title from section 3 |
| `slug` | `we-built-a-real-online-store-in-ten-minutes` |
| `excerpt` | 1 to 2 sentences, merchant-facing |
| `content` | The HTML block |
| `status` | `PUBLISHED` |
| `metaTitle` / `metaDescription` | SEO variants |
| `featuredImage` / `featuredImageAlt` | Banner SVG on GCS, with alt text |
| `category` | `Platform Engineering` |
| `tags` | multi-tenant, commerce, ecommerce, saas, kubernetes, go, nextjs, openfga, platform, launch |
| `authorName` / `authorEmail` / `authorAvatar` | Match existing posts |
| `isFeatured` | `true` (launch post) |
| `publishedAt` / `createdAt` / `updatedAt` | Publish timestamp |
| `viewCount` | `0` |
| `versions` | Single version 1 entry |

**Draft first.** The HTML is written and reviewed locally before anything touches
the production database. The Mongo insert is the last step and happens only on
explicit approval.

---

## 9. Review gate before publishing

The draft is checked for:

1. Grammar, readability, and flow.
2. Reading time inside 12 to 15 minutes.
3. No em dashes and no AI-tell vocabulary.
4. Every technical term introduced in plain language first.
5. All links resolve, all images load from GCS.
6. Renders correctly at mobile width.

---

## 10. Out of scope

- The Mark8ly journal at `mark8ly.com/blog` stays a stub. Cross-posting is
  separate work.
- No changes to the blog application itself.
- No SEO or distribution campaign beyond the post's own metadata.
