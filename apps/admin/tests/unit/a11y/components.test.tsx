/**
 * P16 WCAG 2.1 AA Component Accessibility Audit
 *
 * Runs axe against every P16 component rendered with mocked hooks.
 * - Zero "serious" or "critical" violations are asserted.
 * - "moderate" violations are permitted with inline TODO comments.
 *
 * Tag: @a11y
 *
 * Notes on axe in jsdom:
 * - color-contrast is disabled — jsdom has no CSS paint engine so axe always
 *   reports false positives. Contrast pairs are verified in a11y-manual-audit.md.
 * - All other rules are enabled to surface real DOM-structure issues.
 */

import { describe, it, expect, vi, beforeEach } from 'vitest'
import { render } from '@testing-library/react'
import { axe, toHaveNoViolations } from 'jest-axe'
import { expect as jestExpect } from 'vitest'
import React from 'react'

// Extend vitest's expect with jest-axe matcher
jestExpect.extend(toHaveNoViolations)

// ---------------------------------------------------------------------------
// Shared axe config
// ---------------------------------------------------------------------------
const AXE_CONFIG = {
  rules: {
    // jsdom has no CSS paint engine — contrast checks yield false positives.
    // Manual contrast verification is in a11y-manual-audit.md.
    'color-contrast': { enabled: false },
  },
}

// ---------------------------------------------------------------------------
// Helper: assert zero serious or critical violations
// ---------------------------------------------------------------------------
async function assertNoSeriousCritical(container: HTMLElement) {
  const results = await axe(container, AXE_CONFIG)
  const blocking = results.violations.filter(
    (v) => v.impact === 'serious' || v.impact === 'critical',
  )
  if (blocking.length > 0) {
    const msg = blocking
      .map(
        (v) =>
          `[${v.impact}] ${v.id}: ${v.description}\n  ${v.nodes.map((n) => n.html).join('\n  ')}`,
      )
      .join('\n')
    throw new Error(`axe found blocking violations:\n${msg}`)
  }
}

// ===========================================================================
// Module-level mocks (must be hoisted before imports)
// ===========================================================================

// -- @tesserix/web: stub Radix Select as native <select> for jsdom.
//
// vi.mock factories are hoisted above all import statements by Vitest's
// transform, so top-level imports (including React) are NOT available inside
// the factory. We therefore avoid hooks entirely.
//
// axe only needs the label→input association to exist in the DOM; it does not
// care about React state. The real component wires:
//   <FieldLabel htmlFor="country">   — renders <label htmlFor="country">
//   <SelectTrigger id="country">     — renders the focusable element with id="country"
//
// Our stub makes SelectTrigger render <select id={id}> directly (no hooks,
// no context) and makes Select a plain pass-through wrapper. The <option>
// children come from SelectValue/SelectContent/SelectItem.
vi.mock('@tesserix/web', () => ({
  // Pass-through wrapper — RHF Controller's onValueChange reaches SelectTrigger
  // via the children prop chain; we don't need to intercept it for axe checks.
  Select: ({ children }: { children?: unknown }) => children,

  // Renders the actual <select id={id}> so <label htmlFor> associates correctly
  SelectTrigger: ({
    id,
    children,
    'aria-invalid': ariaInvalid,
    'aria-describedby': ariaDescribedBy,
  }: {
    id?: string
    children?: unknown
    'aria-invalid'?: boolean | 'true' | 'false'
    'aria-describedby'?: string
  }) => (
    <select
      id={id}
      defaultValue=""
      aria-invalid={ariaInvalid as boolean | undefined}
      aria-describedby={ariaDescribedBy}
      onChange={() => undefined}
    >
      {children as React.ReactNode}
    </select>
  ),

  SelectValue: ({ placeholder }: { placeholder?: string }) => (
    <option value="" disabled>{placeholder ?? 'Select'}</option>
  ),

  SelectContent: ({ children }: { children?: unknown }) => children,

  SelectItem: ({ value, children }: { value: string; children?: unknown }) => (
    <option value={value}>{children as React.ReactNode}</option>
  ),
}))

// -- Navigation
vi.mock('next/navigation', () => ({
  useRouter: vi.fn(() => ({ push: vi.fn(), replace: vi.fn(), back: vi.fn() })),
  usePathname: vi.fn(() => '/'),
  useSearchParams: vi.fn(() => new URLSearchParams()),
  useParams: vi.fn(() => ({})),
}))

// -- next/link passthrough
vi.mock('next/link', () => ({
  default: ({ href, children, ...rest }: React.AnchorHTMLAttributes<HTMLAnchorElement> & { href: string; children?: React.ReactNode }) => (
    <a href={href} {...rest}>{children}</a>
  ),
}))

// -- Shared toast
vi.mock('@/components/feedback/Toaster', () => ({
  useToast: vi.fn(() => ({
    toast: { success: vi.fn(), error: vi.fn(), info: vi.fn() },
  })),
}))

// -- Billing hooks
const mockOpenPortalMutate = vi.fn()
vi.mock('@/lib/api/subscription/hooks/useBilling', () => ({
  useCurrentPlan: vi.fn(() => ({
    plan: { plan: 'starter', status: 'active', periodEnd: '2026-05-19T00:00:00Z', trialEndsAt: null, billingCurrency: 'USD', billingPeriod: 'monthly', addOns: [], paymentMethodBrand: 'visa', paymentMethodLast4: '4242', amount: 1900 },
    isLoading: false,
  })),
  useOpenPortal: vi.fn(() => ({ mutate: mockOpenPortalMutate, isPending: false })),
}))

// -- Trial hooks
const mockUseTrialStatus = vi.fn()
vi.mock('@/lib/api/subscription/hooks/useTrial', () => ({
  useTrialStatus: (...args: unknown[]) => mockUseTrialStatus(...args),
}))

// -- Dunning hooks
const mockUsePastDueState = vi.fn()
const mockUsePaymentActionRequiredState = vi.fn()
vi.mock('@/lib/api/subscription/hooks/useDunning', () => ({
  usePastDueState: (...args: unknown[]) => mockUsePastDueState(...args),
  usePaymentActionRequiredState: (...args: unknown[]) => mockUsePaymentActionRequiredState(...args),
  useCompleteActionUrl: vi.fn(() => ({ mutate: vi.fn() })),
}))

// -- Arbitrage hooks
const mockUseArbitrageFlag = vi.fn()
vi.mock('@/lib/api/subscription/hooks/useArbitrage', () => ({
  useArbitrageFlag: (...args: unknown[]) => mockUseArbitrageFlag(...args),
  useSubmitArbitrageAppeal: vi.fn(() => ({ mutate: vi.fn(), isPending: false, isSuccess: false })),
}))

// -- Cancellation hooks
vi.mock('@/lib/api/subscription/hooks/useCancellation', () => ({
  useRevertCancellation: vi.fn(() => ({ mutate: vi.fn(), isPending: false, isError: false })),
  useCancelSubscription: vi.fn(() => ({ mutate: vi.fn(), isPending: false })),
}))

// -- Pro contact hooks
vi.mock('@/lib/api/subscription/hooks/useProContact', () => ({
  useSubmitProContact: vi.fn(() => ({ mutate: vi.fn(), isPending: false })),
}))

// -- Tax ID hooks
vi.mock('@/lib/api/subscription/hooks/useTaxId', () => ({
  useTaxIdStatus: vi.fn(() => ({
    status: 'not_submitted',
    taxId: null,
    pauseReason: null,
    rejectionCode: null,
    isLoading: false,
  })),
  useSubmitTaxId: vi.fn(() => ({ mutate: vi.fn(), isPending: false })),
}))

// -- Attestation hooks
vi.mock('@/lib/api/subscription/hooks/useAttestation', () => ({
  useAttestation: vi.fn(() => ({ attestation: null, isLoading: false, isSigned: false })),
  useSignAttestation: vi.fn(() => ({ mutate: vi.fn(), isPending: false })),
}))

// -- App credentials hooks
vi.mock('@/lib/api/subscription/hooks/useAppCredentials', () => ({
  useUploadAppleCredentials: vi.fn(() => ({ mutate: vi.fn(), isPending: false, isSuccess: false })),
  useUploadGoogleCredentials: vi.fn(() => ({ mutate: vi.fn(), isPending: false, isSuccess: false })),
}))

// -- Plan change hooks
vi.mock('@/lib/api/subscription/hooks/usePlanChange', () => ({
  useProrationPreview: vi.fn(() => ({ data: null, isLoading: false })),
  useApplyPlanChange: vi.fn(() => ({ mutate: vi.fn(), isPending: false })),
}))

// -- Pro app hooks
vi.mock('@/lib/api/subscription/hooks/useProApp', () => ({
  usePurchaseProApp: vi.fn(() => ({ mutate: vi.fn(), isPending: false })),
}))

// -- WhiteLabelAppLifecycleWidget (complex widget, stub it)
vi.mock('@/components/settings/billing/WhiteLabelAppLifecycleWidget', () => ({
  WhiteLabelAppLifecycleWidget: () => <div data-testid="wl-widget">App lifecycle widget</div>,
}))

// ===========================================================================
// Imports — after mocks are registered
// ===========================================================================

import { BannerShell } from '@/components/shell/banners/BannerShell'
import { TrialBanner } from '@/components/shell/banners/TrialBanner'
import { FailedPaymentBanner } from '@/components/shell/banners/FailedPaymentBanner'
import { PaymentActionRequiredBanner } from '@/components/shell/banners/PaymentActionRequiredBanner'
import { ArbitrageBanner } from '@/components/shell/banners/ArbitrageBanner'

import { Money } from '@repo/ui/subscription'
import { PlanBadge } from '@repo/ui/subscription'
import { SubscriptionStatusBadge } from '@repo/ui/subscription'
import type { SubscriptionStatus } from '@repo/ui/subscription'
import type { TrialStatus } from '@/lib/api/subscription/hooks/useTrial'

import { PlanCard } from '@/app/(admin)/settings/billing/PlanCard'
import { InvoicesList } from '@/app/(admin)/settings/billing/InvoicesList'
import { PaymentMethodCard } from '@/app/(admin)/settings/billing/PaymentMethodCard'
import { WhiteLabelAppCard } from '@/app/(admin)/settings/billing/WhiteLabelAppCard'
import type { CurrentPlan } from '@/lib/api/subscription/schemas/billing'

import { ProContactForm } from '@/app/(admin)/settings/billing/pro-contact/ProContactForm'
import { TaxIdForm } from '@/app/(admin)/settings/tax-id/TaxIdForm'
import { AttestationForm } from '@/app/(admin)/settings/attestation/AttestationForm'
import { AppealForm } from '@/app/(admin)/settings/arbitrage/AppealForm'
import { CredentialUploadForm } from '@/app/(admin)/settings/billing/pro-app-purchase/CredentialUploadForm'
import { CancelWizard } from '@/app/(admin)/settings/billing/cancel/CancelWizard'
import { PlanChangeClient } from '@/app/(admin)/settings/billing/plan-change/PlanChangeClient'
import { ProAppPurchaseClient } from '@/app/(admin)/settings/billing/pro-app-purchase/ProAppPurchaseClient'

// ===========================================================================
// Shared fixtures
// ===========================================================================

const BASE_PLAN: CurrentPlan = {
  plan: 'starter',
  status: 'active',
  periodEnd: '2026-05-19T00:00:00Z',
  trialEndsAt: null,
  billingCurrency: 'USD',
  billingPeriod: 'monthly',
  addOns: [],
  paymentMethodBrand: 'visa',
  paymentMethodLast4: '4242',
  amount: 1900,
}

const TRIAL_BASE: TrialStatus = {
  isTrialing: true,
  daysSinceSignup: 60,
  daysUntilTrialEnd: 30,
  trialEndsAt: new Date('2026-06-18T00:00:00Z'),
  bannerVariant: 'day60',
}

// ===========================================================================
// Section 1 — BannerShell
// ===========================================================================

describe('@a11y BannerShell', () => {
  it('info tone — no serious/critical violations', async () => {
    const { container } = render(
      <BannerShell
        tone="info"
        heading="Your bank needs to confirm this payment"
        body="Review the invoice to complete authentication."
      />,
    )
    await assertNoSeriousCritical(container)
  })

  it('warning tone with button CTA — no serious/critical violations', async () => {
    const { container } = render(
      <BannerShell
        tone="warning"
        heading="Payment failed"
        body="We were unable to charge the card on file."
        cta={{ label: 'Add a new card', onClick: vi.fn() }}
      />,
    )
    await assertNoSeriousCritical(container)
  })

  it('danger tone with dismissible button — no serious/critical violations', async () => {
    const { container } = render(
      <BannerShell
        tone="danger"
        heading="Subscription expired"
        body="Your store has been closed."
        dismissible
        onDismiss={vi.fn()}
      />,
    )
    await assertNoSeriousCritical(container)
  })

  it('CTA as anchor (external link) — no serious/critical violations', async () => {
    const { container } = render(
      <BannerShell
        tone="info"
        heading="Action needed"
        body="Please complete bank authentication."
        cta={{
          label: 'Review invoice',
          href: 'https://stripe.com/inv/xxx',
          target: '_blank',
        }}
      />,
    )
    await assertNoSeriousCritical(container)
  })
})

// ===========================================================================
// Section 2 — TrialBanner
// ===========================================================================

describe('@a11y TrialBanner', () => {
  beforeEach(() => vi.clearAllMocks())

  it('day60 variant — no serious/critical violations', async () => {
    mockUseTrialStatus.mockReturnValue({ ...TRIAL_BASE, bannerVariant: 'day60' })
    const { container } = render(<TrialBanner storeId="store-1" />)
    await assertNoSeriousCritical(container)
  })

  it('day85 variant — no serious/critical violations', async () => {
    mockUseTrialStatus.mockReturnValue({
      ...TRIAL_BASE,
      bannerVariant: 'day85',
      daysSinceSignup: 85,
      daysUntilTrialEnd: 5,
    })
    const { container } = render(<TrialBanner storeId="store-1" />)
    await assertNoSeriousCritical(container)
  })

  it('none variant (returns null) — empty output has no violations', async () => {
    mockUseTrialStatus.mockReturnValue({
      ...TRIAL_BASE,
      bannerVariant: 'none',
      isTrialing: false,
    })
    const { container } = render(<TrialBanner storeId="store-1" />)
    const results = await axe(container, AXE_CONFIG)
    expect(results.violations).toHaveLength(0)
  })
})

// ===========================================================================
// Section 3 — FailedPaymentBanner
// ===========================================================================

describe('@a11y FailedPaymentBanner', () => {
  beforeEach(() => vi.clearAllMocks())

  it('past_due state — no serious/critical violations', async () => {
    mockUsePastDueState.mockReturnValue({
      isPastDue: true,
      retryAt: new Date('2026-04-25T00:00:00Z'),
    })
    const { container } = render(<FailedPaymentBanner storeId="store-1" />)
    await assertNoSeriousCritical(container)
  })

  it('not past_due (null render) — no violations', async () => {
    mockUsePastDueState.mockReturnValue({ isPastDue: false, retryAt: null })
    const { container } = render(<FailedPaymentBanner storeId="store-1" />)
    const results = await axe(container, AXE_CONFIG)
    expect(results.violations).toHaveLength(0)
  })
})

// ===========================================================================
// Section 4 — PaymentActionRequiredBanner
// ===========================================================================

describe('@a11y PaymentActionRequiredBanner', () => {
  beforeEach(() => vi.clearAllMocks())

  it('T-14 state with direct href — no serious/critical violations', async () => {
    mockUsePaymentActionRequiredState.mockReturnValue({
      isPending: true,
      hostedInvoiceUrl: 'https://stripe.com/inv/xxx',
      daysSinceFlagged: 0,
    })
    const { container } = render(<PaymentActionRequiredBanner storeId="store-1" />)
    await assertNoSeriousCritical(container)
  })

  it('T-7 state with button fallback — no serious/critical violations', async () => {
    mockUsePaymentActionRequiredState.mockReturnValue({
      isPending: true,
      hostedInvoiceUrl: null,
      daysSinceFlagged: 8,
    })
    const { container } = render(<PaymentActionRequiredBanner storeId="store-1" />)
    await assertNoSeriousCritical(container)
  })

  it('not pending (null render) — no violations', async () => {
    mockUsePaymentActionRequiredState.mockReturnValue({
      isPending: false,
      hostedInvoiceUrl: null,
      daysSinceFlagged: 0,
    })
    const { container } = render(<PaymentActionRequiredBanner storeId="store-1" />)
    const results = await axe(container, AXE_CONFIG)
    expect(results.violations).toHaveLength(0)
  })
})

// ===========================================================================
// Section 5 — ArbitrageBanner
// ===========================================================================

describe('@a11y ArbitrageBanner', () => {
  beforeEach(() => vi.clearAllMocks())

  it('flagged state — no serious/critical violations', async () => {
    mockUseArbitrageFlag.mockReturnValue({
      flagged: true,
      isLoading: false,
      firstFlaggedAt: new Date('2026-03-01T00:00:00Z'),
      severity: 'high',
      latestAudit: null,
    })
    const { container } = render(<ArbitrageBanner storeId="store-1" />)
    await assertNoSeriousCritical(container)
  })

  it('not flagged (null render) — no violations', async () => {
    mockUseArbitrageFlag.mockReturnValue({
      flagged: false,
      isLoading: false,
      firstFlaggedAt: null,
      severity: null,
      latestAudit: null,
    })
    const { container } = render(<ArbitrageBanner storeId="store-1" />)
    const results = await axe(container, AXE_CONFIG)
    expect(results.violations).toHaveLength(0)
  })
})

// ===========================================================================
// Section 6 — Primitives (@repo/ui)
// ===========================================================================

describe('@a11y Money', () => {
  it('USD — no serious/critical violations', async () => {
    const { container } = render(<Money amount={119900} currency="USD" />)
    await assertNoSeriousCritical(container)
  })

  it('INR — no serious/critical violations', async () => {
    const { container } = render(<Money amount={990000} currency="INR" />)
    await assertNoSeriousCritical(container)
  })

  it('CAD — no serious/critical violations', async () => {
    const { container } = render(<Money amount={149800} currency="CAD" />)
    await assertNoSeriousCritical(container)
  })
})

describe('@a11y PlanBadge', () => {
  const plans = ['trial', 'starter', 'studio', 'pro'] as const

  for (const plan of plans) {
    it(`plan=${plan} — no serious/critical violations`, async () => {
      const { container } = render(<PlanBadge plan={plan} />)
      await assertNoSeriousCritical(container)
    })
  }

  it('pro + white_label_app addon — no serious/critical violations', async () => {
    const { container } = render(<PlanBadge plan="pro" addon="white_label_app" />)
    await assertNoSeriousCritical(container)
  })
})

describe('@a11y SubscriptionStatusBadge', () => {
  const statuses: SubscriptionStatus[] = [
    'signup',
    'trialing',
    'active',
    'past_due',
    'payment_action_required',
    'cancel_scheduled',
    'expired',
    'store_closed',
    'pending_hard_delete',
  ]

  for (const status of statuses) {
    it(`status=${status} — no serious/critical violations`, async () => {
      const { container } = render(<SubscriptionStatusBadge status={status} />)
      await assertNoSeriousCritical(container)
    })
  }
})

// ===========================================================================
// Section 7 — Billing panels
// ===========================================================================

describe('@a11y PlanCard', () => {
  it('active starter plan — no serious/critical violations', async () => {
    const { container } = render(<PlanCard plan={BASE_PLAN} storeId="store-1" />)
    await assertNoSeriousCritical(container)
  })

  it('cancel_scheduled status — no serious/critical violations', async () => {
    const plan: CurrentPlan = { ...BASE_PLAN, status: 'cancel_scheduled' }
    const { container } = render(<PlanCard plan={plan} storeId="store-1" />)
    await assertNoSeriousCritical(container)
  })

  it('pro plan — no serious/critical violations', async () => {
    const plan: CurrentPlan = { ...BASE_PLAN, plan: 'pro', addOns: ['white_label_app'] }
    const { container } = render(<PlanCard plan={plan} storeId="store-1" />)
    await assertNoSeriousCritical(container)
  })
})

describe('@a11y InvoicesList', () => {
  it('default state — no serious/critical violations', async () => {
    const { container } = render(<InvoicesList storeId="store-1" />)
    await assertNoSeriousCritical(container)
  })
})

describe('@a11y PaymentMethodCard', () => {
  it('card on file — no serious/critical violations', async () => {
    const { container } = render(<PaymentMethodCard plan={BASE_PLAN} storeId="store-1" />)
    await assertNoSeriousCritical(container)
  })

  it('no card on file — no serious/critical violations', async () => {
    const noPm: CurrentPlan = {
      ...BASE_PLAN,
      paymentMethodBrand: null,
      paymentMethodLast4: null,
    }
    const { container } = render(<PaymentMethodCard plan={noPm} storeId="store-1" />)
    await assertNoSeriousCritical(container)
  })
})

describe('@a11y WhiteLabelAppCard', () => {
  it('non-pro plan (null render) — no violations', async () => {
    const { container } = render(<WhiteLabelAppCard plan={BASE_PLAN} storeId="store-1" />)
    const results = await axe(container, AXE_CONFIG)
    expect(results.violations).toHaveLength(0)
  })

  it('pro with white_label_app addon — no serious/critical violations', async () => {
    const proPlan: CurrentPlan = {
      ...BASE_PLAN,
      plan: 'pro',
      addOns: ['white_label_app'],
    }
    const { container } = render(<WhiteLabelAppCard plan={proPlan} storeId="store-1" />)
    await assertNoSeriousCritical(container)
  })

  it('pro without addon (upsell) — no serious/critical violations', async () => {
    const proNoAddon: CurrentPlan = { ...BASE_PLAN, plan: 'pro', addOns: [] }
    const { container } = render(<WhiteLabelAppCard plan={proNoAddon} storeId="store-1" />)
    await assertNoSeriousCritical(container)
  })
})

// ===========================================================================
// Section 8 — Forms
// ===========================================================================

describe('@a11y ProContactForm', () => {
  it('initial empty state — no serious/critical violations', async () => {
    const { container } = render(<ProContactForm storeId="store-1" />)
    await assertNoSeriousCritical(container)
  })
})

describe('@a11y TaxIdForm', () => {
  it('not_submitted state — no serious/critical violations', async () => {
    const { container } = render(<TaxIdForm storeId="store-1" />)
    await assertNoSeriousCritical(container)
  })
})

describe('@a11y AttestationForm', () => {
  it('unsigned state — no serious/critical violations', async () => {
    const { container } = render(<AttestationForm storeId="store-1" />)
    await assertNoSeriousCritical(container)
  })
})

describe('@a11y AppealForm', () => {
  it('idle state — no serious/critical violations', async () => {
    const { container } = render(<AppealForm storeId="store-1" />)
    await assertNoSeriousCritical(container)
  })
})

describe('@a11y CredentialUploadForm', () => {
  it('idle state — no serious/critical violations', async () => {
    const { container } = render(<CredentialUploadForm storeId="store-1" />)
    await assertNoSeriousCritical(container)
  })
})

describe('@a11y CancelWizard', () => {
  it('confirm step — no serious/critical violations', async () => {
    const { container } = render(<CancelWizard storeId="store-1" />)
    await assertNoSeriousCritical(container)
  })
})

describe('@a11y PlanChangeClient', () => {
  it('initial state — no serious/critical violations', async () => {
    const { container } = render(<PlanChangeClient storeId="store-1" />)
    await assertNoSeriousCritical(container)
  })
})

describe('@a11y ProAppPurchaseClient', () => {
  it('initial state — no serious/critical violations', async () => {
    const { container } = render(<ProAppPurchaseClient storeId="store-1" />)
    await assertNoSeriousCritical(container)
  })
})
