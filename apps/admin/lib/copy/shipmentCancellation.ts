/**
 * Copy for shipment cancel/return outcomes — the status the backend
 * (or a merchant-triggered manual cancel) leaves on a shipment after a
 * full refund, an order cancel, or the "Cancel / return shipment" text
 * link. Also covers the pre-action disclosure shown in the order
 * cancel/refund review steps so a merchant knows a shipment consequence
 * is coming before they confirm.
 *
 * Voice matches cancellation.ts: calm, editorial, confident. Raw enum
 * values (cancel_forward, rto, reverse_pickup, succeeded, failed,
 * unsupported, requested) never reach the UI — every value routes
 * through the maps below.
 */

export const CANCEL_ACTION_LABELS: Record<string, string> = {
  cancel_forward: 'Pickup cancelled',
  rto: 'Return to origin requested',
  reverse_pickup: 'Return pickup requested',
}

export const CANCEL_STATUS_LABELS: Record<string, string> = {
  succeeded: 'Done',
  failed: 'Failed',
  unsupported: 'Needs manual action',
  requested: 'Requested',
}

function actionLabel(action?: string): string {
  if (!action) return 'Return'
  return CANCEL_ACTION_LABELS[action] ?? action
}

function statusLabel(status?: string): string {
  if (!status) return ''
  return CANCEL_STATUS_LABELS[status] ?? status
}

export const shipmentCancellationCopy = {
  detailRowLabel: 'Cancel / return status',
  manualActionLabel: 'Cancel / return shipment',
  manualActionPending: 'Cancelling…',
  toastSuccessTitle: 'Shipment cancellation requested',
  toastErrorTitle: "Couldn't cancel shipment",

  /**
   * Quiet confirmation text for the DetailRow — used when cancel_status
   * is "succeeded" or "requested" (nothing needs the merchant's
   * attention yet).
   */
  statusRowValue(action?: string, status?: string): string {
    const a = actionLabel(action)
    const s = statusLabel(status)
    return s ? `${a} — ${s}` : a
  },

  /**
   * Warning-banner text for cancel_status "failed" or "unsupported".
   * Always names the tracking number so the merchant has what they need
   * to call the carrier directly.
   */
  statusWarningMessage(params: {
    action?: string
    status?: string
    reason?: string
    trackingNumber?: string
  }): string {
    const { action, status, reason, trackingNumber } = params
    const a = actionLabel(action)
    const tracking = trackingNumber
      ? ` using tracking ${trackingNumber}`
      : ''
    if (status === 'unsupported') {
      const prefix = reason ? `${reason} — ` : ''
      return `${prefix}${a.toLowerCase()} isn't supported automatically — arrange the return manually with the carrier${tracking}.`
    }
    // failed
    const prefix = reason ? `${reason}. ` : ''
    return `${a} failed. ${prefix}Arrange this manually with the carrier${tracking}.`
  },

  /**
   * Pre-action disclosure shown in the order cancel/refund review
   * steps when the order has a shipment. Returns null for shipment
   * states that don't map to a known consequence (e.g. already
   * cancelled).
   */
  consequenceByShipmentStatus(shipmentStatus: string | null | undefined): string | null {
    if (!shipmentStatus) return null
    if (shipmentStatus === 'pending' || shipmentStatus === 'created') {
      return 'This will also cancel the courier pickup.'
    }
    if (shipmentStatus === 'in_transit' || shipmentStatus === 'out_for_delivery') {
      return 'This will also trigger a return-to-origin with the courier.'
    }
    if (shipmentStatus === 'delivered') {
      return 'This will also request a reverse pickup from the customer.'
    }
    return null
  },
} as const

export type ShipmentCancellationCopy = typeof shipmentCancellationCopy
