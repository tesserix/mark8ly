/**
 * SubscriptionStatusBadge — editorial label for every subscription lifecycle state.
 *
 * Visual rules (Paper · Ink · Moss):
 *   - active / trialing: moss text — positive, living state.
 *   - signup / cancel_scheduled: ink-700 — neutral, transitional.
 *   - past_due / payment_action_required: warning (amber-bronze) border + text.
 *   - expired / store_closed / pending_hard_delete: danger (oxblood) border + text.
 *
 * One accent per view — moss is reserved for active/trialing only.
 * No shadows on the badge, hairline border only.
 */

export type SubscriptionStatus =
  | 'signup'
  | 'trialing'
  | 'active'
  | 'past_due'
  | 'payment_action_required'
  | 'cancel_scheduled'
  | 'expired'
  | 'store_closed'
  | 'pending_hard_delete'

export interface SubscriptionStatusBadgeProps {
  status: SubscriptionStatus
  className?: string
}

type BadgeVariant = 'success' | 'warning' | 'danger' | 'neutral'

interface StatusConfig {
  label: string
  variant: BadgeVariant
}

const STATUS_CONFIG: Record<SubscriptionStatus, StatusConfig> = {
  signup: { label: 'Signup', variant: 'neutral' },
  trialing: { label: 'Trialing', variant: 'neutral' },
  active: { label: 'Active', variant: 'success' },
  past_due: { label: 'Past due', variant: 'warning' },
  payment_action_required: { label: 'Action required', variant: 'warning' },
  cancel_scheduled: { label: 'Ending', variant: 'neutral' },
  expired: { label: 'Expired', variant: 'danger' },
  store_closed: { label: 'Store closed', variant: 'danger' },
  pending_hard_delete: { label: 'Pending deletion', variant: 'danger' },
}

const VARIANT_CLASSES: Record<BadgeVariant, string> = {
  success: 'border-[var(--moss-700)] text-[var(--moss-700)] bg-[var(--paper-100)] moss-accent',
  warning: 'border-[var(--warning)] text-[var(--warning)] bg-[var(--paper-100)]',
  danger: 'border-[var(--danger)] text-[var(--danger)] bg-[var(--paper-100)]',
  neutral: 'border-[var(--hairline,var(--ink-100))] text-[var(--ink-700)] bg-[var(--paper-100)]',
}

export function SubscriptionStatusBadge({
  status,
  className = '',
}: SubscriptionStatusBadgeProps) {
  const config = STATUS_CONFIG[status]
  const variantClasses = VARIANT_CLASSES[config.variant]

  return (
    <span
      data-testid="status-badge"
      data-variant={config.variant}
      role="status"
      aria-label={config.label}
      className={[
        'inline-flex items-center text-[12px] font-medium tracking-wide',
        'border rounded-md px-2 py-0.5',
        variantClasses,
        className,
      ]
        .filter(Boolean)
        .join(' ')
        .trim()}
    >
      {config.label}
    </span>
  )
}

export default SubscriptionStatusBadge
