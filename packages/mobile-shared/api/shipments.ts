import type { createApiClient } from "./client";
import {
  shipmentSchema,
  shipmentOrNullSchema,
  type Shipment,
} from "./schemas/shipments";

/** POST .../shipments — CreateShipmentRequest (shipments.go:268). Both required. */
export interface CreateShipmentBody {
  provider: string;
  service: string;
}

/** PATCH .../shipments/:id/status — UpdateStatusRequest (shipments.go:912). */
export interface UpdateShipmentStatusBody {
  status: string;
  description?: string;
}

/** POST .../pickup/schedule — SchedulePickupRequest (shipments.go:747). Both optional. */
export interface SchedulePickupBody {
  date?: string;
  slot_start?: string;
}

/** POST .../label/email → `{ok, recipient}` (shipments.go:1309). */
export interface EmailLabelResult {
  ok: boolean;
  recipient: string;
}

/** DELETE .../shipments/:id → `{ok: true}` (shipments.go:1094). */
export interface DeleteShipmentResult {
  ok: boolean;
}

/**
 * POST .../shipments/:id/cancel → shipmentcancel.Outcome (executor.go:43-48).
 * The manual "Cancel / return shipment" button — no body. `status` is one of
 * "succeeded" | "failed" | "unsupported"; `reason` is only present on a
 * non-"succeeded" outcome (`omitempty`).
 */
export interface CancelShipmentResult {
  shipment_id: string;
  action: string;
  status: string;
  reason?: string;
}

/**
 * Admin shipment management for a single order. Mirrors the WEB routes
 * (routes.go:312-355), now exposed on the mobile group. `get` returns the bare
 * ShipmentResponse OR null; every mutation that touches a shipment returns the
 * bare (non-envelope) ShipmentResponse — validate `shipmentSchema` directly.
 * Label DOWNLOAD (a PDF stream) is intentionally NOT wrapped here: it is a
 * web-only file-download flow, same deferral as the invoice/receipt PDF.
 */
export function createShipmentsApi(client: ReturnType<typeof createApiClient>) {
  return {
    get: (orderId: string) =>
      client.get<Shipment | null>(
        `/orders/${orderId}/shipments`,
        undefined,
        shipmentOrNullSchema,
      ),
    create: (orderId: string, body: CreateShipmentBody) =>
      client.post<Shipment>(`/orders/${orderId}/shipments`, body, shipmentSchema),
    updateStatus: (orderId: string, shipmentId: string, body: UpdateShipmentStatusBody) =>
      client.patch<Shipment>(
        `/orders/${orderId}/shipments/${shipmentId}/status`,
        body,
        shipmentSchema,
      ),
    refreshTracking: (orderId: string, shipmentId: string) =>
      client.post<Shipment>(
        `/orders/${orderId}/shipments/${shipmentId}/tracking/refresh`,
        {},
        shipmentSchema,
      ),
    schedulePickup: (orderId: string, shipmentId: string, body?: SchedulePickupBody) =>
      client.post<Shipment>(
        `/orders/${orderId}/shipments/${shipmentId}/pickup/schedule`,
        body ?? {},
        shipmentSchema,
      ),
    emailLabel: (orderId: string, shipmentId: string, recipient: string) =>
      client.post<EmailLabelResult>(
        `/orders/${orderId}/shipments/${shipmentId}/label/email`,
        { recipient },
      ),
    remove: (orderId: string, shipmentId: string) =>
      client.delete<DeleteShipmentResult>(`/orders/${orderId}/shipments/${shipmentId}`),
    /**
     * Manual "Cancel / return shipment" — resolves the shipment's lifecycle
     * state and takes the matching carrier action (routes.go:360). Best-
     * effort server-side: a non-"succeeded" `status` is a normal response,
     * not a thrown ApiError — callers should check `status`/`reason`.
     */
    cancel: (orderId: string, shipmentId: string) =>
      client.post<CancelShipmentResult>(`/orders/${orderId}/shipments/${shipmentId}/cancel`, {}),
  };
}

export type { Shipment };
