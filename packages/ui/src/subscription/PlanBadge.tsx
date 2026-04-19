/**
 * PlanBadge — editorial capsule for the tenant's current plan.
 *
 * Visual rules (Paper · Ink · Moss):
 *   - All plans: hairline border, paper-100 bg, ink text.
 *   - Pro only: moss accent border + moss text — the single accent per view.
 *   - Add-on suffix: middot separator, ink-600 text (secondary).
 *
 * Accessibility: role="status" so screen readers announce plan changes;
 * aria-label carries the full human-readable string including any add-on.
 */

export type PlanTier = 'trial' | 'starter' | 'studio' | 'pro'
export type PlanAddon = 'white_label_app'

export interface PlanBadgeProps {
  plan: PlanTier | (string & {})
  addon?: PlanAddon
  className?: string
}

const PLAN_LABELS: Record<string, string> = {
  trial: 'Trial',
  starter: 'Starter',
  studio: 'Studio',
  pro: 'Pro',
}

const ADDON_LABELS: Record<string, string> = {
  white_label_app: '+White-label App',
}

export function PlanBadge({ plan, addon, className = '' }: PlanBadgeProps) {
  const label = PLAN_LABELS[plan] ?? plan
  const addonLabel = addon ? ADDON_LABELS[addon] : null
  const isPro = plan === 'pro'

  const baseClasses = [
    'inline-flex items-center gap-1 text-[12px] font-medium tracking-wide uppercase',
    'border rounded-md px-2 py-0.5',
    'bg-[var(--paper-100)]',
  ].join(' ')

  const accentClasses = isPro
    ? 'border-[var(--moss-700)] text-[var(--moss-700)]'
    : 'border-[var(--hairline,var(--ink-100))] text-[var(--ink-900)]'

  const ariaLabel = addonLabel ? `${label} · ${addonLabel}` : label

  return (
    <span
      role="status"
      aria-label={ariaLabel}
      className={`${baseClasses} ${accentClasses} ${className}`.trim()}
    >
      <span>{label}</span>
      {addonLabel && (
        <>
          <span aria-hidden="true" className="text-[var(--ink-400)]">·</span>
          <span className="text-[var(--ink-600)]">{addonLabel}</span>
        </>
      )}
    </span>
  )
}

export default PlanBadge
