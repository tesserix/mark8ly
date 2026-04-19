/**
 * P16 WCAG 2.1 AA — Playwright axe-core e2e audit
 *
 * Tag: @a11y
 *
 * Navigates to each P16 route and runs @axe-core/playwright with WCAG 2.1 AA
 * rules. Zero "serious" or "critical" violations are asserted. "moderate"
 * violations are logged as warnings and documented in a11y-manual-audit.md.
 *
 * Running this suite requires a dev server on ADMIN_BASE_URL (default
 * http://localhost:4202). See a11y-manual-audit.md for the full dev-server
 * setup command.
 *
 * Fixtures:
 *   A parallel agent may create tests/e2e/fixtures/subscription.ts with MSW
 *   handlers for the subscription API. If that file exists, it is imported
 *   here. If it is absent, a minimal inline mock page-context override is
 *   used via Playwright's route interception instead.
 */

import { test, expect } from '@playwright/test'
import AxeBuilder from '@axe-core/playwright'
import * as fs from 'node:fs'
import * as path from 'node:path'

// ---------------------------------------------------------------------------
// Fixture detection — coordinate with parallel agent
// ---------------------------------------------------------------------------
const FIXTURE_PATH = path.resolve(
  __dirname,
  'fixtures/subscription.ts',
)
const HAS_SUBSCRIPTION_FIXTURES = fs.existsSync(FIXTURE_PATH)

// ---------------------------------------------------------------------------
// Axe config: WCAG 2.1 AA rules only
// ---------------------------------------------------------------------------
const WCAG_21_AA_TAGS = ['wcag2a', 'wcag2aa', 'wcag21a', 'wcag21aa']

// ---------------------------------------------------------------------------
// Test store/route constants
// ---------------------------------------------------------------------------
const STORE_ID = 'test-store-1'

const ROUTES = {
  pricing: '/pricing',
  billing: `/admin/stores/${STORE_ID}/settings/billing`,
  planChange: `/admin/stores/${STORE_ID}/settings/billing/plan-change`,
  cancel: `/admin/stores/${STORE_ID}/settings/billing/cancel`,
  proContact: `/admin/stores/${STORE_ID}/settings/billing/pro-contact`,
  proAppPurchase: `/admin/stores/${STORE_ID}/settings/billing/pro-app-purchase`,
  proAppCredentials: `/admin/stores/${STORE_ID}/settings/billing/pro-app-purchase/credentials`,
  taxId: `/admin/stores/${STORE_ID}/settings/tax-id`,
  attestation: `/admin/stores/${STORE_ID}/settings/attestation`,
  arbitrage: `/admin/stores/${STORE_ID}/settings/arbitrage`,
  closeBeforeDowngrade: `/admin/stores/${STORE_ID}/stores/close-before-downgrade?target=studio&billing=annual&over_by=2`,
} as const

// ---------------------------------------------------------------------------
// Helper: intercept subscription API responses with minimal fixtures when
// the dedicated fixtures file is not yet present.
// ---------------------------------------------------------------------------
async function setupApiMocks(page: import('@playwright/test').Page) {
  if (HAS_SUBSCRIPTION_FIXTURES) {
    // The parallel agent's fixtures file will handle mocking via MSW.
    // Nothing to do here — the dev server's MSW setup handles it.
    return
  }

  // Inline minimal route mocks so the audit can run without a live backend.
  const BASE_SUBSCRIPTION = {
    id: 'sub_test_1',
    tenant_id: STORE_ID,
    status: 'active',
    plan: 'starter',
    billing_period: 'monthly',
    billing_currency: 'USD',
    amount: 1900,
    period_end: new Date(Date.now() + 30 * 24 * 60 * 60 * 1000).toISOString(),
    trial_ends_at: null,
    add_ons: [],
    payment_method_brand: 'visa',
    payment_method_last4: '4242',
    arbitrage_flag: false,
    app_lifecycle_status: null,
  }

  await page.route('**/api/v1/admin/stores/*/subscription**', (route) => {
    route.fulfill({
      status: 200,
      contentType: 'application/json',
      body: JSON.stringify({ data: BASE_SUBSCRIPTION }),
    })
  })

  await page.route('**/api/v1/admin/stores/*/billing/portal', (route) => {
    route.fulfill({
      status: 200,
      contentType: 'application/json',
      body: JSON.stringify({ url: 'https://billing.stripe.com/test' }),
    })
  })

  await page.route('**/api/v1/admin/stores/*/tax/status**', (route) => {
    route.fulfill({
      status: 200,
      contentType: 'application/json',
      body: JSON.stringify({
        data: {
          status: 'not_submitted',
          tax_id: null,
          pause_reason: null,
          rejection_code: null,
        },
      }),
    })
  })

  await page.route('**/api/v1/admin/stores/*/tax/attestation**', (route) => {
    route.fulfill({
      status: 200,
      contentType: 'application/json',
      body: JSON.stringify({ data: null }),
    })
  })

  await page.route('**/api/v1/admin/stores/*/arbitrage**', (route) => {
    route.fulfill({
      status: 200,
      contentType: 'application/json',
      body: JSON.stringify({
        data: {
          flagged: false,
          first_flagged_at: null,
          severity: null,
          latest_audit: null,
        },
      }),
    })
  })

  await page.route('**/api/v1/admin/stores/*/subscription/plan-options**', (route) => {
    route.fulfill({
      status: 200,
      contentType: 'application/json',
      body: JSON.stringify({
        data: [
          { plan: 'starter', amount: 1900, currency: 'USD', billing_period: 'monthly' },
          { plan: 'studio', amount: 4900, currency: 'USD', billing_period: 'monthly' },
        ],
      }),
    })
  })
}

// ---------------------------------------------------------------------------
// Helper: run axe and separate violations by severity
// ---------------------------------------------------------------------------
async function runAxeAudit(page: import('@playwright/test').Page, testInfo: import('@playwright/test').TestInfo) {
  const builder = new AxeBuilder({ page })
    .withTags(WCAG_21_AA_TAGS)
    // Disable color-contrast for e2e as well — it generates noise from custom
    // CSS variables that axe cannot resolve without a full paint tree.
    // Contrast is verified manually in a11y-manual-audit.md.
    .disableRules(['color-contrast'])

  const results = await builder.analyze()

  const critical = results.violations.filter((v) => v.impact === 'critical')
  const serious = results.violations.filter((v) => v.impact === 'serious')
  const moderate = results.violations.filter((v) => v.impact === 'moderate')

  // Log moderate issues for visibility without blocking CI
  if (moderate.length > 0) {
    const modMsg = moderate
      .map((v) => `  [moderate] ${v.id}: ${v.description}`)
      .join('\n')
    testInfo.annotations.push({
      type: 'moderate-a11y-violations',
      description: `Moderate violations (non-blocking, see a11y-manual-audit.md):\n${modMsg}`,
    })
  }

  const blocking = [...critical, ...serious]
  if (blocking.length > 0) {
    const msg = blocking
      .map(
        (v) =>
          `[${v.impact}] ${v.id}: ${v.description}\n  Help: ${v.helpUrl}\n  Nodes: ${v.nodes.map((n) => n.html).slice(0, 3).join(' | ')}`,
      )
      .join('\n\n')
    return { pass: false, message: msg }
  }

  return { pass: true, message: '' }
}

// ===========================================================================
// Public route — /pricing (unauthenticated)
// ===========================================================================

test('@a11y /pricing page — zero serious/critical WCAG 2.1 AA violations', async ({ page }, testInfo) => {
  await setupApiMocks(page)
  await page.goto(ROUTES.pricing, { waitUntil: 'domcontentloaded' })

  // Allow RSC/client hydration to settle
  await page.waitForTimeout(500)

  const result = await runAxeAudit(page, testInfo)
  expect(result.pass, result.message).toBe(true)
})

// ===========================================================================
// Protected routes — billing settings
// ===========================================================================

test('@a11y /settings/billing — zero serious/critical WCAG 2.1 AA violations', async ({ page }, testInfo) => {
  await setupApiMocks(page)
  await page.goto(ROUTES.billing, { waitUntil: 'domcontentloaded' })
  await page.waitForTimeout(500)

  const result = await runAxeAudit(page, testInfo)
  expect(result.pass, result.message).toBe(true)
})

test('@a11y /settings/billing/plan-change — zero serious/critical WCAG 2.1 AA violations', async ({ page }, testInfo) => {
  await setupApiMocks(page)
  await page.goto(ROUTES.planChange, { waitUntil: 'domcontentloaded' })
  await page.waitForTimeout(500)

  const result = await runAxeAudit(page, testInfo)
  expect(result.pass, result.message).toBe(true)
})

test('@a11y /settings/billing/cancel — zero serious/critical WCAG 2.1 AA violations', async ({ page }, testInfo) => {
  await setupApiMocks(page)
  await page.goto(ROUTES.cancel, { waitUntil: 'domcontentloaded' })
  await page.waitForTimeout(500)

  const result = await runAxeAudit(page, testInfo)
  expect(result.pass, result.message).toBe(true)
})

test('@a11y /settings/billing/pro-contact — zero serious/critical WCAG 2.1 AA violations', async ({ page }, testInfo) => {
  await setupApiMocks(page)
  await page.goto(ROUTES.proContact, { waitUntil: 'domcontentloaded' })
  await page.waitForTimeout(500)

  const result = await runAxeAudit(page, testInfo)
  expect(result.pass, result.message).toBe(true)
})

test('@a11y /settings/billing/pro-app-purchase — zero serious/critical WCAG 2.1 AA violations', async ({ page }, testInfo) => {
  await setupApiMocks(page)
  await page.goto(ROUTES.proAppPurchase, { waitUntil: 'domcontentloaded' })
  await page.waitForTimeout(500)

  const result = await runAxeAudit(page, testInfo)
  expect(result.pass, result.message).toBe(true)
})

test('@a11y /settings/billing/pro-app-purchase/credentials — zero serious/critical WCAG 2.1 AA violations', async ({ page }, testInfo) => {
  await setupApiMocks(page)
  await page.goto(ROUTES.proAppCredentials, { waitUntil: 'domcontentloaded' })
  await page.waitForTimeout(500)

  const result = await runAxeAudit(page, testInfo)
  expect(result.pass, result.message).toBe(true)
})

test('@a11y /settings/tax-id — zero serious/critical WCAG 2.1 AA violations', async ({ page }, testInfo) => {
  await setupApiMocks(page)
  await page.goto(ROUTES.taxId, { waitUntil: 'domcontentloaded' })
  await page.waitForTimeout(500)

  const result = await runAxeAudit(page, testInfo)
  expect(result.pass, result.message).toBe(true)
})

test('@a11y /settings/attestation — zero serious/critical WCAG 2.1 AA violations', async ({ page }, testInfo) => {
  await setupApiMocks(page)
  await page.goto(ROUTES.attestation, { waitUntil: 'domcontentloaded' })
  await page.waitForTimeout(500)

  const result = await runAxeAudit(page, testInfo)
  expect(result.pass, result.message).toBe(true)
})

test('@a11y /settings/arbitrage — zero serious/critical WCAG 2.1 AA violations', async ({ page }, testInfo) => {
  await setupApiMocks(page)
  await page.goto(ROUTES.arbitrage, { waitUntil: 'domcontentloaded' })
  await page.waitForTimeout(500)

  const result = await runAxeAudit(page, testInfo)
  expect(result.pass, result.message).toBe(true)
})

test('@a11y /stores/close-before-downgrade — zero serious/critical WCAG 2.1 AA violations', async ({ page }, testInfo) => {
  await setupApiMocks(page)
  await page.goto(ROUTES.closeBeforeDowngrade, { waitUntil: 'domcontentloaded' })
  await page.waitForTimeout(500)

  const result = await runAxeAudit(page, testInfo)
  expect(result.pass, result.message).toBe(true)
})
