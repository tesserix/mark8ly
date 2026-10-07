import type { createApiClient } from "./client";
import {
  artworkLinkSchema,
  orderDetailSchema,
  orderListSchema,
  type ArtworkLink,
  type Order,
  type OrderDetail,
  type OrderListResponse,
} from "./schemas/orders";

export interface ListOrdersParams {
  status?: string;
  payment_status?: string;
  search?: string;
  page?: string;
  page_size?: string;
}

export interface ConfirmOrderBody {
  /** Optional payment-status change on confirm (e.g. "authorized" | "paid"). */
  payment_status?: string;
  reason?: string;
}

export interface RefundOrderBody {
  /** Omit for a full remaining-balance refund. */
  amount?: number;
  /**
   * REQUIRED idempotency scope — a stable id per refund attempt, reused on
   * retry (RefundOrderRequest.refund_request_id). Generate with `randomId()`.
   */
  refund_request_id: string;
  reason?: string;
}

/** POST .../invoice|receipt/email → `{sent, recipient}` (orders.go:663). */
export interface EmailDocumentResult {
  sent: boolean;
  recipient: string;
}

export function createOrdersApi(client: ReturnType<typeof createApiClient>) {
  return {
    list: (params?: ListOrdersParams) =>
      client.get<OrderListResponse>("/orders", params as Record<string, string>, orderListSchema),
    // get/confirm/fulfill/cancel all return a BARE AdminOrderResponse
    // (items[]/addresses[], tax_lines only on get). refund returns a different
    // RefundOrderResponse, so it stays unschema'd.
    get: (id: string) =>
      client.get<OrderDetail>(`/orders/${id}`, undefined, orderDetailSchema),
    /** ConfirmOrderRequest: optional payment_status + reason. */
    confirm: (id: string, body?: ConfirmOrderBody) =>
      client.post<OrderDetail>(`/orders/${id}/confirm`, body ?? {}, orderDetailSchema),
    /** MarkFulfilled ignores the body — the old tracking_number was a no-op. */
    fulfill: (id: string) =>
      client.post<OrderDetail>(`/orders/${id}/fulfill`, {}, orderDetailSchema),
    /** CancelOrderRequest.reason is REQUIRED (binding) — omitting it is a 400. */
    cancel: (id: string, reason: string) =>
      client.post<OrderDetail>(`/orders/${id}/cancel`, { reason }, orderDetailSchema),
    /**
     * A signed, short-lived link to one buyer's artwork (mark8ly#969).
     *
     * Fetched on TAP, not with the order: the URL expires in ten minutes
     * (DownloadURLTTL), so one baked into a cached order payload would be
     * dead by the time a merchant scrolled to it.
     *
     * Every call is audited server-side as order.artwork.downloaded —
     * this is a merchant reading a customer's photograph, and the audit
     * trail is the point, not a side effect.
     */
    artworkLink: (orderId: string, personalisationId: string) =>
      client.get<ArtworkLink>(
        `/orders/${orderId}/personalisations/${personalisationId}/download`,
        undefined,
        artworkLinkSchema,
      ),
    /** refund_request_id is REQUIRED; amount omitted ⇒ full remaining balance. */
    refund: (id: string, body: RefundOrderBody) => client.post(`/orders/${id}/refund`, body),
    /**
     * Resend the invoice email. Optional `note` renders as a "Note from
     * {store}" block. Returns `{sent, recipient}`. 422 when the order has no
     * customer email on file (orders.go:651).
     */
    emailInvoice: (id: string, note?: string) =>
      client.post<EmailDocumentResult>(`/orders/${id}/invoice/email`, note ? { note } : {}),
    /**
     * Resend the receipt email. Gated on shipment delivery — a 409
     * ("not_delivered", orders.go:622) means no delivered shipment yet.
     */
    emailReceipt: (id: string, note?: string) =>
      client.post<EmailDocumentResult>(`/orders/${id}/receipt/email`, note ? { note } : {}),
  };
}

export type { Order, OrderDetail };
