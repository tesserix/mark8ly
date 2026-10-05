import { execFileSync } from "node:child_process";
import { existsSync } from "node:fs";
import path from "node:path";

import { test, expect } from "@playwright/test";

import sitemap from "../../app/sitemap";

/**
 * Issue #994 — three comparison pages gained real content in #616 and the
 * sitemap kept telling crawlers they were last touched a month earlier.
 * The dates in app/sitemap.ts are checked in by hand on purpose (#603 is
 * why they are not generated), which means nothing stopped them drifting.
 *
 * This spec is that stop. For every route whose `lastmod` comes from the
 * LAST_MODIFIED table it asks git when the page's source last changed and
 * fails when the table is older than that. The convention it enforces is
 * the one the table's own header documents: `git log -1 --format=%cs`,
 * the committer's calendar date.
 *
 * Visual-only commits are the escape hatch. If the newest commit on a
 * page only touched presentation, list its hash in CONTENT_NEUTRAL_COMMITS
 * and the guard looks past it to the next commit. That keeps the table's
 * "content only" rule honest instead of forcing a bump for a CSS edit.
 */

const APP_ROOT = path.resolve(__dirname, "..", "..");

/**
 * Commits that touched a page file without changing anything a searcher
 * would read. Full or abbreviated hashes, one per line, with a note.
 */
const CONTENT_NEUTRAL_COMMITS: ReadonlyArray<string> = [
  // none yet
];

function git(...args: string[]): string | null {
  try {
    return execFileSync("git", args, {
      cwd: APP_ROOT,
      encoding: "utf8",
      stdio: ["ignore", "pipe", "ignore"],
    }).trim();
  } catch {
    return null;
  }
}

function isNeutral(hash: string): boolean {
  return CONTENT_NEUTRAL_COMMITS.some(
    (neutral) => neutral.length >= 7 && hash.startsWith(neutral),
  );
}

/**
 * The page source for a sitemap route, or null when the route is not a
 * plain `app/<route>/page.tsx` (guides derive their dates elsewhere).
 */
function pageFileFor(routePath: string): string | null {
  if (routePath.startsWith("/guides")) return null;
  const relative =
    routePath === "/" ? "app/page.tsx" : `app${routePath}/page.tsx`;
  return existsSync(path.join(APP_ROOT, relative)) ? relative : null;
}

function isoDate(value: Date | string | undefined): string {
  if (value === undefined) return "";
  const date = typeof value === "string" ? new Date(value) : value;
  return date.toISOString().slice(0, 10);
}

test.describe("sitemap lastmod stays honest", () => {
  test("repeated generation yields identical, midnight-anchored dates", () => {
    const first = sitemap();
    const second = sitemap();
    expect(second.length).toBe(first.length);

    for (let i = 0; i < first.length; i++) {
      const entry = first[i];
      const again = second[i];
      if (!entry || !again) throw new Error(`sitemap entry ${i} missing`);
      const a = entry.lastModified;
      expect(a, `${entry.url} has no lastModified`).toBeDefined();
      expect(isoDate(a)).toBe(isoDate(again.lastModified));

      // A checked-in date parses to UTC midnight. A `new Date()` sneaking
      // back in would carry the wall-clock time of the build (#603).
      const date = typeof a === "string" ? new Date(a) : (a as Date);
      expect(
        date.getUTCHours() + date.getUTCMinutes() + date.getUTCSeconds(),
        `${entry.url} lastModified is not a plain date: ${date.toISOString()}`,
      ).toBe(0);
    }
  });

  test("every sitemap URL appears exactly once", () => {
    const urls = sitemap().map((entry) => entry.url);
    const seen = new Map<string, number>();
    for (const url of urls) seen.set(url, (seen.get(url) ?? 0) + 1);
    const duplicates = [...seen].filter(([, n]) => n > 1).map(([u]) => u);
    expect(duplicates).toEqual([]);
  });

  test("no page has changed since the date the sitemap claims", () => {
    const shallow = git("rev-parse", "--is-shallow-repository");
    test.skip(
      shallow === null,
      "git is not available here; the freshness guard needs history",
    );
    test.skip(
      shallow === "true",
      "shallow clone; `git log` cannot see when pages last changed",
    );

    const stale: string[] = [];

    for (const entry of sitemap()) {
      const routePath = new URL(entry.url).pathname || "/";
      const pageFile = pageFileFor(routePath);
      if (!pageFile) continue;

      const log = git("log", "--format=%H %cs", "--", pageFile);
      if (!log) continue;

      const latestContentCommit = log
        .split("\n")
        .map((line) => {
          const [hash = "", date = ""] = line.split(" ");
          return { hash, date };
        })
        .find(({ hash }) => hash.length > 0 && !isNeutral(hash));
      if (!latestContentCommit) continue;

      const claimed = isoDate(entry.lastModified);
      if (claimed < latestContentCommit.date) {
        stale.push(
          `${routePath}: sitemap says ${claimed}, ${pageFile} last changed ` +
            `${latestContentCommit.date} in ${latestContentCommit.hash.slice(0, 8)}`,
        );
      }
    }

    expect(
      stale,
      "LAST_MODIFIED in app/sitemap.ts is behind the page source. Move the " +
        "date forward in the same change, or list the commit in " +
        "CONTENT_NEUTRAL_COMMITS if it did not touch readable content.",
    ).toEqual([]);
  });
});
