import { expect, test } from "@playwright/test";
import { readFileSync } from "node:fs";
import path from "node:path";

/**
 * Regression guard for GitHub issue #597.
 *
 * Middleware runs on every user-facing route. Any `Set-Cookie` it
 * attaches travels with the response, and a response carrying
 * `Set-Cookie` cannot be shared between visitors — so Cloudflare will
 * not cache it, and every marketing page reverts to a full origin
 * round-trip to a single pod in Sydney.
 *
 * That is exactly what used to happen: middleware wrote `mk8_currency`
 * from `CF-IPCountry` on every response, and `cf-cache-status` was
 * `DYNAMIC` site-wide as a result. The geo lookup now lives in
 * /api/geo-currency, which the client fetches and caches itself.
 *
 * This regression is invisible without a guard. Adding a cookie back
 * breaks no test, renders correctly, and passes review — it just
 * quietly makes the site uncacheable again, and the only symptom is a
 * latency number nobody is watching.
 */
function source(relative: string): string {
  return readFileSync(path.join(__dirname, "../..", relative), "utf8");
}

/** Comments name the removed cookie in order to explain it, so scanning
 *  raw source would trip the guard on its own documentation. */
function codeOnly(text: string): string {
  return text.replace(/\/\*[\s\S]*?\*\//g, "").replace(/^\s*\/\/.*$/gm, "");
}

test("middleware attaches no cookie to any response (#597)", () => {
  const code = codeOnly(source("middleware.ts"));
  expect(code).not.toContain("cookies.set");
  expect(code.toLowerCase()).not.toContain("set-cookie");
});

test("the geo endpoint stays per-request, never cached (#597)", () => {
  const code = source("app/api/geo-currency/route.ts");
  // If this response were ever cached it would report the currency of
  // whoever warmed the cache — the same bug, moved rather than fixed.
  expect(code).toContain('export const dynamic = "force-dynamic"');
  expect(code).toContain("no-store");
});
