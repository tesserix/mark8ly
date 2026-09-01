import { test, expect } from "@playwright/test";

import HelpPage, { metadata as helpMetadata } from "../../app/help/page";
import { metadata as integrationsMetadata } from "../../app/integrations/page";
import sitemap from "../../app/sitemap";

/**
 * Issue #149 — /help and /integrations were reported by Search Console as
 * excluded by a noindex tag. They're now real content, so this locks in
 * that both are indexable, carry distinct metadata, and are listed in the
 * sitemap — plus that the /help FAQPage JSON-LD is well-formed.
 */

test.describe("metadata no longer sets noindex", () => {
  for (const [name, metadata] of [
    ["/help", helpMetadata],
    ["/integrations", integrationsMetadata],
  ] as const) {
    test(`${name} metadata does not set robots index: false`, () => {
      const robots = metadata.robots as
        | { index?: boolean }
        | string
        | undefined;
      if (robots && typeof robots === "object") {
        expect(robots.index).not.toBe(false);
      } else {
        expect(robots).toBeUndefined();
      }
    });

    test(`${name} has a non-empty title and description`, () => {
      expect(String(metadata.title ?? "").length).toBeGreaterThan(0);
      expect(String(metadata.description ?? "").length).toBeGreaterThan(0);
    });
  }

  test("/help and /integrations have distinct titles and descriptions", () => {
    expect(helpMetadata.title).not.toEqual(integrationsMetadata.title);
    expect(helpMetadata.description).not.toEqual(
      integrationsMetadata.description,
    );
  });

  test("/help and /integrations declare canonical URLs and OpenGraph blocks", () => {
    for (const metadata of [helpMetadata, integrationsMetadata]) {
      expect(metadata.alternates?.canonical).toBeTruthy();
      expect(metadata.openGraph?.title).toBeTruthy();
      expect(metadata.openGraph?.description).toBeTruthy();
    }
  });
});

test.describe("sitemap", () => {
  test("includes /help and /integrations", () => {
    const urls = sitemap().map((entry) => entry.url);
    expect(urls).toContain("https://mark8ly.com/help");
    expect(urls).toContain("https://mark8ly.com/integrations");
  });

  test("still excludes noindexed and funnel-only routes", () => {
    const urls = sitemap().map((entry) => entry.url);
    for (const excluded of [
      "https://mark8ly.com/blog",
      "https://mark8ly.com/dpa",
      "https://mark8ly.com/onboarding",
      "https://mark8ly.com/welcome",
    ]) {
      expect(urls).not.toContain(excluded);
    }
  });
});

test.describe("/help FAQPage JSON-LD", () => {
  test("renders valid JSON with a non-empty FAQPage mainEntity", () => {
    const element = HelpPage();

    // The <script type="application/ld+json"> is the first child of the
    // MarketingPage tree. Rather than pull in a renderer, walk the plain
    // React element tree — server components return ordinary element
    // objects, no DOM involved — to find it and parse its content.
    const script = findJsonLdScript(element);
    expect(script).not.toBeNull();

    const parsed = JSON.parse(script as string);
    expect(parsed["@type"]).toBe("FAQPage");
    expect(Array.isArray(parsed.mainEntity)).toBe(true);
    expect(parsed.mainEntity.length).toBeGreaterThan(0);

    for (const entry of parsed.mainEntity) {
      expect(entry["@type"]).toBe("Question");
      expect(typeof entry.name).toBe("string");
      expect(entry.name.length).toBeGreaterThan(0);
      expect(entry.acceptedAnswer["@type"]).toBe("Answer");
      expect(typeof entry.acceptedAnswer.text).toBe("string");
      expect(entry.acceptedAnswer.text.length).toBeGreaterThan(0);
    }
  });
});

// Minimal recursive walk over a React element tree looking for the
// dangerouslySetInnerHTML payload of a <script type="application/ld+json">.
// Works without a DOM because JSX is just nested plain objects until a
// renderer processes it.
function findJsonLdScript(node: unknown): string | null {
  if (node === null || node === undefined || typeof node !== "object") {
    return null;
  }
  const el = node as {
    type?: unknown;
    props?: {
      type?: string;
      dangerouslySetInnerHTML?: { __html?: string };
      children?: unknown;
    };
  };

  if (el.type === "script" && el.props?.type === "application/ld+json") {
    return el.props.dangerouslySetInnerHTML?.__html ?? null;
  }

  const children = el.props?.children;
  if (Array.isArray(children)) {
    for (const child of children) {
      const found = findJsonLdScript(child);
      if (found) return found;
    }
  } else if (children) {
    const found = findJsonLdScript(children);
    if (found) return found;
  }

  return null;
}
