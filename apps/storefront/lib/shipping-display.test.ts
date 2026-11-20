import { describe, expect, it } from "vitest";
import { rateEstimate, rateLabel, trackingHref } from "./shipping-display";

const carrier = { service: "two_day", estimated_days: 2 };

describe("rateLabel", () => {
  it("uses the seller's name for own rates", () => {
    expect(rateLabel({ service: "own:x", name: "Standard" })).toBe("Standard");
  });
  it("keeps the humanized carrier label", () => {
    expect(rateLabel(carrier)).toBe("2-Day Shipping");
    expect(rateLabel({ service: "fast_lane" })).toBe("Fast lane");
  });
});

describe("rateEstimate", () => {
  it("prefers the seller's estimate", () => {
    expect(rateEstimate({ ...carrier, estimate: "3–6 days" })).toBe("3–6 days");
  });
  it("falls back to estimated_days", () => {
    expect(rateEstimate(carrier)).toBe("Est. 2 business days");
    expect(rateEstimate({ estimated_days: 1 })).toBe("Est. 1 business day");
  });
  it("is null when there is nothing to show", () => {
    expect(rateEstimate({ estimated_days: 0 })).toBeNull();
    expect(rateEstimate({})).toBeNull();
  });
});

describe("trackingHref", () => {
  it("returns https urls only", () => {
    expect(trackingHref({ tracking_url: "https://c.in/t?awb=1" })).toBe("https://c.in/t?awb=1");
    expect(trackingHref({ tracking_url: "http://c.in/t" })).toBeNull();
    expect(trackingHref({ tracking_url: "javascript:alert(1)" })).toBeNull();
    expect(trackingHref({})).toBeNull();
    expect(trackingHref({ tracking_url: "" })).toBeNull();
  });
});
