import { describe, expect, it } from "vitest";

import { readinessFor } from "./shipping-readiness";
import type { ShippingConfig } from "@/lib/api/settings-api";

// Minimal shape — readinessFor only reads these fields.
const ready = {
  provider: "shipengine",
  enabled: true,
  mode: "test",
  warehouse_line1: "1 Campbell Parade",
  warehouse_city: "Bondi Beach",
  warehouse_postal: "2026",
  warehouse_phone: "+61255500000",
} as unknown as ShippingConfig;

// Most cases describe a store that HAS a warehouse; the zero case gets its
// own test below.
const codes = (cfg: ShippingConfig | undefined, warehouseCount = 1) =>
  readinessFor(cfg, warehouseCount).map((b) => b.code);

describe("readinessFor", () => {
  it("reports nothing when the carrier can quote", () => {
    expect(readinessFor(ready, 1)).toEqual([]);
  });

  it("reports a missing carrier", () => {
    expect(codes(undefined)).toEqual(["no_carrier"]);
  });

  // The exact production state: everything filled, Active left unticked,
  // storefront silently shows no delivery options.
  it("reports an inactive carrier", () => {
    expect(codes({ ...ready, enabled: false })).toEqual(["inactive"]);
  });

  // The exact ShipEngine 400 we hit on the-bondi-store.
  it("reports a warehouse with no phone", () => {
    expect(codes({ ...ready, warehouse_phone: "" })).toEqual([
      "no_warehouse_phone",
    ]);
  });

  it("treats a whitespace-only phone as missing", () => {
    expect(codes({ ...ready, warehouse_phone: "   " })).toEqual([
      "no_warehouse_phone",
    ]);
  });

  it("reports a carrier not linked to a warehouse, without nagging about the phone", () => {
    // A phone with no address to attach it to is not the useful advice.
    expect(
      codes({
        ...ready,
        warehouse_line1: "",
        warehouse_city: "",
        warehouse_postal: "",
        warehouse_phone: "",
      }),
    ).toEqual(["no_warehouse_address"]);
  });

  // "Link this carrier to a warehouse" is useless advice when there are
  // none — it sends the merchant looking for a control that is not there.
  it("distinguishes a store with no warehouses at all", () => {
    expect(
      codes(
        {
          ...ready,
          warehouse_line1: "",
          warehouse_city: "",
          warehouse_postal: "",
          warehouse_phone: "",
        },
        0,
      ),
    ).toEqual(["no_warehouses"]);
  });

  // Reporting only the first blocker would send the merchant round the
  // loop twice.
  it("reports every blocker at once", () => {
    expect(
      codes({ ...ready, enabled: false, warehouse_phone: "" }),
    ).toEqual(["inactive", "no_warehouse_phone"]);
  });
});
