import { registerSubscriptionInactiveHandler } from '@/lib/api/client'

// TODO: swap for `copy.subscription.inactiveRedirect` once
// `@/lib/copy/subscription` lands (P16 Task — copy file).
const INACTIVE_COPY =
  'Your subscription needs attention. Opening billing\u2026'

interface Router {
  push: (url: string) => void
}

interface Toast {
  error: (message: string) => void
}

/**
 * Registers a module-level handler on the default apiClient that fires
 * whenever a 402 subscription_inactive response is received.
 *
 * Call once at app init (e.g. inside a layout Client Component):
 *
 *   const router = useRouter()
 *   attachBillingRedirectOnInactive(router, toast)
 */
export function attachBillingRedirectOnInactive(
  router: Router,
  toast?: Toast,
): void {
  registerSubscriptionInactiveHandler((_status: string) => {
    toast?.error(INACTIVE_COPY)
    router.push('/admin/settings/billing')
  })
}
