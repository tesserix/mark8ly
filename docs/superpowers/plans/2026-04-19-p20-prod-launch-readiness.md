# P20 — Production Launch Readiness Plan

> **Not a code plan.** Do not run `superpowers:executing-plans` against this. This is a cross-functional launch-readiness tracker with legal/ops/content/testing/product tasks — no TDD cycle applies. Use the checkbox list as a punchlist; track owners + dates inline.

**Goal:** Bring Mark8ly subscription v2.3 from "code complete" to "live serving real merchants" with every legal, operational, content, and verification dependency resolved.

**Start state (2026-04-19):** 17/19 code plans shipped; P15 + P16 in progress, expected complete ~2 weeks.

**Target launch:** 6-8 weeks from today, gated by NZ tax counsel critical path.

**Spec:** [`docs/superpowers/specs/2026-04-17-subscription-model-design.md`](../specs/2026-04-17-subscription-model-design.md) — §20 Legal & TOS, §26 Risks & open questions.

---

## Critical-path summary

```
Week:  1      2      3      4      5      6      7      8
       ├──────┼──────┼──────┼──────┼──────┼──────┼──────┼──────┤
WS-A   [NZ tax counsel opinion ←────┤
                    └─[NZ GST registration if required ───→ GO/NO-GO]
       [TOS/Privacy/DPA review ─────────┤
WS-B   [Stripe/SendGrid/reCAPTCHA/SM provisioning ──┤
WS-C        [Email templates + legal templates + FAQ copy ───────┤
WS-D                           [E2E + load + chaos ─────────────┤
WS-E                                   [On-call + runbooks ─────┤
WS-F   [Product decisions ─┤
WS-H        [CVE triage ──┤[critical + high remediation ─────┤
                                                        SOFT LAUNCH ✓
```

**The NZ tax opinion is the single biggest risk.** If counsel determines NZ GST registration is required, the 4-8 week registration processing can push launch to week 12. Start WS-A Task 1 THIS WEEK regardless of other progress.

---

## Dependency map

| Depends on | Unblocks |
|---|---|
| WS-A1 (NZ counsel opinion) | Accept NZ signups (code already gates via `NZ_TAX_VALIDATION_ENABLED=false` per P7) |
| WS-A2 (TOS/Privacy/DPA) | Public marketing, any paid signup |
| WS-A3 (EU/UK/India tax counsel) | Confirms reverse-charge model stands |
| WS-B1 (Stripe AU verified + GST registered) | All billing |
| WS-B2 (SendGrid + reCAPTCHA) | Signup gate + email enforcement |
| WS-B3 (Tax registry credentials) | Tax-ID validation (P7) moves from code-ready to operational |
| WS-C1 (Email templates) | Dunning, cancellation, trial, SCA flows user-facing |
| WS-D1 (Full E2E) | Go/no-go signal |
| WS-D2 (CNPG chaos drill) | Validates §22 RTO/RPO claims |
| WS-F1 (SOC 2 timing decision) | Insurance requirements, procurement deals |
| WS-H1 (CVE triage baseline) | Categorizes 24 dependabot alerts by severity × load-bearing |
| WS-H2 (Critical CVE remediation) | Launch gate — no open critical vulns |
| WS-H3 (High CVE remediation) | Launch gate — no open high vulns without documented waiver |

---

## Workstream A — Legal + Tax (CRITICAL PATH)

**Owner:** Founder/CEO working with external counsel. Budget: $15-30k per spec §26.1.

### A1. NZ tax counsel + potential GST registration — **CRITICAL PATH**

- [ ] **A1.1** Engage NZ tax counsel (ideally a Big-4 Australasian firm with NZ GST OIDAR expertise). Scope: is Mark8ly a non-resident B2B SaaS provider required to register for NZ GST under the 2016 remote services rules?
  - **Owner:** CEO
  - **Start:** Week 1, day 1
  - **Duration:** 1-2 weeks for opinion
  - **Acceptance:** Signed legal opinion in writing, stating GST-required OR reverse-charge-sufficient
- [ ] **A1.2** If opinion says **registration required:** initiate IRD GST registration.
  - **Duration:** 4-8 weeks processing
  - **During:** keep `NZ_TAX_VALIDATION_ENABLED=false` — P7 code already refuses NZ signups with 503
  - **Acceptance:** GST number issued; env flag flipped to `true` in production; first NZ test signup validates
- [ ] **A1.3** If opinion says **reverse-charge sufficient:** document decision in `docs/legal/nz-tax-opinion.md`, flip `NZ_TAX_VALIDATION_ENABLED=true`, ship minor copy update clarifying B2B-only for NZ.

**Blockers for launch in NZ:** A1 must resolve. Rest of world can launch without A1.

### A2. TOS + Privacy Policy + DPA — MANDATORY

- [ ] **A2.1** Engage counsel for TOS drafting. Must cover all items in spec §20.1:
  - 14-day cooling-off terms
  - Non-refundable Pro+App setup fee ($2,000)
  - Jurisdictional notices (18 countries)
  - Subprocessor list (Stripe, SendGrid, GCP, Cloudflare, OpenFGA, Firebase for Pro+App)
  - GDPR/DPDP disclosures
  - Right-to-erasure carve-out (§15.4 customer-subject data)
  - SLA definition (Pro+App 99.9%)
  - AUP (Acceptable Use Policy)
  - US/CA business-entity attestation (§19.3.1)
  - AU GST inclusivity (§19.4)
  - PPP pricing disclosure (§4.1.3)
  - **Owner:** CEO + external counsel
  - **Duration:** 4-8 weeks
- [ ] **A2.2** Privacy Policy covering GDPR + DPDP (India) + CCPA (California). India needs a named grievance officer — designate + publish contact.
  - **Owner:** CEO + counsel
  - **Duration:** 2-3 weeks
- [ ] **A2.3** DPA template (GDPR Article 28 compliant) — required on Pro+App deals.
  - **Owner:** Counsel
  - **Duration:** 1-2 weeks
- [ ] **A2.4** Cookie policy + GDPR consent banner — coordinate with WS-C (content) + P16 admin team for implementation.
  - **Owner:** Counsel + marketing
  - **Duration:** 1 week
- [ ] **A2.5** MSA template for Pro+App — required before first Pro+App deal.
  - **Owner:** Counsel
  - **Duration:** 2-3 weeks
- [ ] **A2.6** Publish all documents to `/legal/` on marketing site + link from admin footer.
  - **Owner:** Marketing + P16 frontend team
  - **Acceptance:** All docs live, checksum-logged to `docs/legal/published-versions.md`

### A3. EU/UK/India reverse-charge tax counsel

- [ ] **A3.1** Get legal opinion confirming B2B reverse-charge model is valid in EU (6 countries), UK, and India for Mark8ly's supply type (§19.1).
  - **Owner:** CEO + counsel (separate firm from NZ recommended)
  - **Duration:** 1-2 weeks
  - **Acceptance:** Signed opinion filed at `docs/legal/eu-uk-in-reverse-charge-opinion.md`

### A4. Post-launch tax items (tracked, not blocking)

- [ ] A4.1 EU/UK tax re-confirm at 100 merchants/country (§20.4) — calendar reminder
- [ ] A4.2 India GST OIDAR registration trigger at ₹20 lakh/mo (§20.4) — monitoring alert in P17

---

## Workstream B — Operational accounts + secrets

**Owner:** Platform engineering lead. Budget: subscription costs + setup fees.

### B1. Stripe

- [ ] **B1.1** Stripe AU account fully verified (business docs, bank account, tax ID) — confirm prod access beyond test mode
- [ ] **B1.2** AU GST registration activated on Stripe Tax for AU tenants only
- [ ] **B1.3** Webhook endpoint URL registered in Stripe Dashboard pointing to prod `https://api.mark8ly.com/webhooks/stripe-billing`
- [ ] **B1.4** Webhook signing secret rotated into GCP Secret Manager as `stripe-billing-webhook-secret` (prod value)
- [ ] **B1.5** Stripe live secret key in Secret Manager as `stripe-billing-secret-key`
- [ ] **B1.6** Radar rules reviewed — `radar.early_fraud_warning` webhook wired (P2), confirm Radar enabled in Stripe Dashboard
- [ ] **B1.7** Test `invoice.payment_action_required` flow end-to-end on Stripe India sandbox — use test card `4000 0035 6000 0008` — spec §26.1 blocker
- [ ] **B1.8** Run `cmd/billing-bootstrap` against **prod** Stripe to create all 8 Price objects + products
- [ ] **B1.9** Stripe Australia bank account remains single-currency (split-currency USD-AU settlement deferred per §4.2.2 until $200k ARR)
- **Owner:** Platform lead
- **Duration:** 2-3 weeks (Stripe verification can take 2 weeks)
- **Acceptance:** Manual test subscription creates + webhook round-trips end-to-end in prod

### B2. GCP Secret Manager — provision real secrets

- [ ] **B2.1** `arbitrage-ip-hmac-key` (P8) — generate initial 256-bit secret via `openssl rand -base64 32`, store at `projects/tesserix-prod/secrets/arbitrage-ip-hmac-key/versions/1`
- [ ] **B2.2** Cloud Scheduler job for 30-day HMAC key rotation — deploy per P8's cron design; verify first rotation happens on schedule
- [ ] **B2.3** Break-glass TOTP secrets (P13) — auto-generated on first Pro tenant signup; IAM policy: only `break-glass-responders` Google Group can read
- [ ] **B2.4** Apple ASC + Google Play credential paths (P15) — provisioned per Pro+App tenant at onboarding; CSM workflow in WS-E
- [ ] **B2.5** Audit IAM on every secret path — confirm ≤2 staff on merchant-credential paths, ≤3 staff on Stripe paths (§18.3, §18.9)
- **Owner:** Platform lead
- **Duration:** 1 week
- **Acceptance:** `gcloud secrets list` shows all expected paths; `gcloud secrets get-iam-policy` matches §18 constraints

### B3. Third-party accounts

- [ ] **B3.1** SendGrid account upgraded to sending plan supporting projected volume (Starter 15k + Studio 50k + trial 5k per merchant per month — assume 100 merchants avg Starter at launch = 1.5M emails/month)
- [ ] **B3.2** SendGrid domain authentication: SPF + DKIM + DMARC DNS records for `mark8ly.com`
- [ ] **B3.3** SendGrid API key rotated into Secret Manager as `sendgrid-api-key`
- [ ] **B3.4** reCAPTCHA Enterprise site created in GCP — site key (public) + secret (Secret Manager `recaptcha-secret`)
- [ ] **B3.5** Disposable-email blocklist feed — spec §5.1 says "refreshed weekly". Confirm code ships a refresh service (P5) and point it at [disposable-email-domains](https://github.com/disposable-email-domains/disposable-email-domains) or a commercial equivalent
- [ ] **B3.6** PagerDuty service + integration URL (for P2 orphan webhook, P17 alerts); route to on-call (WS-E)
- [ ] **B3.7** Slack workspace channels: `#security-alerts`, `#sales-inbox`, `#oncall`, `#dunning-ops`, `#billing-ops` — incoming-webhook URLs stored in Secret Manager
- [ ] **B3.8** Openpanel / PostHog analytics in prod mode (already wired per CLAUDE.md stack) — confirm events flow
- **Owner:** Platform lead
- **Duration:** 1-2 weeks
- **Acceptance:** Test email delivery to real inbox; reCAPTCHA challenge on staging signup; test Slack message to each channel

### B4. Tax registry API credentials (13 validators — code ready, keys not)

- [ ] **B4.1** UK HMRC VAT API — register as a third-party software; complete application + receive production credentials
- [ ] **B4.2** EU VIES — open access; confirm our prod IP isn't rate-limited; set up polite retry with backoff
- [ ] **B4.3** India GSTN — production API credentials via [GSTN Suvidha Provider](https://www.gstsuvidhaprovider.com/) partner agreement
- [ ] **B4.4** Australia ABN Lookup — free, register for GUID; store in Secret Manager
- [ ] **B4.5** Singapore ACRA — API access application
- [ ] **B4.6** SEA registries (MY/TH/PH/ID/VN) — spec says "enrollment-gated possibly". P7 code supports manual-review queue as fallback; consider **launching SEA with manual-review only** for v1 and pursuing API integration post-launch
- [ ] **B4.7** NZ IRD — gated by A1 outcome
- **Owner:** Platform lead + CEO (some require business relationships)
- **Duration:** 2-4 weeks (GSTN + HMRC are slowest)
- **Acceptance:** Hit each registry's production endpoint once with a real business's tax ID; P7 validator returns `valid=true`

---

## Workstream C — Content + templates

**Owner:** Marketing lead + founder. Tone: editorial, calm (per brand voice in `mark8ly/.impeccable.md`).

### C1. Email templates (editorial tone, no urgency language)

Required per P5/P6/P11 + §16.4:
- [ ] **C1.1** Welcome email (signup) — confirms email, outlines 90-day trial
- [ ] **C1.2** Email verification link
- [ ] **C1.3** Tax-ID validation success
- [ ] **C1.4** Tax-ID lapsed (quarterly revalidation §19.5) — 14-day window warning
- [ ] **C1.5** Trial nudge day 60 — "Add card before day 90"
- [ ] **C1.6** Trial nudge day 75 — amber escalation
- [ ] **C1.7** Trial nudge day 85 — "5 days" final
- [ ] **C1.8** Trial expired day 90 — store closed, grace window explanation
- [ ] **C1.9** Card added during trial — "First charge on [date]" confirmation
- [ ] **C1.10** Invoice paid receipt
- [ ] **C1.11** Payment failed (past_due entry)
- [ ] **C1.12** Dunning day 5 nudge
- [ ] **C1.13** Dunning day 7 final warning
- [ ] **C1.14** Dunning failure (expired)
- [ ] **C1.15** SCA action required T-14, T-7, T-1 (3 templates)
- [ ] **C1.16** SCA resolved (invoice.paid)
- [ ] **C1.17** Cancellation confirmation
- [ ] **C1.18** Save-offer email copy (if triggered outside admin dialog)
- [ ] **C1.19** Cancellation finalized (at cancels_at)
- [ ] **C1.20** Win-back day 30 — 20% off 6 months
- [ ] **C1.21** Hard-delete day 150 warning
- [ ] **C1.22** Hard-delete completed (data purged; billing archive retained notice)
- [ ] **C1.23** Refund issued confirmation
- [ ] **C1.24** Promo applied confirmation
- [ ] **C1.25** Plan changed (upgrade/downgrade) confirmation
- [ ] **C1.26** Downgrade blocked at period end (§4.5.1 failure — over quota)
- [ ] **C1.27** Arbitrage flag notice (P8) — "We've noted a discrepancy"
- [ ] **C1.28** Arbitrage appeal received + resolution emails
- [ ] **C1.29** Country change notice 14d before renewal (§4.6)
- [ ] **C1.30** Pro+App onboarding sequence: setup fee invoice, ASC creds request, UI customization gate, build start, first submission, live
- [ ] **C1.31** Pro+App teardown sequence: day 0 CSM notice, day 30 downloads blocked, day 60 pulled, day 90 credentials purged

**Format:** HTML + plaintext pairs per template. Store under `services/notification-service/templates/` (or wherever existing templates live). Use handlebars-style interpolation.

**Owner:** Marketing + copywriter; engineering reviews for variable placeholders match the service's data shape.
**Duration:** 2-3 weeks
**Acceptance:** Every template renders in test-send to a real inbox; tone reviewed against brand; no emoji, no urgency copy, no "Hey there!"

### C2. Pricing-page FAQ content

Per spec §3.3, §4.1.3, §9.1:
- [ ] **C2.1** "What's the difference between Pro priority support and CSM?" (§3.3)
- [ ] **C2.2** "Why do prices vary by country?" (§4.1.3 PPP disclosure)
- [ ] **C2.3** "Can I negotiate pricing based on another region?" (§4.1.3 — answer: no)
- [ ] **C2.4** "What's the refund policy?" (§8 14-day cooling-off)
- [ ] **C2.5** "What happens to my data if I cancel?" (§15 + §15.4 GDPR customer portal)
- [ ] **C2.6** "Do I need a tax ID?" (§19 B2B-only explanation)
- [ ] **C2.7** "Can I use my own payment gateway?" (§6 BYO keys)
- [ ] "Feature comparison" table matches §9 matrix exactly
- **Owner:** Marketing + founder
- **Duration:** 1 week

### C3. Pro sales brief (PDF)

- [ ] **C3.1** 2-3 page PDF for Pro "Download brief" CTA — covers feature depth, SLA, SSO, full R/W API, pricing floor, contact path
- [ ] **C3.2** Separate Pro+App brochure covering the white-label app scope, timeline, 4.2.6 risk acknowledgment, setup fee breakdown
- **Owner:** Marketing + design
- **Duration:** 2 weeks

### C4. Closed-store page copy audit (P12 Worker)

- [ ] **C4.1** Review `tesserix-k8s/workers/storefront-gate/src/closed.html` — default copy likely placeholder
- [ ] **C4.2** Ensure merchant branding interpolation variables work end-to-end (store name, logo, support email)
- [ ] **C4.3** Editorial copy per brand voice — no urgency
- **Owner:** Marketing + P12 reviewer
- **Duration:** 2 days

### C5. Admin UX microcopy

The admin frontend (P16) ships strings in component files. Catalog them for copy review:
- [ ] **C5.1** Cancellation modal sequence (§15 flow) — including save-offer prospective-only text from §15.1 BA finding verbatim
- [ ] **C5.2** Store-close-before-downgrade dialog (§4.5.1) — "Close does NOT free a plan slot" clarity
- [ ] **C5.3** Payment-action-required banner (§4.7) — "Complete authentication" CTA
- [ ] **C5.4** Failed-payment banner (past_due)
- [ ] **C5.5** Arbitrage banner (§18.8.1)
- [ ] **C5.6** Tax-ID clock-pause indicator
- [ ] **C5.7** 402 Payment Required error message from `RequireActive` (P3)
- **Owner:** P16 frontend team + marketing review
- **Duration:** parallel to P16 execution

---

## Workstream D — Testing + verification

**Owner:** QA + engineering lead.

### D1. P1 review follow-ups (still open since 2026-04-18)

- [ ] **D1.1** Add `models_test.go` for `billingarchive` + `campaignbudget` packages — matching pattern of other 5 new tables
- [ ] **D1.2** Confirm CI runs `go test -tags=integration ./...` — otherwise new test suites never execute
- [ ] **D1.3** Add CI assertion: `go test ./...` (no tags) must pass without "no test files" false green for new packages
- **Owner:** Engineering
- **Duration:** 2 days
- **Acceptance:** CI run on a PR shows integration tests executed + test counts > 0 per new package

### D2. Full E2E journey — spec §28 criteria 1-55

Write a single E2E test (Playwright + backend) covering the complete merchant lifecycle:
- [ ] **D2.1** Signup → email verify → reCAPTCHA pass → tax ID submit (IN merchant) → trial day 0
- [ ] **D2.2** Product create day 15 (activation milestone)
- [ ] **D2.3** Card add day 45 — assert subscription created with `trial_end = day 90`, no charge yet (criterion 46)
- [ ] **D2.4** Day 90 advance clock → assert first charge in INR, subscription active
- [ ] **D2.5** Plan change Starter → Studio → assert proration
- [ ] **D2.6** Plan change Studio → Pro monthly → assert +20% premium applied ($119 equiv)
- [ ] **D2.7** Apply promo code → assert absolute floor rejects at 50% off ₹999 (criterion 40)
- [ ] **D2.8** Cancel with save-offer → assert prospective-only (criterion 54)
- [ ] **D2.9** Reverse save-offer → back to active
- [ ] **D2.10** Cancel final → reach expired → store_closed day 14 → pending_hard_delete day 90 (criteria 48 + 49)
- [ ] **D2.11** Full 151-day simulated time
- **Owner:** QA
- **Duration:** 2 weeks (includes time-mocking infrastructure)
- **Acceptance:** Test runs in CI nightly; green

### D3. Load test — webhook dispatcher

- [ ] **D3.1** k6 or vegeta scenario: 1000 signed Stripe webhook events/min for 10 min; assert 99p < 500ms, 0 rejections
- [ ] **D3.2** Orphan-resolver under load: flood with events for unknown customers; assert retry cap + manual-review-flag behavior
- [ ] **D3.3** Campaign-budget concurrent send test: 100 simultaneous `campaign.email.sent` at limit edge; assert exactly-limit decrements
- **Owner:** Engineering
- **Duration:** 1 week
- **Acceptance:** Load reports attached to `docs/load-tests/2026-04-xx-v2.3.md`

### D4. CNPG chaos drill — validate §22 RTO/RPO

- [ ] **D4.1** On staging CNPG cluster: `kubectl delete pod mark8ly-postgres-primary-1` — measure time to standby promotion, app reconnect, successful write
- [ ] **D4.2** Assert RTO ≤ 2 min (§22 target at 100-merchant tier)
- [ ] **D4.3** Write during failover → query after — assert RPO = 0 (synchronous_commit works)
- [ ] **D4.4** Document results in `docs/runbooks/cnpg-failover-drill-2026-04-xx.md`
- **Owner:** Platform
- **Duration:** 2 days
- **Acceptance:** Drill passes; runbook updated with actual numbers

### D5. Security regression suite

- [ ] **D5.1** IDOR sweep: for every admin endpoint, verify a request with tenant A's session carrying tenant B's `storeId` returns 404 (not 403 — 403 leaks existence)
- [ ] **D5.2** Signature replay: Stripe webhook replay within 5-min window → `{"status":"duplicate"}`; outside window → 401
- [ ] **D5.3** `business_entity_attestations` + `app_contract_attestations` DELETE rejected at role level (criterion 50)
- [ ] **D5.4** API key revoked → immediate 401; rotation overlap → both valid for 24h
- [ ] **D5.5** Break-glass login requires both password AND TOTP; Slack alert fires on success
- [ ] **D5.6** Cross-tenant credential access (P15) returns 404
- [ ] **D5.7** HMAC IP rotation 30d preserves 31d overlap; cross-window correlation severed beyond 31d (criterion 51)
- [ ] **D5.8** `payment_action_required` merchant keeps full admin + storefront access (criterion 38)
- **Owner:** Security + engineering
- **Duration:** 1 week
- **Acceptance:** All 8 assertions green as automated tests; log to `docs/security/2026-04-xx-regression-suite.md`

### D6. Staging soak

- [ ] **D6.1** Deploy full v2.3 stack to staging with prod-shape configuration (Stripe test mode, SendGrid test mode, real CNPG sync standby)
- [ ] **D6.2** Run synthetic traffic (D2 E2E looped) for 10-14 days
- [ ] **D6.3** Watch P17 dashboards for anomalies — any alert firing > diagnosed root cause
- [ ] **D6.4** Prod-release go/no-go meeting based on soak results
- **Owner:** Platform + on-call
- **Duration:** 2 weeks (elapsed; human oversight time ~1 day total)
- **Acceptance:** Zero unplanned PagerDuty incidents over last 7 days of soak

---

## Workstream E — Ops readiness

**Owner:** Platform lead + founder.

### E1. On-call rotation

- [ ] **E1.1** Define on-call schedule (who, what hours). At 2-3 engineers total for v1, follow-the-sun not feasible; 24/7 pager on 1 person rotating weekly is reasonable
- [ ] **E1.2** Publish on-call calendar (Google Calendar + PagerDuty)
- [ ] **E1.3** Compensation + time-off policy for on-call
- [ ] **E1.4** Escalation policy: L1 on-call → L2 (CEO/CTO) after 15 min no-ack
- **Owner:** Founder
- **Duration:** 1 week

### E2. Runbooks beyond P19 CNPG failover

- [ ] **E2.1** Stripe webhook orphan queue — how to triage `manual_review_required` events
- [ ] **E2.2** Geo-arbitrage appeal queue (P8) — billing-ops workflow: 5-biz-day SLA, resolution decision tree
- [ ] **E2.3** SEA tax-ID manual review queue (P7) — 5-biz-day SLA, 30/week capacity threshold alert handling
- [ ] **E2.4** Break-glass account use procedure — who can authorize, post-use rotation checklist, audit trail review
- [ ] **E2.5** Pro+App onboarding checklist — CSM workflow from contact form to live app
- [ ] **E2.6** Pro+App teardown checklist — CSM workflow day 0-90
- [ ] **E2.7** Refund approval workflow — who signs off on >14-day refunds (CSM escalation path)
- [ ] **E2.8** Incident response template
- **Owner:** Founder + CSM (future hire) + engineering
- **Duration:** 2 weeks
- **Acceptance:** All runbooks live at `docs/runbooks/` + linked from `docs/runbooks/README.md`

### E3. Team role definitions

- [ ] **E3.1** Define `billing-ops` team scope — appeal review, refund approval, tax-ID review queue
- [ ] **E3.2** Define CSM scope — Pro contact-sales, Pro+App onboarding, teardown, migration fast-path evidence review
- [ ] **E3.3** Define support scope — general merchant questions; explicit carve-out: support does NOT approve pricing exceptions (§4.1.3), DOES NOT process GDPR erasure directly (§15.4)
- [ ] **E3.4** Hire or assign people to each role — at v1 scale probably 1 person wearing multiple hats + founder backup
- **Owner:** Founder
- **Duration:** 2-4 weeks (if hiring)

### E4. Customer communication plan

- [ ] **E4.1** Launch announcement email to existing beta/waitlist
- [ ] **E4.2** Pricing communication — how do existing early users transition? (grandfathering decision in WS-F2)
- [ ] **E4.3** Status page (statuspage.io or similar) live at `status.mark8ly.com`
- **Owner:** Marketing + founder
- **Duration:** 1-2 weeks

---

## Workstream F — Product decisions (gating)

**Owner:** Founder. Decisions needed in weeks 1-2 to avoid reopening later workstreams.

### F1. SOC 2 Type I timing

- [ ] **F1.1** Decide: triggered at first $100k+ deal OR kick off pre-launch 18-month program now
- [ ] **F1.2** If now: engage auditor (Vanta, Drata, or direct) — 18 months + $30-60k
- [ ] **F1.3** If later: document decision, plan for Q3 2026 re-evaluation
- **Decision owner:** Founder
- **Duration:** 2 days to decide

### F2. Grandfathered launch rate slots

- [ ] **F2.1** Decide: how many slots (50? 100?), what discount (20%? 50%?), for how long (first year? lifetime?)
- [ ] **F2.2** Communicate to existing beta merchants
- [ ] **F2.3** P10 promo infrastructure supports this via `grandfathered_price` override — coordinate with engineering to create the specific promo codes
- **Decision owner:** Founder
- **Duration:** 1 week

### F3. Pro+App "app-only $49/mo" continuity tier

- [ ] **F3.1** Decide: ship in v2 or never. Spec §26.3 flagged as open. v1 explicitly excludes it (§13.5 teardown does NOT offer this option).
- [ ] **F3.2** Document decision; keep placeholder removed from UI
- **Decision owner:** Founder
- **Duration:** 1 day (can defer to post-launch review)

### F4. Trial length iteration trigger

- [ ] **F4.1** Confirm the <30% Day-30 activation threshold for shortening trial to 30 days (§26.2)
- [ ] **F4.2** Set up P17 alert rule (already in plan Task 20) — verify firing threshold matches decision
- **Decision owner:** Founder + growth lead
- **Duration:** 1 day

### F5. Strategic-watch items (track, don't decide now)

- [ ] F5.1 Pro $99 contact-sales friction — revisit at 3-month post-launch review (§3.3)
- [ ] F5.2 Stripe AU acquiring rate for US/UK cards — revisit at $500k ARR
- [ ] F5.3 SendGrid → SES migration evaluation — trigger at 500 paid merchants (§10.3)
- [ ] F5.4 Split-currency USD-AU settlement — trigger at $200k ARR (§4.2.2)
- [ ] F5.5 White-label app build automation — required before 2nd-3rd Pro+App deal (§26.2)

---

## Workstream H — Dependency + vulnerability cleanup

**Owner:** Platform engineering. Budget: engineering time + possibly minor version bumps causing test churn.

**Context:** As of 2026-04-19 push, GitHub dependabot reports **24 vulnerabilities on `main`** — 3 critical, 13 high, 7 moderate, 1 low. Report at https://github.com/tesserix/mark8ly/security/dependabot.

### H1. Triage baseline audit

- [ ] **H1.1** Export full dependabot alert list:
  ```
  gh api repos/tesserix/mark8ly/dependabot/alerts --paginate \
    | jq -r '.[] | [.number, .security_advisory.severity, .dependency.package.ecosystem, .dependency.package.name, .security_advisory.cve_id, .dependency.manifest_path] | @tsv' \
    > docs/security/2026-04-19-dependabot-baseline.tsv
  ```
- [ ] **H1.2** Categorize each alert across two axes:
  - **Severity:** critical / high / moderate / low (GitHub's classification)
  - **Load-bearing:** yes = code path touches user input, crypto, auth, network I/O, or parses untrusted data; no = build-time, dev-only, or unreachable path
- [ ] **H1.3** Special attention to alerts under:
  - `services/marketplace-api/` (Go modules — most important given subscription work)
  - `apps/admin/` + `apps/onboarding/` + `apps/storefront/` (npm packages — XSS/prototype-pollution risk)
  - `cmd/billing-bootstrap/` + P2 Stripe client deps (billing surface)
- [ ] **H1.4** Output: `docs/security/2026-04-19-cve-triage.md` — table with (alert#, severity, package, advisory, our assessment, action: fix/waive/monitor)
- **Owner:** Platform
- **Duration:** 1-2 days
- **Acceptance:** Every one of 24 alerts has a documented action

### H2. Critical CVE remediation (3 alerts) — LAUNCH GATE

- [ ] **H2.1** For each critical: bump to patched version; run full integration test suite (`go test -tags=integration ./...` + admin Playwright)
- [ ] **H2.2** If no patched version available: assess viable mitigations (code-level workaround, disable affected feature, fork dependency with patch)
- [ ] **H2.3** If critical sits in a transitive dep: add direct pin via `go mod edit -require` or npm `overrides`
- [ ] **H2.4** Document each remediation commit's CVE number in commit message
- **Owner:** Platform
- **Duration:** 3-5 days (depends on breaking changes)
- **Acceptance:** `gh api repos/tesserix/mark8ly/dependabot/alerts --jq '[.[] | select(.state=="open" and .security_advisory.severity=="critical")] | length'` returns `0`

### H3. High CVE remediation (13 alerts) — LAUNCH GATE

- [ ] **H3.1** Remediate in batches — critical-path packages first (Stripe client deps, auth libs, JSON parsers, HTML sanitizers)
- [ ] **H3.2** Each remediation: bump → `go build ./...` + `npm run build` → unit tests → integration tests → commit separately per package for traceability
- [ ] **H3.3** Any high-severity alert marked `fix = waive` in triage MUST have a written justification in `docs/security/waivers/YYYY-MM-DD-<package>.md`. Waivers require CEO + platform-lead sign-off.
- [ ] **H3.4** Common high-severity Go dep suspects (from industry pattern — verify against actual list): `golang.org/x/net`, `golang.org/x/crypto`, `gopkg.in/yaml.v3`, TLS-related libs. npm side: `next`, `react`, `@types/*` aren't usually load-bearing but `dompurify`, `axios`, `zod` can be.
- **Owner:** Platform
- **Duration:** 1 week
- **Acceptance:** Zero open high-severity alerts OR every open alert has a filed waiver

### H4. Moderate + Low (8 alerts total) — target but not blocking

- [ ] **H4.1** Fix trivial moderate/low CVEs opportunistically during H2/H3 work (often a single `go mod tidy` clears several)
- [ ] **H4.2** Remaining moderate/low schedule to post-launch backlog with a 30-day SLA
- **Owner:** Platform
- **Duration:** ongoing
- **Acceptance:** Moderate alerts <5 at launch; all with post-launch SLA dates

### H5. Continuous dependency hygiene (ongoing)

- [ ] **H5.1** Enable dependabot version-update PRs on `main` (Go modules + npm workspaces); group minor+patch into weekly batches to reduce noise
- [ ] **H5.2** `.github/dependabot.yml` configuration committed to repo with:
  - Daily security updates (auto-open PR)
  - Weekly version updates (auto-open PR, grouped)
  - Ignore major-version bumps (require human review)
- [ ] **H5.3** Add CI job: `go list -json -m all | nancy sleuth` OR `osv-scanner --lockfile=go.sum` on every PR; same for npm via `npm audit --audit-level=high` — fail the build on new high/critical
- [ ] **H5.4** Renovate alternative considered and rejected/accepted — document decision
- **Owner:** Platform
- **Duration:** 3-5 days setup; ongoing maintenance
- **Acceptance:** CI blocks PRs introducing new high/critical; dependabot PRs auto-opened on schedule

### H6. Launch gate on zero criticals/highs

- [ ] **H6.1** Day-of-launch: verify dependabot badge shows 0 critical, 0 high on main
- [ ] **H6.2** Record snapshot in `docs/security/launch-day-cve-snapshot.md`
- **Owner:** Platform + CEO sign-off
- **Duration:** 30 min on launch day

---

## Workstream G — Post-launch calendar items

Not blocking; track via Google Calendar reminders or Linear recurring tasks.

| When | Item | Spec ref |
|---|---|---|
| +30 days | Trial activation Day-30 metric review; adjust length if <30% | §26.2 |
| +90 days | Pro contact-sales friction review — consider self-serve switch | §3.3 |
| 100 merchants/country (EU/UK) | Re-confirm tax counsel opinions | §20.4 |
| 500 paid merchants | Evaluate SendGrid → SES migration | §10.3 |
| First $100k+ deal | Kick off SOC 2 Type I (if not already started) | §20.4 |
| $200k ARR | Activate split-currency USD-AU settlement | §4.2.2 |
| $500k ARR | Evaluate Stripe US entity + additional acquiring | §26.2 |
| ₹20 lakh/mo India revenue | India GST OIDAR registration | §20.4 |
| 2nd-3rd Pro+App deal | Ship white-label app build automation | §26.2 |

---

## Go / no-go checklist (final launch gate)

Check these the day of public launch. Any `✗` = no-go, investigate.

**Legal**
- [ ] TOS published; legal has signed off
- [ ] Privacy Policy published; India grievance officer named
- [ ] DPA template ready for Pro+App
- [ ] NZ tax counsel opinion on file (registered OR reverse-charge confirmed)
- [ ] EU/UK/India tax counsel opinion on file
- [ ] Cyber liability $1M policy active

**Billing**
- [ ] Stripe AU prod verified
- [ ] 8 Price objects exist in prod Stripe with correct lookup_keys
- [ ] Webhook signing secret rotated; test event round-trips
- [ ] Stripe India SCA test passed
- [ ] AU Stripe Tax registration confirmed

**Security**
- [ ] All 8 D5 security regression tests green
- [ ] All Secret Manager paths have correct IAM per §18
- [ ] Break-glass accounts provisioned for Pro tenants (if any exist)
- [ ] HMAC IP key rotation job scheduled

**Verification**
- [ ] D2 full E2E green for 7 consecutive nightly runs
- [ ] D3 load tests pass thresholds
- [ ] D4 CNPG chaos drill RTO ≤ 2 min confirmed
- [ ] D6 staging soak 7 days zero unplanned incidents

**Ops**
- [ ] On-call rotation live; first-week schedule published
- [ ] PagerDuty routes verified (test alert rings phone)
- [ ] Slack channels provisioned; webhooks functional
- [ ] All 8 E2 runbooks published
- [ ] Status page live

**Content**
- [ ] All 31 C1 email templates reviewed, test-sent
- [ ] C2 FAQ content live
- [ ] C3 Pro sales brief downloadable
- [ ] C4 closed-store copy reviewed
- [ ] C5 admin microcopy reviewed

**Product**
- [ ] F1-F4 decisions documented
- [ ] Grandfathering promos created (if applicable)

**Code**
- [ ] All 19 plans merged to main
- [ ] Schema version = (final migration number)
- [ ] No open `manual_review_required` webhook events
- [ ] D1 test-coverage gaps closed

**Security (CVE)**
- [ ] Dependabot: 0 critical alerts on main
- [ ] Dependabot: 0 high alerts on main (or waivers filed for each)
- [ ] H5 continuous hygiene wired — CI fails PRs introducing new high/critical
- [ ] Launch-day CVE snapshot recorded at `docs/security/launch-day-cve-snapshot.md`

---

## First-7-days post-launch monitoring plan

Daily review of:
- MRR gauge across all currencies (P17 Subscription Health dashboard)
- Trial signup rate vs anomaly threshold (>50/day = investigate)
- Webhook success rate (target >99%)
- Failed-payment rate (target <2% of active subs in first month)
- Arbitrage flag count (expect near-zero baseline)
- Any PagerDuty page → post-mortem within 48h

Calendar reminders:
- Day 3: First weekly metrics digest to founder
- Day 7: First retro — what broke, what held
- Day 14: Decide if any hot-patches needed before more marketing push
- Day 30: Trial cohort #1 reaches day-30 activation milestone → first real data point

---

## Summary

**Blocker count at start state (2026-04-19):** 8 hard blockers, 4 code gaps, ~31 content items, ~8 testing items, 24 dependabot alerts (3 critical + 13 high on launch gate).

**Optimistic launch:** 6 weeks from today if NZ counsel says reverse-charge sufficient.
**Realistic launch:** 8 weeks — assumes NZ registration required + normal 4-8 week processing.
**Pessimistic launch:** 12 weeks — NZ + tax registry API delays + legal iteration rounds.

**Critical path owner:** founder (WS-A + WS-F — all others run in parallel).

**Review cadence:** weekly P20 status meeting tracking each workstream's task completion + blocked items; one hour on Mondays works at this scale.
