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
 *   • the store has no warehouses at all (#177 PR 5d)
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
    | "no_warehouses"
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
export function readinessFor(
  cfg: ShippingConfig | undefined,
  /**
   * How many live warehouses the store has. Distinguishes "this carrier
   * is not linked to one" from "there are none to link to" — two very
   * different next actions, and telling a merchant with zero warehouses
   * to pick one sends them looking for a control that is not there.
   *
   * Required, deliberately. A default would let a call site that never
   * learned about warehouses keep compiling while reporting the wrong
   * blocker — the same shape of silent seam this slice exists to close.
   */
  warehouseCount: number,
): ShippingBlocker[] {
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

  // The warehouse_* fields below are still populated — since #484 the
  // backend resolves them from the warehouses row via warehouse_id rather
  // than from the carrier's own columns. So these reads stay correct after
  // #177 PR 5d; what changed is where the merchant FIXES them, which is
  // why the messages now point at the Warehouses page instead of implying
  // an address field on this card.
  const hasAddress = Boolean(
    cfg.warehouse_line1 || cfg.warehouse_city || cfg.warehouse_postal,
  );
  if (warehouseCount === 0) {
    blockers.push({
      code: "no_warehouses",
      message:
        "This store has no warehouses. Add one on the Warehouses page — carriers need an origin address to quote a rate from.",
    });
  } else if (!hasAddress) {
    blockers.push({
      code: "no_warehouse_address",
      message:
        "This carrier is not linked to a warehouse. Open it and choose which one it ships from.",
    });
  } else if (!cfg.warehouse_phone?.trim()) {
    // Only meaningful once there is an address to attach it to.
    blockers.push({
      code: "no_warehouse_phone",
      message:
        "The warehouse has no phone number. Carriers reject rate requests without one, so checkout shows no delivery options — add it on the Warehouses page.",
    });
  }

  return blockers;
}
