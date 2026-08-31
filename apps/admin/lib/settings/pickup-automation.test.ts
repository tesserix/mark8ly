import { describe, expect, it } from "vitest";

import {
  defaultAutoSchedulePickup,
  supportsPickupAutomation,
} from "./pickup-automation";

describe("supportsPickupAutomation", () => {
  it("is true only for carriers that implement SchedulePickup", () => {
    expect(supportsPickupAutomation("delhivery")).toBe(true);
    expect(supportsPickupAutomation("shipengine")).toBe(false);
    expect(supportsPickupAutomation("ninjavan")).toBe(false);
  });

  it("ignores casing and padding from the provider id", () => {
    expect(supportsPickupAutomation("Delhivery")).toBe(true);
    expect(supportsPickupAutomation(" delhivery ")).toBe(true);
  });
});

describe("defaultAutoSchedulePickup", () => {
  // The bug: this defaulted to true for every provider, so an AU store
  // adding ShipEngine saw a pre-ticked Delhivery pickup option.
  it("defaults off for a carrier without pickup automation", () => {
    expect(defaultAutoSchedulePickup("shipengine", undefined)).toBe(false);
  });

  it("defaults on for Delhivery", () => {
    expect(defaultAutoSchedulePickup("delhivery", undefined)).toBe(true);
  });

  it("always honours a saved value over the default", () => {
    expect(defaultAutoSchedulePickup("delhivery", false)).toBe(false);
    expect(defaultAutoSchedulePickup("shipengine", true)).toBe(true);
  });
});
