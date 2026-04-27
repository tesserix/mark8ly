import { describe, expect, it } from "vitest";

import {
  classifyAdminHost,
  isCanonicalAllowedPath,
} from "./host-policy";

describe("classifyAdminHost", () => {
  it("recognises the canonical admin host", () => {
    expect(classifyAdminHost("admin.mark8ly.com")).toEqual({ kind: "canonical" });
    expect(classifyAdminHost("admin.mark8ly.com:443")).toEqual({ kind: "canonical" });
    expect(classifyAdminHost("ADMIN.MARK8LY.COM")).toEqual({ kind: "canonical" });
  });

  it("recognises tenanted slug-admin subdomains", () => {
    expect(classifyAdminHost("playwrite-test-admin.mark8ly.com")).toEqual({
      kind: "slug",
      slug: "playwrite-test",
    });
    expect(classifyAdminHost("my-store-admin.mark8ly.dev")).toEqual({
      kind: "slug",
      slug: "my-store",
    });
  });

  it("recognises admin custom domains", () => {
    expect(classifyAdminHost("admin.acme.com")).toEqual({
      kind: "custom_admin",
      domain: "acme.com",
    });
    expect(classifyAdminHost("admin.shop.example.org")).toEqual({
      kind: "custom_admin",
      domain: "shop.example.org",
    });
  });

  it("treats localhost as canonical for dev iteration", () => {
    expect(classifyAdminHost("localhost")).toEqual({ kind: "canonical" });
    expect(classifyAdminHost("localhost:4202")).toEqual({ kind: "canonical" });
    expect(classifyAdminHost("127.0.0.1:4202")).toEqual({ kind: "canonical" });
  });

  it("rejects malformed slug subdomains — REGRESSION GUARD", () => {
    // Empty slug ("-admin.mark8ly.com") is malformed; the policy must
    // not trust it as a tenanted host.
    expect(classifyAdminHost("-admin.mark8ly.com")).toEqual({ kind: "unknown" });
  });

  it("rejects non-admin mark8ly subdomains", () => {
    expect(classifyAdminHost("api.mark8ly.com")).toEqual({ kind: "unknown" });
    expect(classifyAdminHost("www.mark8ly.com")).toEqual({ kind: "unknown" });
    expect(classifyAdminHost("playwrite-test.mark8ly.com")).toEqual({ kind: "unknown" });
  });

  it("rejects off-mark8ly hosts that aren't admin custom domains", () => {
    expect(classifyAdminHost("evil.com")).toEqual({ kind: "unknown" });
    expect(classifyAdminHost("shop.acme.com")).toEqual({ kind: "unknown" });
  });

  it("rejects empty / nonsense input", () => {
    expect(classifyAdminHost("")).toEqual({ kind: "unknown" });
    expect(classifyAdminHost(null)).toEqual({ kind: "unknown" });
    expect(classifyAdminHost(undefined)).toEqual({ kind: "unknown" });
    expect(classifyAdminHost("singlelabel")).toEqual({ kind: "unknown" });
  });
});

describe("isCanonicalAllowedPath", () => {
  it("allows auth + admin-shell utility paths", () => {
    expect(isCanonicalAllowedPath("/login")).toBe(true);
    expect(isCanonicalAllowedPath("/login/")).toBe(true);
    expect(isCanonicalAllowedPath("/logout")).toBe(true);
    expect(isCanonicalAllowedPath("/forgot-password")).toBe(true);
    expect(isCanonicalAllowedPath("/reset-password")).toBe(true);
    expect(isCanonicalAllowedPath("/accept-invite")).toBe(true);
    expect(isCanonicalAllowedPath("/accept-invite/abc123")).toBe(true);
    expect(isCanonicalAllowedPath("/pick-tenant")).toBe(true);
    expect(isCanonicalAllowedPath("/pricing")).toBe(true);
    expect(isCanonicalAllowedPath("/webhooks/stripe-billing")).toBe(true);
    expect(isCanonicalAllowedPath("/api/health")).toBe(true);
    expect(isCanonicalAllowedPath("/_next/static/chunks/main.js")).toBe(true);
  });

  it("rejects tenant-scoped paths — REGRESSION GUARD", () => {
    // These are exactly the paths that, if served on the canonical host,
    // would render tenant-scoped data on a URL whose bar doesn't match
    // the active store. If this test ever turns green for these paths,
    // the screenshot bug (admin.mark8ly.com/orders showing playwrite-test
    // orders) is back.
    expect(isCanonicalAllowedPath("/dashboard")).toBe(false);
    expect(isCanonicalAllowedPath("/orders")).toBe(false);
    expect(isCanonicalAllowedPath("/orders/abc-123")).toBe(false);
    expect(isCanonicalAllowedPath("/products")).toBe(false);
    expect(isCanonicalAllowedPath("/customers")).toBe(false);
    expect(isCanonicalAllowedPath("/marketing")).toBe(false);
    expect(isCanonicalAllowedPath("/settings")).toBe(false);
    expect(isCanonicalAllowedPath("/")).toBe(false);
  });

  it("doesn't substring-match between sibling segments", () => {
    // `/loginx` must not be allowed just because `/login` is. The
    // implementation must check exact match or `prefix + "/"`.
    expect(isCanonicalAllowedPath("/loginx")).toBe(false);
    expect(isCanonicalAllowedPath("/orders/dashboard")).toBe(false);
    expect(isCanonicalAllowedPath("/api/healthcheck")).toBe(false);
  });
});
