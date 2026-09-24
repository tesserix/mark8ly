/**
 * usePortalCta — the billing-portal CTA shared by every admin shell banner.
 *
 * Exists because every banner CTA called .mutate() with no onError, and both
 * underlying mutations only act on SUCCESS -- useOpenPortal redirects,
 * useCompleteActionUrl opens a tab. A failed call therefore did nothing at
 * all: no redirect, no message, nothing a merchant would see. "Add a card"
 * looked like a dead button.
 *
 * That is how a real outage stayed unreported. Both billing-PAGE call sites
 * (PaymentMethodCard, InvoicesList) already passed onError and surfaced
 * "internal server error"; the banners silently ate the identical 500, so the
 * same broken call read as two different problems depending on where it was
 * clicked, and the banner version read as no problem at all.
 *
 * A CTA that cannot report its own failure is worse than one that fails
 * loudly: it costs the user the information that anything went wrong.
 */
'use client'

import { useOpenPortal } from '@/lib/api/subscription/hooks/useBilling'
import { useToast } from '@/components/feedback/Toaster'
import { subscriptionCopy } from '@/lib/copy/subscription'

export interface PortalCta {
  /** Runs the action, reporting failure via a toast. */
  open: () => void
  /** True while in flight — for disabling the control. */
  isPending: boolean
}

export function usePortalCta(storeId: string): PortalCta {
  const openPortal = useOpenPortal(storeId)
  const { toast } = useToast()

  return {
    isPending: openPortal.isPending,
    open: () =>
      openPortal.mutate(undefined, {
        // Per-call onError rather than a default on useOpenPortal: react-query
        // runs BOTH the hook-level and call-level handlers, so a default there
        // would double-toast on the billing page, which already passes one.
        onError: (err: Error) => {
          toast.error(subscriptionCopy.banners.portalError, err.message)
        },
      }),
  }
}
