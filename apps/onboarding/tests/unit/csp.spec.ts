import { expect, test } from "@playwright/test";
import { readdirSync, readFileSync, statSync } from "node:fs";
import path from "node:path";

import {
  buildCsp,
  buildStaticCsp,
  NONCE_PATH_PREFIXES,
  usesNonce,
} from "../../lib/security/csp";

function directives(csp: string): Map<string, string> {
  return new Map(
    csp.split("; ").map((d) => {
      const [name, ...rest] = d.split(" ");
      return [name!, rest.join(" ")];
    }),
  );
}

test("the nonce policy drops unsafe-inline and carries the nonce", () => {
  const scriptSrc = directives(buildCsp("abc123", "sha256-x")).get("script-src")!;
  expect(scriptSrc).toContain("'nonce-abc123'");
  expect(scriptSrc).toContain("'sha256-x'");
  expect(scriptSrc).not.toContain("'unsafe-inline'");
});

test("the static policy still needs unsafe-inline", () => {
  // Prerendered HTML cannot carry a per-request nonce, so the marketing
  // pages keep the weaker policy rather than lose static generation.
  const scriptSrc = directives(buildStaticCsp()).get("script-src")!;
  expect(scriptSrc).toContain("'unsafe-inline'");
});

test("both policies allow the same external scripts", () => {
  const strict = directives(buildCsp("n", "sha256-x")).get("script-src")!;
  const relaxed = directives(buildStaticCsp()).get("script-src")!;
  for (const host of [
    "https://accounts.google.com/gsi/client",
    "https://analytics.tesserix.app",
  ]) {
    expect(strict, `strict policy must allow ${host}`).toContain(host);
    expect(relaxed, `static policy must allow ${host}`).toContain(host);
  }
});

test("eval is allowed only in development", () => {
  expect(buildCsp("n", "h", "production")).not.toContain("'unsafe-eval'");
  expect(buildCsp("n", "h", "development")).toContain("'unsafe-eval'");
  expect(buildStaticCsp("production")).not.toContain("'unsafe-eval'");
});

test("the credential-handling routes get the nonce policy", () => {
  expect(usesNonce("/onboarding")).toBe(true);
  expect(usesNonce("/onboarding/set-password")).toBe(true);
  expect(usesNonce("/auth/google")).toBe(true);
  expect(usesNonce("/about")).toBe(false);
  expect(usesNonce("/guides/getting-started")).toBe(false);
});

/**
 * A nonce only reaches script tags that are rendered per request. If one
 * of these routes ever goes back to being prerendered, its own scripts
 * get blocked — so the opt-out has to be pinned down here rather than
 * discovered in prod.
 */
test("every nonce route is rendered per request", () => {
  const appDir = path.resolve(__dirname, "../../app");
  const pages: string[] = [];
  for (const prefix of NONCE_PATH_PREFIXES) {
    collectPages(path.join(appDir, prefix), pages);
  }
  expect(pages.length, "the walk found no pages — the guard would pass vacuously").toBeGreaterThan(0);
  for (const file of pages) {
    expect(
      readFileSync(file, "utf8"),
      `${path.relative(appDir, file)} must export dynamic = "force-dynamic" — a prerendered page under a nonce CSP ships script tags the browser then blocks`,
    ).toContain('export const dynamic = "force-dynamic"');
  }
});

function collectPages(dir: string, out: string[]): string[] {
  let entries: string[];
  try {
    entries = readdirSync(dir);
  } catch {
    return out;
  }
  for (const entry of entries) {
    const full = path.join(dir, entry);
    if (statSync(full).isDirectory()) collectPages(full, out);
    else if (entry === "page.tsx") out.push(full);
  }
  return out;
}
