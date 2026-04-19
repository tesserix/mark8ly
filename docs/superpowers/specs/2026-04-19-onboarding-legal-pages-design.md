# Onboarding app — Legal & compliance pages

**Date:** 2026-04-19
**App:** `apps/onboarding`
**Scope:** Fill gaps in the legal/compliance surface. Correct the entity identification (Tesserix Pty Ltd AU, not Mark8ly Mumbai). Wire new pages into footer + sitemap.

---

## 1. Background

The `apps/onboarding` marketing site currently exposes three legal routes:

- `/legal` — 21-line stub pointing at `security@mark8ly.com`
- `/privacy` — 166-line plain-language policy, but identifies the entity as "based in Mumbai, India"
- `/terms` — 124-line ToS that references an "acceptable use policy" and a "privacy policy" at signup but no AUP page exists, and the ToS has no governing law, jurisdiction, or dispute-resolution ladder

The operating entity is **Tesserix Pty Ltd** (ABN 59 694 070 865, ACN 694 070 865), registered in NSW, Australia. Mark8ly is a product of Tesserix Pty Ltd. Mumbai is an engineering/ops presence, not the legal entity. Every legal document must reflect this.

## 2. Goals

- Close the obvious legal-surface gaps before public launch
- Identify Tesserix Pty Ltd correctly on every page
- Honor extraterritorial privacy rights (GDPR, UK GDPR, CCPA/CPRA, DPDP India) without shifting governing law away from NSW
- Keep the editorial voice — plain language, Paper · Ink · Moss design tokens, `MarketingPage` + `PageHero` + `Prose` primitives
- Surface new pages in the footer and sitemap so they are discoverable
- Ship documentation only — cookie-consent banner runtime and other compliance *code* is a separate plan

## 3. Non-goals

- No cookie-consent banner runtime / CMP integration
- No changes to admin or storefront apps
- No new design primitives or components beyond `Prose`, `PageHero`, `MarketingPage`
- No claims of certification Tesserix doesn't actually hold (e.g., "SOC 2 Type II") — use "working toward" or omit
- No translation / i18n — English only for v1
- No sub-processor change-notification mailing list plumbing — the commitment is stated but delivery is manual

## 4. Entity identification block (verbatim, used everywhere)

> **Tesserix Pty Ltd** (ACN 694 070 865) — ABN 59 694 070 865 — registered in New South Wales, Australia. "Mark8ly" is a product of Tesserix Pty Ltd. Mumbai operations are conducted under Tesserix Pty Ltd.

Contact emails (already in use, keep as-is): `privacy@`, `dpo@`, `legal@`, `security@`, `support@`, all `@mark8ly.com`.

## 5. Page inventory

### 5.1 New pages

| Route | File | Indexable | Purpose |
|---|---|---|---|
| `/acceptable-use` | `app/acceptable-use/page.tsx` | yes | Prohibited content/conduct, anti-abuse, takedown process, appeals. Referenced in ToS §1. |
| `/cookies` | `app/cookies/page.tsx` | yes | Cookie categories, purposes, third-party cookies list, how to opt out (browser settings until CMP lands). |
| `/refunds` | `app/refunds/page.tsx` | yes | 90-day trial, 14-day post-purchase refund, Australian Consumer Law carve-out, process + SLAs. |
| `/sub-processors` | `app/sub-processors/page.tsx` | yes | Table of every processor, purpose, location, change-notification commitment. |
| `/dpa` | `app/dpa/page.tsx` | **no** (`robots: { index: false }`) | GDPR Art. 28 controller-processor addendum. Auto-accepted by B2B merchants at signup. SCCs referenced for EU→AU transfers. |
| `/security` | `app/security/page.tsx` | yes | Trust page: TLS, encryption-at-rest, access controls, SDLC, incident response SLA, vulnerability disclosure. |

### 5.2 Existing pages — updates

- **`/privacy`** — rewrite entity block; add APP-compliant sections (collection, use, disclosure, access/correction, complaints); add retention periods; add international-transfer basis (SCCs for EU, adequacy where applicable); add CCPA "Do Not Sell / Share" notice; add DPDP India rights pointer; add UK GDPR reference; add breach notification commitment (72 hours to regulator where required); add pointers to `/cookies` and `/sub-processors`.
- **`/terms`** — add governing law (NSW, Australia) and jurisdiction (courts of NSW); add Australian Consumer Law carve-out (cannot contract out of consumer guarantees); add DMCA-equivalent under Copyright Act 1968 (Cth) takedown process; add SLA stance (best-effort, no uptime warranty on current plans); add IP ownership + merchant-content license (limited — host/display/backup only); add force majeure; add dispute resolution ladder (negotiate → mediate → litigate in NSW); add link to `/acceptable-use`, `/dpa`, `/refunds`.
- **`/legal`** — repurpose from stub to hub: one-paragraph trust statement + linked index of all 8 legal documents. Keep indexable.

### 5.3 Hub layout (`/legal`)

Replaces the current `MarketingStub`. Uses `MarketingPage` + `PageHero` + a plain `<section>` with a grid of linked cards. One card per document with title + one-line description + last-updated date.

## 6. Shared patterns across all pages

- **Layout primitives:** `MarketingPage` shell; `PageHero` for eyebrow/title/lede; `Prose` for body.
- **Metadata:** `title`, `description`, `alternates.canonical`, and `robots` (noindex for `/dpa` only).
- **Last-updated eyebrow:** `"Last updated · April 2026"`.
- **Contact footer block:** every legal page ends with an explicit contact section pointing at the relevant email (`legal@`, `privacy@`, `dpo@`, `security@`).
- **Entity footer line:** every legal page ends with the entity identification block (section 4) verbatim in a hairline-separated footer inside the `Prose`.
- **Headings:** `h2` for top-level sections, `h3` for subsections. Numbered sections (e.g., `1. Who we are`) match the existing `/privacy` and `/terms` style.

## 7. Content outlines (per page)

### 7.1 `/acceptable-use`

1. Who this applies to (every account, every user of every store)
2. Prohibited content — illegal goods, weapons, controlled substances, adult content without age-gating, counterfeit, IP-infringing
3. Prohibited conduct — fraud, chargebacks, money-laundering, phishing, malware, unauthorized access, interference with other merchants
4. Intellectual property — you are responsible for rights to what you upload; we honor valid takedown notices (link to ToS DMCA-equivalent section)
5. Enforcement — warning → suspension → termination; severe violations skip warning
6. Appeals — email `legal@mark8ly.com` within 30 days
7. Reporting abuse — `abuse@mark8ly.com` (new mailbox; noted as aspirational until wired)
8. Contact + entity block

### 7.2 `/cookies`

1. What cookies are (one paragraph)
2. Categories we use
   - Strictly necessary — session, CSRF, auth (can't be disabled)
   - Functional — remembered preferences (lang, currency)
   - Analytics — OpenPanel / PostHog (where applicable)
3. Third-party cookies (table — same categories mapped to each vendor)
4. How to opt out — browser settings per major browser, with caveats about functional cookies
5. Consent posture — "We use strictly-necessary cookies without consent. Analytics cookies require consent where required by law (EU/UK). Consent banner is being rolled out in 2026."
6. Contact (`privacy@`) + entity block

### 7.3 `/refunds`

1. 90-day free trial — no charge, cancel any time
2. 14-day refund window post-first-charge — full refund, email `support@`
3. After 14 days — charges non-refundable, but cancel any time
4. Australian Consumer Law notice — "Nothing in this policy excludes, restricts, or modifies consumer guarantees under the Australian Consumer Law, which apply alongside this policy to Australian consumers."
5. Chargebacks — we prefer a conversation first; aggressive chargebacks without contact may lead to account review
6. Process — email template, typical turnaround 5 business days
7. Contact (`support@`, `billing@`) + entity block

### 7.4 `/sub-processors`

1. One-paragraph intro — what a sub-processor is and why the list matters
2. Table with columns: **Vendor · Purpose · Data category · Location / region · Transfer mechanism**
3. Rows:
   - Google Cloud Platform — hosting, Cloud SQL, Pub/Sub, GCS, Secret Manager, GIP — operational + authentication data — `asia-south1` (Mumbai) — SCCs for non-AU data subjects, GCP adequacy where applicable
   - Cloudflare — DNS, CDN, DDoS protection, tunnel — request metadata — global edge — SCCs
   - Stripe — payment processing — payment metadata (no card numbers stored by us) — global — SCCs + PCI DSS
   - Razorpay — payment processing (IN merchants) — payment metadata — India — domestic
   - SendGrid (Twilio) — transactional email — email + message content — US — SCCs
   - AWS SES — transactional email — email + message content — multiple — SCCs
   - AWS SNS — SMS — phone + message content — multiple — SCCs
   - Firebase Cloud Messaging — push notifications — device tokens — global — SCCs
   - OpenPanel — product analytics — usage events — EU — GDPR domestic
   - PostHog — product analytics (onboarding only) — usage events — US/EU — SCCs
   - GitHub — source control, CI — source code, build metadata — US — SCCs
4. Change-notification commitment — we will announce new sub-processors on this page at least 14 days before onboarding them. B2B merchants may subscribe to updates by emailing `privacy@`.
5. Contact (`dpo@`) + entity block

### 7.5 `/dpa`

Framed as "Mark8ly Data Processing Addendum" auto-accepted on signup.

1. Parties — Tesserix Pty Ltd (Processor) and the merchant (Controller)
2. Scope — applies whenever merchant Personal Data is processed under the ToS
3. Definitions — GDPR-aligned (Controller, Processor, Personal Data, Processing, Sub-processor, Data Subject)
4. Processing details — nature, purpose, duration, categories of data, categories of data subjects
5. Processor obligations — instructions, confidentiality, security, sub-processor authorisation, assistance with DSRs, assistance with DPIAs, breach notification, deletion/return on termination, records of processing
6. Sub-processors — general authorisation, link to `/sub-processors`, 14-day notice for new sub-processors, objection right
7. International transfers — SCCs incorporated by reference for EU → AU transfers; SCCs (UK IDTA addendum) for UK → AU; adequacy / reliance on transfer mechanisms where applicable
8. Security measures — reference `/security` as the authoritative description; encryption in transit (TLS 1.2+), encryption at rest, access controls, secure SDLC, logging
9. Audit rights — merchant may request summary of Processor's most recent third-party security review; on-site audits require 30 days' notice and NDA
10. Liability — capped at ToS liability cap; does not limit liability arising from breach of data protection law
11. Term — co-terminous with the main agreement
12. Governing law — NSW, Australia (consistent with ToS)
13. Contact (`dpo@`, `legal@`) + entity block

### 7.6 `/security`

Replaces the existing `/legal` stub content. This is the trust page. Keep the same `MarketingPage` shell but fill with real content.

1. Our posture — short opening paragraph on what "security" means at Mark8ly
2. Encryption — TLS 1.2+ everywhere, AES-256 at rest via GCP CMEK where configured
3. Access controls — least-privilege, SSO for staff, MFA required, audit logging on production access
4. Infrastructure — GKE Autopilot, Cloud SQL PostgreSQL, Istio mTLS, Knative scale-to-zero
5. Authentication & authorisation — Google Identity Platform for end-user auth, OpenFGA for fine-grained permissions
6. Secrets management — GCP Secret Manager + External Secrets Operator; no secrets in source
7. Secure SDLC — code review, dependency scanning, signed container images, GAR registry
8. Incident response — 72-hour breach notification to affected customers and regulators where required
9. Vulnerability disclosure — responsible disclosure program; email `security@mark8ly.com`; we will acknowledge within 72 hours
10. What we're working toward — explicit "not yet certified" language for SOC 2 / ISO 27001 so we don't misrepresent
11. Sub-processors — link to `/sub-processors`
12. Contact (`security@`) + entity block

### 7.7 `/legal` (repurposed hub)

1. `PageHero` — eyebrow "Legal & compliance", title "Policies.", lede about transparency
2. `<section>` with a responsive 2-column grid; one card per document, each card linking to a real legal page. Cards: Privacy, Terms, Acceptable use, Cookies, Refunds, Sub-processors, DPA, Security.
3. Entity identification block at the bottom.
4. `robots: { index: true, follow: true }`.

### 7.8 `/privacy` — update

- Replace §1 "Who we are" — use entity identification block verbatim, note Mumbai operations
- Add a new §5a (after §5 Data security) — **International transfers**: "Personal information may be processed in Australia, India, the EU, the UK, and the United States by Tesserix Pty Ltd and our sub-processors. Where we transfer personal information out of the EEA, UK, or other regions with data-transfer restrictions, we rely on Standard Contractual Clauses (SCCs) and adequacy decisions as applicable."
- Add a new §6a **Retention** — "Account data retained for the life of the account plus 90 days after closure for wind-down. Billing records retained 7 years to meet Australian tax obligations. Support correspondence retained 2 years. You may request earlier deletion under §6."
- Expand §6 to mention specific frameworks: Australian Privacy Principles (APPs), GDPR, UK GDPR, CCPA/CPRA (including "Right to Know / Delete / Correct / Opt out of sale or sharing — note: we do not sell or share personal information for cross-context behavioural advertising"), DPDP India
- Add §8 **Cookies** — pointer to `/cookies`
- Add §9 **Sub-processors** — pointer to `/sub-processors`
- Add §10 **Breach notification** — "We notify affected users and regulators within 72 hours of becoming aware of a personal data breach where required by applicable law."
- Renumber accordingly; keep §1–§7 structure intact otherwise
- Update contact section: keep `privacy@`, `dpo@`; add Australian Privacy Commissioner (OAIC) complaint pointer

### 7.9 `/terms` — update

- Add a new §0 **Parties** — entity identification block verbatim, define "Mark8ly", "you / your", "we / us / our"
- Keep §1–§5 as-is structurally; add a link in §4 to `/acceptable-use` instead of listing prohibited activities inline (keep a short summary, deep-link to AUP)
- Replace §6 **Limitation of liability** with:
  - 6.1 Cap: 12 months' fees paid
  - 6.2 Exclusions: death/personal injury, fraud, willful misconduct, statutory liabilities that cannot be excluded (including under ACL)
  - 6.3 Consequential damages excluded to the extent permitted
- Add new §7 **Australian Consumer Law** — explicit carve-out that consumer guarantees under ACL apply regardless of anything in these terms
- Add new §8 **Content license** — merchant grants Tesserix a limited, non-exclusive license to host, display, process, and back up merchant content solely to provide the service
- Add new §9 **Intellectual property** — Tesserix owns the platform, merchant owns their content, third-party marks attributed
- Add new §10 **Takedown (Copyright Act 1968 (Cth))** — process for submitting IP takedown notices; counter-notice process
- Add new §11 **Data processing** — reference `/dpa`, auto-accepted by merchants acting as controllers
- Add new §12 **Refunds** — pointer to `/refunds`; 14-day window, ACL carve-out
- Add new §13 **Service levels** — best-effort availability; no uptime guarantee on current plans; planned maintenance windows announced in-app
- Add new §14 **Force majeure** — cloud outages, natural disasters, acts of government, etc.
- Add new §15 **Governing law and dispute resolution**:
  - 15.1 Governed by the laws of New South Wales, Australia
  - 15.2 Exclusive jurisdiction of the courts of New South Wales
  - 15.3 Dispute ladder: good-faith negotiation → mediation (Resolution Institute rules) → litigation
- Update contact section: `legal@`, `support@`
- Keep tone editorial and plain-language; this is additions and restructuring, not a rewrite

## 8. Navigation & discoverability

### 8.1 Footer

Update `apps/onboarding/components/marketing/Footer.tsx` "Company" column:

Current:
```
About · Contact · Privacy · Terms
```

New: keep the Company column focused on company, add a fourth "Legal" column.

```
Company: About · Contact · Careers (existing if present)
Legal: Privacy · Terms · Cookies · Acceptable use · Refunds · Security
```

Sub-processors and DPA are not in the footer — they are reachable from `/legal` and from contextual links inside Privacy and Terms. Keeps the footer from ballooning.

Grid adjusts from `sm:grid-cols-3` to `sm:grid-cols-4` when the Legal column is added. The visual rhythm stays editorial — four columns still fit the `max-w-6xl` container.

### 8.2 Sitemap

Update `apps/onboarding/app/sitemap.ts` `legal` array to include every indexable legal route (everything except `/dpa`):

```
/privacy · /terms · /cookies · /acceptable-use · /refunds · /sub-processors · /security · /legal
```

All `changeFrequency: "yearly"`, `priority: 0.3` — matches the existing pattern for `/privacy` and `/terms`.

## 9. File layout

```
apps/onboarding/
  app/
    acceptable-use/page.tsx     [new]
    cookies/page.tsx            [new]
    refunds/page.tsx            [new]
    sub-processors/page.tsx     [new]
    dpa/page.tsx                [new, noindex]
    security/page.tsx           [new]
    legal/page.tsx              [rewrite — hub, no longer stub]
    privacy/page.tsx            [update — entity, APPs, transfers, retention, breach, cookie/subprocessor pointers]
    terms/page.tsx              [update — governing law, ACL, DPA, DMCA-equivalent, licenses, SLA, force majeure, dispute ladder]
    sitemap.ts                  [update — new routes]
  components/
    marketing/
      Footer.tsx                [update — add Legal column]
```

No new components are introduced. All existing primitives are reused.

## 10. Testing & verification

- `pnpm --filter onboarding build` — must compile cleanly with `output: "standalone"`
- `pnpm --filter onboarding lint` — no new warnings
- `pnpm --filter onboarding typecheck` (or `tsc --noEmit`) — clean
- Manual browser verification for each of the 8 pages:
  - Correct metadata (`title`, `description`, `robots` where applicable)
  - Entity identification block present and accurate on every legal page
  - All internal links resolve
  - Keyboard nav works end-to-end, focus ring visible on all links
  - `prefers-reduced-motion` honored (no animations introduced anyway, but sanity-check)
  - WCAG 2.1 AA spot-check on headings, link contrast, text size
- `/legal` renders a real hub grid, no longer the stub
- Sitemap includes the new indexable routes; `/dpa` is absent
- Footer shows the new Legal column on desktop and collapses correctly on mobile

## 11. Risks & caveats

- **Not a lawyer-reviewed document.** These drafts are written by an engineer using plain-language legal style and industry-standard structures. Before public launch, have an Australian-qualified lawyer review `/terms`, `/dpa`, and `/privacy` at minimum. Flag this explicitly to the user post-implementation.
- **Sub-processors list must stay truthful.** If a vendor on the list is actually not used, remove it. If we add one, update the list within 14 days.
- **Cookie consent banner is separate work.** This doc promises it "is being rolled out in 2026" — that commitment needs to be scheduled, not just written.
- **ACL carve-out is non-negotiable.** Any future change to `/terms` liability clauses must preserve the ACL carve-out for AU consumers.
- **SOC 2 / ISO language.** Never claim certifications we don't hold. "Working toward" is acceptable; "certified" is not.
- **Breach notification SLA (72h) is a commitment.** Must be reflected in the incident-response runbook once it exists.

## 12. Out of scope (explicit)

- Cookie consent banner / CMP integration — separate plan
- Admin / storefront legal surface updates — separate plan
- Multi-language / i18n for legal pages
- Automated sub-processor notification mailing list
- Signed-DPA workflow (wet signatures / DocuSign for enterprise customers)
- Marketing changes beyond the footer
- Moving any existing content to a CMS

## 13. Rollout plan

1. Land all page files + footer + sitemap in a single commit on `main`
2. Visual spot-check every new route locally
3. Deploy to onboarding prod via existing pipeline (no infra changes needed)
4. Post-deploy: ask user to confirm entity details pass internal review before marketing the new pages
5. Follow-up ticket: "Have AU-qualified lawyer review /terms, /privacy, /dpa"
