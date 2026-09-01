import { describe, expect, it } from "vitest";

import { defaultCarrierActive } from "./carrier-active";

describe("defaultCarrierActive", () => {
  // The bug: a new config defaulted to false, so a fully-filled carrier
  // saved inactive and quoted nothing with no explanation.
  it("defaults a new config to active", () => {
    expect(defaultCarrierActive(undefined)).toBe(true);
  });

  // A merchant who deliberately switched a carrier off during an outage
  // must not have it silently switched back on when they reopen the form.
  it("keeps a deliberate off", () => {
    expect(defaultCarrierActive(false)).toBe(false);
  });

  it("keeps an explicit on", () => {
    expect(defaultCarrierActive(true)).toBe(true);
  });
});
