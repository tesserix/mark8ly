import { describe, expect, it } from "vitest";

import { pricingCopy } from "@/lib/copy/pricing";

/**
 * Every CTA href the public pricing page renders.
 *
 * Collected from the copy module rather than listed, so a CTA added later is
 * covered without anyone remembering to add it here — the failure this guards
 * against is precisely a link nobody checked.
 */
const ALL_HREFS: ReadonlyArray<{ what: string; href: string }> = [
  { what: "proCtas.conversationHref", href: pricingCopy.proCtas.conversationHref },
  { what: "proApp.ctaHref", href: pricingCopy.proApp.ctaHref },
  ...pricingCopy.plans.map((p) => ({ what: `plans.${p.id}.ctaHref`, href: p.ctaHref })),
];

describe("pricing page CTA hrefs (#849)", () => {
  // THE bug this file exists for. `(admin)` is a Next.js route GROUP:
  // parentheses are excluded from the URL, so `/admin/settings/...` never
  // resolved and never could. Three CTAs carried it, and the same mistaken
  // assumption killed five e2e specs in #834.
  it.each(ALL_HREFS)("$what does not carry the (admin) route-group prefix", ({ href }) => {
    expect(href.startsWith("/admin/")).toBe(false);
    expect(href).not.toContain("/(admin)");
  });

  it.each(ALL_HREFS)("$what is not empty", ({ href }) => {
    expect(href.trim()).not.toBe("");
  });

  // Signup crosses a HOST boundary — this page is served from the admin host,
  // signup lives in apps/onboarding on the marketing host — so a relative
  // href would resolve against the wrong origin and 404. Asserted as a SHAPE
  // rather than a literal URL because the host comes from
  // NEXT_PUBLIC_MARKETING_URL and differs between dev, UAT and production;
  // pinning one string here would just move the breakage to another
  // environment.
  it.each(
    pricingCopy.plans.filter((p) => p.id === "starter" || p.id === "studio"),
  )("$id sends a prospect to an absolute signup URL", (plan) => {
    expect(plan.ctaHref).toMatch(/^https?:\/\//);
    expect(plan.ctaHref.endsWith("/onboarding")).toBe(true);
  });

  // `/signup` has never existed anywhere under apps/admin/app. If it comes
  // back, someone has reintroduced the 404 rather than pointed at onboarding.
  it.each(ALL_HREFS)("$what does not point at the non-existent /signup", ({ href }) => {
    expect(href).not.toContain("/signup");
  });

  // A query parameter the receiving app ignores is a URL promising something
  // the product does not do. apps/onboarding reads no `plan` param, and by
  // design cannot: the trial starts from an email alone and the plan is
  // chosen later.
  it.each(ALL_HREFS)("$what carries no ?plan= the signup flow ignores", ({ href }) => {
    expect(href).not.toContain("plan=");
  });

  // The in-app CTAs stay relative — they are for a merchant already signed in
  // on their own tenant subdomain, and an absolute admin host would send them
  // to a different tenant's admin.
  it("keeps the in-app Pro CTAs relative to the current tenant's admin host", () => {
    expect(pricingCopy.proCtas.conversationHref).toBe("/settings/billing/pro-contact");
    expect(pricingCopy.proApp.ctaHref).toBe("/settings/billing/pro-app-purchase");
  });

  // Removed in #849 because the asset does not exist and the URL 404s. If a
  // `briefHref` reappears, the PDF must exist too — this fails until whoever
  // adds it back also deletes this test, which forces the question.
  it("does not offer a brief download while no such asset is shipped", () => {
    expect("briefHref" in pricingCopy.proCtas).toBe(false);
    expect("brief" in pricingCopy.proCtas).toBe(false);
  });
});
