import { expect, test } from "@playwright/test";

// guides.ts is the single source of truth for /guides (the index),
// /guides/[slug] (the article), and sitemap.ts. There's no DOM harness in
// this unit-test config (see playwright.unit.config.ts), so "renders at
// /guides/<slug>" and "appears on the index" are proven against the data
// both pages actually render from — generateStaticParams for the article
// route, and the GUIDES array itself for the index — rather than by
// mounting a component.
import { GUIDES, getGuide } from "../../app/guides/guides";
import { generateStaticParams } from "../../app/guides/[slug]/page";

const NEW_SLUGS = ["how-to-add-your-first-product", "how-to-connect-your-domain"];

test.describe("guides content", () => {
  for (const slug of NEW_SLUGS) {
    test(`${slug} is present in GUIDES`, () => {
      expect(getGuide(slug)).toBeDefined();
    });

    test(`${slug} generates a static param for its article route`, () => {
      const params = generateStaticParams();
      expect(params).toContainEqual({ slug });
    });

    test(`${slug} appears on the /guides index (GUIDES array)`, () => {
      expect(GUIDES.some((g) => g.slug === slug)).toBe(true);
    });
  }

  test("every slug is unique", () => {
    const slugs = GUIDES.map((g) => g.slug);
    expect(new Set(slugs).size).toBe(slugs.length);
  });

  test("every title is unique (guards against copy-paste drift)", () => {
    const titles = GUIDES.map((g) => g.title);
    expect(new Set(titles).size).toBe(titles.length);
  });

  test("every description is unique (guards against copy-paste drift)", () => {
    const descriptions = GUIDES.map((g) => g.description);
    expect(new Set(descriptions).size).toBe(descriptions.length);
  });

  test("every guide has a non-empty blocks array", () => {
    for (const guide of GUIDES) {
      expect(guide.blocks.length).toBeGreaterThan(0);
    }
  });

  test("every guide has a valid ISO `updated` date", () => {
    for (const guide of GUIDES) {
      expect(guide.updated).toMatch(/^\d{4}-\d{2}-\d{2}$/);
      expect(Number.isNaN(new Date(guide.updated).getTime())).toBe(false);
    }
  });

  test("no guide's title duplicates the brand name (layout appends it)", () => {
    for (const guide of GUIDES) {
      expect(guide.title.toLowerCase()).not.toContain("mark8ly");
    }
  });
});
