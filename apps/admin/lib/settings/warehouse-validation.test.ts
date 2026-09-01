import { describe, expect, it } from "vitest";

import { validateWarehouseAddress } from "./warehouse-validation";

const base = { line1: "1 Campbell Parade", city: "Bondi Beach", postal: "2026", phone: "" };

describe("validateWarehouseAddress", () => {
  // The exact production failure: address saved, phone left blank,
  // ShipEngine 400s every quote and the storefront shows nothing.
  it("rejects an address with no phone", () => {
    expect(validateWarehouseAddress(base)).toMatch(/phone/i);
  });

  it("treats a whitespace-only phone as missing", () => {
    expect(validateWarehouseAddress({ ...base, phone: "   " })).toMatch(/phone/i);
  });

  it("accepts an address with a phone", () => {
    expect(validateWarehouseAddress({ ...base, phone: "+61255500000" })).toBeNull();
  });

  // Saving credentials before the address is a legitimate half-step;
  // blocking it would be a worse trap than the one we are preventing.
  it("allows a completely empty address through", () => {
    expect(
      validateWarehouseAddress({ line1: "", city: "", postal: "", phone: "" }),
    ).toBeNull();
  });

  it("requires a phone as soon as any address field is filled", () => {
    expect(
      validateWarehouseAddress({ line1: "", city: "Bondi Beach", postal: "", phone: "" }),
    ).toMatch(/phone/i);
  });
});
