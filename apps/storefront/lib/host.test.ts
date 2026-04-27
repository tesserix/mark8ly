import { describe, it, expect } from "vitest";
import { sanitizeHost } from "./host";

describe("sanitizeHost", () => {
  it("strips port", () => {
    expect(sanitizeHost("store-a.mark8ly.com:443")).toBe("store-a.mark8ly.com");
  });
  it("returns null for empty / null / undefined", () => {
    expect(sanitizeHost("")).toBeNull();
    expect(sanitizeHost(null)).toBeNull();
    expect(sanitizeHost(undefined)).toBeNull();
  });
  it("rejects path characters", () => {
    expect(sanitizeHost("store-a.mark8ly.com/evil")).toBeNull();
    expect(sanitizeHost("store-a.mark8ly.com#evil")).toBeNull();
    expect(sanitizeHost("store-a.mark8ly.com?x=1")).toBeNull();
  });
  it("rejects double dots and edge dots", () => {
    expect(sanitizeHost("store-a..mark8ly.com")).toBeNull();
    expect(sanitizeHost(".mark8ly.com")).toBeNull();
    expect(sanitizeHost("mark8ly.com.")).toBeNull();
  });
  it("accepts standard mark8ly subdomain", () => {
    expect(sanitizeHost("store-a.mark8ly.com")).toBe("store-a.mark8ly.com");
  });
  it("accepts custom domain", () => {
    expect(sanitizeHost("shop.brand-a.com")).toBe("shop.brand-a.com");
  });
  it("accepts apex", () => {
    expect(sanitizeHost("mark8ly.com")).toBe("mark8ly.com");
  });
  it("rejects userinfo / @ / spaces", () => {
    expect(sanitizeHost("user@evil.com")).toBeNull();
    expect(sanitizeHost("evil.com\nfoo")).toBeNull();
    expect(sanitizeHost("a b.com")).toBeNull();
  });
  it("rejects raw IP literals", () => {
    // Per design, customer cookie is for hostnames only; reject IPs.
    expect(sanitizeHost("[::1]")).toBeNull();
  });
});
