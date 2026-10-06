import { readFileSync } from "node:fs";
import path from "node:path";

import { test, expect } from "@playwright/test";

/**
 * The root layout must declare a revalidation period, or every static
 * marketing page ships with a one-year `s-maxage` and a deploy does not
 * reach visitors until someone purges the CDN by hand (seen 2026-10-06).
 *
 * A source scan rather than an import: app/layout.tsx pulls next/font,
 * which this Node-only runner cannot load. The regex is deliberately
 * narrow so a refactor that keeps the export passes and one that drops
 * it, or pushes it past ten minutes, does not.
 */
test("root layout caps the edge lifetime of prerendered pages", () => {
  const source = readFileSync(
    path.resolve(__dirname, "..", "..", "app", "layout.tsx"),
    "utf8",
  );
  const match = source.match(/^export const revalidate = (\d+);/m);
  expect(match, "app/layout.tsx no longer exports a numeric `revalidate`").not.toBeNull();
  const seconds = Number(match![1]);
  expect(seconds).toBeGreaterThan(0);
  expect(seconds, "edge lifetime over ten minutes defeats the point").toBeLessThanOrEqual(600);
});
