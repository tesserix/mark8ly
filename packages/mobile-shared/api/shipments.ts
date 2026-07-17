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
  };
}

export type { Shipment };
