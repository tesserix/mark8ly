import { test, expect } from "@playwright/test";

import { metadata as shopifyMetadata } from "../../app/shopify-alternative/page";
import { metadata as makersMetadata } from "../../app/ecommerce-for-makers/page";
import { metadata as indiaMetadata } from "../../app/sell-online-india/page";
import { metadata as etsyMetadata } from "../../app/etsy-alternative/page";
import robots from "../../app/robots";
import sitemap from "../../app/sitemap";

/**
 * Issue #994 — the four comparison landing pages (#151, #602) are the
 * routes built to capture buying-intent search traffic, and until now the
 * only SEO contract tests covered /help and /integrations (#149). This
 * locks in, from the source of truth each thing is rendered from, that
 * every comparison page is indexable, self-canonical, distinct from its
 * siblings, listed once in the sitemap, and not blocked for Googlebot.
 *
 * What it cannot prove: that the deployed build matches, or that Google
 * indexes the page. The deployed half is a post-release curl of status,
 * canonical, X-Robots-Tag and the sitemap entry — see the issue.
 */

const PAGES = [
  ["/shopify-alternative", shopifyMetadata],
  ["/ecommerce-for-makers", makersMetadata],
  ["/sell-online-india", indiaMetadata],
  ["/etsy-alternative", etsyMetadata],
] as const;

test.describe("comparison page metadata", () => {
  for (const [route, metadata] of PAGES) {
    test(`${route} is indexable`, () => {
      const robotsField = metadata.robots as
        | { index?: boolean; follow?: boolean }
        | string
        | undefined;
      if (robotsField && typeof robotsField === "object") {
        expect(robotsField.index).not.toBe(false);
        expect(robotsField.follow).not.toBe(false);
      } else if (typeof robotsField === "string") {
        expect(robotsField).not.toMatch(/noindex/i);
      }
    });

    test(`${route} is its own canonical`, () => {
      expect(metadata.alternates?.canonical).toBe(route);
    });

    test(`${route} has a title, description and OpenGraph block`, () => {
      expect(String(metadata.title ?? "").length).toBeGreaterThan(0);
      expect(String(metadata.description ?? "").length).toBeGreaterThan(0);
      expect(metadata.openGraph?.title).toBeTruthy();
      expect(metadata.openGraph?.description).toBeTruthy();
    });
  }

  test("the four pages do not share a title or description", () => {
    const titles = PAGES.map(([, m]) => String(m.title));
    const descriptions = PAGES.map(([, m]) => String(m.description));
    expect(new Set(titles).size).toBe(PAGES.length);
    expect(new Set(descriptions).size).toBe(PAGES.length);
  });
});

test.describe("comparison pages in the sitemap", () => {
  for (const [route] of PAGES) {
    test(`${route} is listed exactly once with landing-page priority`, () => {
      const matches = sitemap().filter(
        (entry) => entry.url === `https://mark8ly.com${route}`,
      );
      expect(matches).toHaveLength(1);
      expect(matches[0]?.priority).toBe(0.9);
      expect(matches[0]?.lastModified).toBeDefined();
    });
  }
});

test.describe("comparison pages in robots.txt", () => {
  test("no rule disallows a comparison route for Googlebot or everyone", () => {
    const config = robots();
    const rules = Array.isArray(config.rules) ? config.rules : [config.rules];

    for (const rule of rules) {
      const agents = Array.isArray(rule.userAgent)
        ? rule.userAgent
        : [rule.userAgent ?? "*"];
      const applies = agents.some(
        (agent) => agent === "*" || /^googlebot$/i.test(agent ?? ""),
      );
      if (!applies) continue;

      const disallow = Array.isArray(rule.disallow)
        ? rule.disallow
        : rule.disallow
          ? [rule.disallow]
          : [];
      for (const [route] of PAGES) {
        const blocked = disallow.filter(
          (prefix) => prefix === "/" || route.startsWith(prefix),
        );
        expect(
          blocked,
          `${route} is disallowed for ${agents.join(", ")}`,
        ).toEqual([]);
      }
    }
  });
});
