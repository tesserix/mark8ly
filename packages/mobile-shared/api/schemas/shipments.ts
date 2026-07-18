import { z } from "zod";

/**
 * Wire truth for the admin shipment endpoints — ShipmentResponse
 * (shipments.go:274-302), now exposed on the mobile group (mirror of the web
 * routes). The Bondi store has zero orders, so like orders this could not be
 * verified against a live payload; it is derived from the Go DTO, the only
 * truth available.
 *
 * Field notes traced to the struct tags:
 *  - `estimated_delivery`, `shipped_at`, `delivered_at`, `pickup_scheduled_for`
 *    are `*time.Time` + omitempty → ABSENT, not null → `.optional()`.
 *  - `pickup_request_id` is a `string` + omitempty → ABSENT when empty → `.optional()`.
 *  - the remaining strings (`provider`, `tracking_number`, `label_url`,
 *    `service`, `status`, `currency_code`) have NO omitempty → always present,
 *    possibly "" → `z.string()` (allow empty).
 */
export const shipmentSchema = z.object({
  id: z.string(),
  order_id: z.string(),
  provider: z.string(),
  provider_shipment_id: z.string(),
  tracking_number: z.string(),
  label_url: z.string(),
  service: z.string(),
  status: z.string(),
  currency_code: z.string(),
  estimated_delivery: z.string().optional(),
  shipped_at: z.string().optional(),
  delivered_at: z.string().optional(),
  pickup_request_id: z.string().optional(),
  pickup_scheduled_for: z.string().optional(),
  created_at: z.string(),
  /**
   * Cancel/return outcome fields (ShipmentResponse.CancelAction/CancelStatus/
   * CancelReason, shipments.go:359-361). Plain `string` + `omitempty` →
   * ABSENT (not null) for shipments that never had a cancel/return
   * attempted → `.optional()`, same convention as `pickup_request_id`
   * above. `cancel_action`/`cancel_status` are closed vocabularies
   * server-side ("cancel_forward"|"rto"|"reverse_pickup" and
   * "succeeded"|"failed"|"unsupported"|"requested") but modelled as bare
   * strings so an added value never trips the parse.
   */
  cancel_action: z.string().optional(),
  cancel_status: z.string().optional(),
  cancel_reason: z.string().optional(),
});
export type Shipment = z.infer<typeof shipmentSchema>;

/**
 * GET /orders/:id/shipments returns the bare ShipmentResponse OR JSON `null`
 * when the order has no shipment yet (shipments.go:1037/1043 — `c.JSON(200, nil)`).
 * A 200-with-null is deliberately NOT a 404, so it never trips the api client's
 * store-invalid reset (client.ts:119). Model it as nullable so the parse passes.
 */
export const shipmentOrNullSchema = shipmentSchema.nullable();

/**
 * Advance-status vocabulary accepted by PATCH .../shipments/:id/status
 * (shipments.go:917-922). "exception" is a terminal-ish error state; the
 * mobile UI surfaces the forward-progress trio and leaves exception to the
 * carrier tracking sync.
 */
export const SHIPMENT_ADVANCE_STATUSES = [
  "in_transit",
  "out_for_delivery",
  "delivered",
] as const;
export type ShipmentAdvanceStatus = (typeof SHIPMENT_ADVANCE_STATUSES)[number];
