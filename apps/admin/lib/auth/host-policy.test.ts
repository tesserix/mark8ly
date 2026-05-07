import { describe, expect, it } from "vitest";

import {
  classifyAdminHost,
  isCanonicalAllowedPath,
  isValidSlugReturnUrl,
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

  it("recognises the UAT canonical admin host", () => {
    // `admin-uat.mark8ly.com` is the UAT mirror of the prod
    // `admin.mark8ly.com` — same canonical role, different env.
    expect(classifyAdminHost("admin-uat.mark8ly.com")).toEqual({ kind: "canonical" });
    expect(classifyAdminHost("ADMIN-UAT.MARK8LY.COM")).toEqual({ kind: "canonical" });
  });

  it("recognises tenanted UAT slug-admin subdomains", () => {
    // `{slug}-admin-uat.mark8ly.com` is the UAT mirror of the prod
    // `{slug}-admin.mark8ly.com`. Slug returned is identical so
    // downstream tenant lookups don't have to be env-aware.
    expect(classifyAdminHost("acceptance-training-admin-uat.mark8ly.com")).toEqual({
      kind: "slug",
      slug: "acceptance-training",
    });
    expect(classifyAdminHost("playwrite-test-admin-uat.mark8ly.com")).toEqual({
      kind: "slug",
      slug: "playwrite-test",
    });
  });

  it("rejects malformed UAT slug subdomains", () => {
    // `-admin-uat.mark8ly.com` (empty slug) is malformed; same guard
    // as the prod equivalent.
    expect(classifyAdminHost("-admin-uat.mark8ly.com")).toEqual({ kind: "unknown" });
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

  it("rejects the bare root path so anonymous traffic 404s on canonical — REGRESSION GUARD", () => {
    // The canonical admin host isn't tenant-onboarded; the bare
    // admin.mark8ly.com URL must not be discoverable. The middleware
    // returns 404 for any anonymous request to a canonical path that
    // isn't on this allow-list. Concretely:
    //   admin.mark8ly.com/        → 404 (bare root)
    //   admin.mark8ly.com/login   → 200 (auth entry point)
    //   admin.mark8ly.com/orders  → 404 anon, redirect-to-slug authed
    expect(isCanonicalAllowedPath("/")).toBe(false);
    // Spot-check that the auth + invite + utility paths stay allowed
    // so anonymous users with a returnUrl can still reach them.
    expect(isCanonicalAllowedPath("/login")).toBe(true);
    expect(isCanonicalAllowedPath("/forgot-password")).toBe(true);
    expect(isCanonicalAllowedPath("/accept-invite/abc")).toBe(true);
    expect(isCanonicalAllowedPath("/pick-tenant")).toBe(true);
  });

  it("doesn't substring-match between sibling segments", () => {
    // `/loginx` must not be allowed just because `/login` is. The
    // implementation must check exact match or `prefix + "/"`.
    expect(isCanonicalAllowedPath("/loginx")).toBe(false);
    expect(isCanonicalAllowedPath("/orders/dashboard")).toBe(false);
    expect(isCanonicalAllowedPath("/api/healthcheck")).toBe(false);
  });
});

describe("isValidSlugReturnUrl", () => {
  it("accepts middleware-bounced returnUrls back to a slug-admin host", () => {
    expect(isValidSlugReturnUrl("https://playwrite-test-admin.mark8ly.com/")).toBe(true);
    expect(isValidSlugReturnUrl("https://playwrite-test-admin.mark8ly.com/orders")).toBe(true);
    expect(isValidSlugReturnUrl("https://my-store-admin.mark8ly.dev/dashboard")).toBe(true);
  });

  it("accepts returnUrls back to an admin custom domain", () => {
    expect(isValidSlugReturnUrl("https://admin.acme.com/")).toBe(true);
    expect(isValidSlugReturnUrl("https://admin.shop.example.org/orders")).toBe(true);
  });

  it("rejects null / empty / non-URL inputs — REGRESSION GUARD", () => {
    expect(isValidSlugReturnUrl(null)).toBe(false);
    expect(isValidSlugReturnUrl(undefined)).toBe(false);
    expect(isValidSlugReturnUrl("")).toBe(false);
    expect(isValidSlugReturnUrl("not a url")).toBe(false);
  });

  it("rejects http (non-https) returnUrls so we don't downgrade", () => {
    expect(isValidSlugReturnUrl("http://playwrite-test-admin.mark8ly.com/")).toBe(false);
  });

  it("rejects the canonical host itself — prevents self-recursion (REGRESSION GUARD)", () => {
    // If anyone ever crafts ?returnUrl=https://admin.mark8ly.com/ to
    // smuggle past the gate, the canonical host should NOT pass.
    expect(isValidSlugReturnUrl("https://admin.mark8ly.com/")).toBe(false);
    expect(isValidSlugReturnUrl("https://admin.mark8ly.com/login")).toBe(false);
  });

  it("rejects mark8ly subdomains that aren't slug-admin shaped", () => {
    expect(isValidSlugReturnUrl("https://www.mark8ly.com/")).toBe(false);
    expect(isValidSlugReturnUrl("https://api.mark8ly.com/")).toBe(false);
    expect(isValidSlugReturnUrl("https://playwrite-test.mark8ly.com/")).toBe(false); // storefront
  });

  it("rejects malformed slug-admin shapes", () => {
    // -admin.mark8ly.com (empty slug) — the regex requires a non-empty
    // slug starting+ending with alphanumeric.
    expect(isValidSlugReturnUrl("https://-admin.mark8ly.com/")).toBe(false);
    // admin-admin.mark8ly.com would actually match (slug=admin), which
    // is fine — slug-existence is checked downstream when the user
    // reaches the actual host. This test just pins the regex shape.
    expect(isValidSlugReturnUrl("https://admin-admin.mark8ly.com/")).toBe(true);
  });

  it("rejects javascript: / data: / file: URLs to prevent open-redirect abuse", () => {
    expect(isValidSlugReturnUrl("javascript:alert(1)")).toBe(false);
    expect(isValidSlugReturnUrl("data:text/html,<h1>hi</h1>")).toBe(false);
    expect(isValidSlugReturnUrl("file:///etc/passwd")).toBe(false);
  });

  it("rejects off-mark8ly hosts that aren't admin custom domains", () => {
    expect(isValidSlugReturnUrl("https://evil.com/")).toBe(false);
    expect(isValidSlugReturnUrl("https://shop.acme.com/")).toBe(false);
  });
});
