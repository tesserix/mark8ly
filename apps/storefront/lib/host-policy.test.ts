import { describe, expect, it } from "vitest";

import { classifyStorefrontHost } from "./host-policy";

describe("classifyStorefrontHost", () => {
  it("recognises tenant slug subdomains", () => {
    expect(classifyStorefrontHost("playwrite-test.mark8ly.com")).toEqual({
      kind: "slug",
      slug: "playwrite-test",
    });
    expect(classifyStorefrontHost("my-store.mark8ly.dev")).toEqual({
      kind: "slug",
      slug: "my-store",
    });
  });

  it("treats apex + reserved subdomains as marketing (not storefront)", () => {
    expect(classifyStorefrontHost("mark8ly.com")).toEqual({ kind: "marketing" });
    expect(classifyStorefrontHost("www.mark8ly.com")).toEqual({ kind: "marketing" });
    expect(classifyStorefrontHost("api.mark8ly.com")).toEqual({ kind: "marketing" });
    expect(classifyStorefrontHost("admin.mark8ly.com")).toEqual({ kind: "marketing" });
    expect(classifyStorefrontHost("identity.mark8ly.com")).toEqual({ kind: "marketing" });
  });

  it("recognises custom domains as needing API resolution", () => {
    expect(classifyStorefrontHost("shop.acme.com")).toEqual({
      kind: "custom",
      domain: "shop.acme.com",
    });
    expect(classifyStorefrontHost("primasyss.com")).toEqual({
      kind: "custom",
      domain: "primasyss.com",
    });
  });

  it("rejects admin-shaped hosts that hit storefront in error", () => {
    // {slug}-admin.mark8ly.com routed to the storefront pod is a config
    // drift symptom — don't render anything.
    expect(classifyStorefrontHost("playwrite-test-admin.mark8ly.com")).toEqual({
      kind: "unknown",
    });
  });

  it("rejects malformed input", () => {
    expect(classifyStorefrontHost("")).toEqual({ kind: "unknown" });
    expect(classifyStorefrontHost(null)).toEqual({ kind: "unknown" });
    expect(classifyStorefrontHost(undefined)).toEqual({ kind: "unknown" });
    expect(classifyStorefrontHost("singlelabel")).toEqual({ kind: "unknown" });
  });

  it("treats localhost as marketing for dev iteration — REGRESSION GUARD", () => {
    // Local dev points at localhost:port; the storefront page-level code
    // falls back to DEFAULT_STORE_SLUG. We don't want middleware to 404
    // here or `npm run dev` breaks.
    expect(classifyStorefrontHost("localhost")).toEqual({ kind: "marketing" });
    expect(classifyStorefrontHost("localhost:4203")).toEqual({ kind: "marketing" });
  });

  it("recognises the UAT slug subdomain pattern and returns the canonical slug", () => {
    // `{slug}-uat.mark8ly.com` is the UAT mirror of `{slug}.mark8ly.com`;
    // the slug returned is the same so downstream tenant lookups don't
    // need to know which env they're in.
    expect(classifyStorefrontHost("acceptance-training-uat.mark8ly.com")).toEqual({
      kind: "slug",
      slug: "acceptance-training",
    });
    expect(classifyStorefrontHost("playwrite-test-uat.mark8ly.com")).toEqual({
      kind: "slug",
      slug: "playwrite-test",
    });
  });

  it("treats UAT reserved subdomains as marketing (not storefront)", () => {
    // The bare UAT canonical hosts (uat, admin-uat, onboarding-uat,
    // uat-landing) are NOT tenant slugs — each has its own VirtualService
    // routing to a different service.
    expect(classifyStorefrontHost("uat.mark8ly.com")).toEqual({ kind: "marketing" });
    expect(classifyStorefrontHost("admin-uat.mark8ly.com")).toEqual({ kind: "marketing" });
    expect(classifyStorefrontHost("onboarding-uat.mark8ly.com")).toEqual({ kind: "marketing" });
    expect(classifyStorefrontHost("uat-landing.mark8ly.com")).toEqual({ kind: "marketing" });
  });

  it("rejects UAT admin-shaped hosts that hit storefront in error", () => {
    // {slug}-admin-uat.mark8ly.com is the UAT admin host; if a request
    // for it lands on the storefront pod it's misrouted — render nothing.
    expect(classifyStorefrontHost("acceptance-training-admin-uat.mark8ly.com")).toEqual({
      kind: "unknown",
    });
  });
});
