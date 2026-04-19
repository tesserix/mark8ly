/**
 * ImageLimitGrandfatheredBadge — inline badge shown on product pages when
 * a product has more images than its current plan's cap allows.
 *
 * This is an informational badge, not a warning. The merchant's existing
 * images are preserved ("grandfathered") from their previous plan — they
 * can keep them but cannot add more until they are under the cap.
 *
 * Returns null when imageCount <= planLimit.
 *
 * Design: hairline capsule, ink-700 text, info tone (never warning/danger).
 * Tooltip on click/focus clarifies the grandfathering behaviour.
 *
 * TODO: Wire into the product media section in `components/products/media/`
 * or the product list row once a `planLimit` field is available from the
 * subscription context. The component is standalone until that wiring lands.
 */
'use client'

import { useState } from 'react'

// ---------------------------------------------------------------------------
// Props
// ---------------------------------------------------------------------------

interface ImageLimitGrandfatheredBadgeProps {
  productId: string
  imageCount: number
  planLimit: number
  className?: string
}

// ---------------------------------------------------------------------------
// Component
// ---------------------------------------------------------------------------

export function ImageLimitGrandfatheredBadge({
  productId: _productId,
  imageCount,
  planLimit,
  className = '',
}: ImageLimitGrandfatheredBadgeProps) {
  const [tooltipVisible, setTooltipVisible] = useState(false)

  if (imageCount <= planLimit) return null

  const label = `Grandfathered: ${imageCount} images (plan cap ${planLimit})`
  const tooltipText =
    'Kept from your previous plan. You can keep these images but cannot add more until you are under the cap.'

  return (
    <span className={`relative inline-flex items-center ${className}`}>
      <button
        type="button"
        aria-describedby={`grandfathered-tooltip-${_productId}`}
        onClick={() => setTooltipVisible((v) => !v)}
        onFocus={() => setTooltipVisible(true)}
        onBlur={() => setTooltipVisible(false)}
        className="inline-flex items-center rounded-full border border-[var(--hairline,var(--ink-100))] px-2.5 py-0.5 text-xs font-medium text-[var(--ink-700)] transition-colors hover:bg-[var(--paper-200)] focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-[var(--moss-700)]"
        data-testid="grandfathered-badge"
      >
        {label}
      </button>

      {tooltipVisible && (
        <span
          id={`grandfathered-tooltip-${_productId}`}
          role="tooltip"
          className="absolute bottom-full left-0 z-10 mb-1.5 w-64 rounded-[6px] border border-[var(--hairline,var(--ink-100))] bg-[var(--background-elevated,#fff)] px-3 py-2 text-xs leading-5 text-[var(--ink-700)] shadow-[var(--shadow-1)]"
          data-testid="grandfathered-tooltip"
        >
          {tooltipText}
        </span>
      )}
    </span>
  )
}
