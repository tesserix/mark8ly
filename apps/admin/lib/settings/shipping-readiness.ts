import type { ShippingConfig } from "@/lib/api/settings-api";

/**
 * Why a store cannot quote shipping rates.
 *
 * Three separate misconfigurations all surface to the shopper as the same
 * thing — no delivery options, no explanation — and each was hit for real
 * on the-bondi-store before it could take an order:
 *
 *   • no carrier configured at all
 *   • a carrier configured but left inactive (marketplace-api filters
 *     `is_active = true`, shipping/repository.go:303)
 *   • a warehouse address with no phone (ShipEngine answers
 *     400 "'phone' should not be empty"; the field is `omitempty`, so a
 *     blank phone is dropped from the payload entirely)
 *   • no warehouse address, so there is no origin to quote from
 *
 * Each is individually reasonable and collectively a maze, because the
 * rate path can only report that nothing came back. This turns them into
 * one answer the merchant can act on.
 */
export interface ShippingBlocker {
  /** Stable key, for tests and for keying React lists. */
  code:
    | "no_carrier"
    | "inactive"
    | "no_warehouse_address"
    | "no_warehouse_phone";
  message: string;
}

/**
 * readinessFor returns every blocker for a single carrier, most
 * fundamental first. An empty array means the carrier can quote.
 *
 * Reports ALL blockers rather than stopping at the first: a merchant who
 * fixes the phone only to be told next about the inactive checkbox has
 * been sent round the loop twice for no reason.
 */
export function readinessFor(cfg: ShippingConfig | undefined): ShippingBlocker[] {
  if (!cfg) {
    return [
      {
        code: "no_carrier",
        message: "No credentials yet — add them to start quoting rates.",
      },
    ];
  }

  const blockers: ShippingBlocker[] = [];

  if (!cfg.enabled) {
    blockers.push({
      code: "inactive",
      message:
        "This carrier is inactive, so it is never quoted at checkout. Tick “Active” to enable it.",
    });
  }

  const hasAddress = Boolean(
    cfg.warehouse_line1 || cfg.warehouse_city || cfg.warehouse_postal,
  );
  if (!hasAddress) {
    blockers.push({
      code: "no_warehouse_address",
      message:
        "No warehouse address — carriers need an origin to quote a rate from.",
    });
  } else if (!cfg.warehouse_phone?.trim()) {
    // Only meaningful once there is an address to attach it to.
    blockers.push({
      code: "no_warehouse_phone",
      message:
        "The warehouse has no phone number. Carriers reject rate requests without one, so checkout shows no delivery options.",
    });
  }

  return blockers;
}
