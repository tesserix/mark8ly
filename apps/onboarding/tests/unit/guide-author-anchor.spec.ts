import { expect, test } from "@playwright/test";
import { readFileSync } from "node:fs";
import path from "node:path";

import { GUIDES } from "../../app/guides/guides";

/**
 * Regression guard for GitHub issue #594.
 *
 * The UPI guide publishes under a named Person whose schema `@id` is
 * `https://mark8ly.com/about#mahesh-sangawar`. That identifier is only
 * worth anything if it resolves — the whole argument for a named byline
 * over an anonymous Organization one is that a reader can go and check
 * who wrote this. An `@id` pointing at an anchor that does not exist is
 * a byline that fails exactly the test it was added to pass, and it
 * fails silently: the page renders, the JSON-LD validates, and nothing
 * anywhere complains.
 *
 * So this pins the coupling in both directions — every named author's
 * `@id` fragment must exist as an `id=` on /about, and the bio we show
 * a crawler must be the bio we show a person.
 */
function source(relative: string): string {
  return readFileSync(path.join(__dirname, "../..", relative), "utf8");
}

const authored = GUIDES.filter((guide) => guide.author !== undefined);

test("at least one guide carries a named author, or this guard is vacuous", () => {
  expect(authored.length).toBeGreaterThan(0);
});

for (const guide of authored) {
  const author = guide.author!;

  test(`${guide.slug}: the author @id resolves to a real anchor on /about (#594)`, () => {
    const fragment = author.id.split("#")[1];
    expect(fragment, `${author.id} has no #fragment`).toBeTruthy();
    expect(
      source("app/about/page.tsx"),
      `/about has no element with id="${fragment}"`,
    ).toContain(`id="${fragment}"`);
  });

  test(`${guide.slug}: the author is named on /about, not only in schema`, () => {
    expect(source("app/about/page.tsx")).toContain(author.name);
  });

  test(`${guide.slug}: the byline is checkable off-site`, () => {
    // A bio we host is an assertion about ourselves. The point of
    // sameAs is that at least one profile lives somewhere a reader can
    // corroborate without taking our word for it.
    const offsite = author.sameAs.filter(
      (url) => !url.startsWith("https://mark8ly.com"),
    );
    expect(offsite.length).toBeGreaterThan(0);
    for (const url of author.sameAs) {
      expect(url).toMatch(/^https:\/\//);
    }
  });
}
