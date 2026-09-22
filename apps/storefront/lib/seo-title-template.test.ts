import { describe, expect, it } from "vitest";
import { readFileSync } from "node:fs";
import path from "node:path";

import { resolveTitleTemplate } from "./seo";

// #894: a merchant set an SEO title template and it applied nowhere. The
// root layout hardcoded `title: { template: "%s" }`, and because a
// `title.template` only affects a segment's CHILDREN, that pass-through
// always won. Every page computed the merchant's template inside its own
// generateMetadata and discarded it.
//
// The visible cost was larger than the dead setting: product pages titled
// as bare "Widget" instead of "Widget · Store Name", so the store name was
// missing from search results and browser tabs on every product page.

describe("resolveTitleTemplate", () => {
  it("uses the merchant's template when it contains %s", () => {
    expect(
      resolveTitleTemplate("Acme", { seo_title_template: "%s — Acme Goods" } as never),
    ).toBe("%s — Acme Goods");
  });

  it("falls back to the store name when no template is set", () => {
    expect(resolveTitleTemplate("Acme", null)).toBe("%s · Acme");
    expect(resolveTitleTemplate("Acme", {} as never)).toBe("%s · Acme");
    expect(resolveTitleTemplate("Acme", { seo_title_template: "   " } as never)).toBe(
      "%s · Acme",
    );
  });

  it("ignores a template with no %s, which has nowhere to put the page title", () => {
    // Honouring it would replace every page title with one constant.
    expect(
      resolveTitleTemplate("Acme", { seo_title_template: "Acme Goods" } as never),
    ).toBe("%s · Acme");
  });
});

describe("the root layout", () => {
  const layout = readFileSync(
    path.join(__dirname, "..", "app", "layout.tsx"),
    "utf8",
  );

  // Strip comments: they legitimately discuss the old hardcoded template.
  const code = layout
    .replace(/\/\*[\s\S]*?\*\//g, "")
    .split("\n")
    .filter((l) => !l.trim().startsWith("//") && !l.trim().startsWith("*"))
    .join("\n");

  it("resolves the template per request rather than exporting a static one", () => {
    expect(
      code.includes("export async function generateMetadata"),
      "the root layout must resolve title.template from branding — a static " +
        "export cannot see the merchant's setting (#894)",
    ).toBe(true);
    expect(code).toContain("resolveTitleTemplate");
  });

  it("never ships a bare pass-through template to children", () => {
    // `template: "%s"` renders child titles unchanged AND outranks any
    // template a page sets for itself.
    const passthrough = /template:\s*["'`]%s["'`]/.exec(code);
    expect(
      passthrough?.[0],
      "found a hardcoded pass-through template in the root layout; it " +
        "silently defeats every child title (#894)",
    ).toBeUndefined();
  });
});
