/**
 * CSP allowlist guard.
 *
 * A CSP violation only exists in a real browser at runtime, so tsc, the
 * unit suite and the build are all blind to it. That blind spot has now
 * cost two production outages from one commit (1cece186, which added the
 * CSP):
 *   - Google sign-in broke for 6 days  (fixed in 6b0ed1ea)
 *   - Razorpay checkout broke for 2 MONTHS — every INR order stuck at
 *     "reserved, awaiting payment" — found only by manually testing
 *     checkout on 2026-07-17.
 *
 * Both had the same cause: the allowlist was written from what someone
 * remembered, not from what the code actually loads. This test closes
 * that loop — it scans the source for external script URLs and fails if
 * script-src does not cover them.
 *
 * It cannot catch hosts the code never names (e.g. cdn.razorpay.com,
 * which checkout.js fetches at runtime). That is exactly why third-party
 * SDK vendors are allowlisted by wildcard rather than enumerated.
 */

import { describe, it, expect } from "vitest";
import { readFileSync, readdirSync, statSync } from "node:fs";
import path from "node:path";

const APP_ROOT = path.resolve(__dirname, "../..");
const SCAN_DIRS = ["app", "components", "lib"];
const SCRIPT_URL_RE = /https:\/\/[a-z0-9.-]+\.[a-z]{2,}(?:\/[^"'`\s]*)?\.js\b/gi;

function walk(dir: string, out: string[] = []): string[] {
  let entries: string[];
  try {
    entries = readdirSync(dir);
  } catch {
    return out;
  }
  for (const entry of entries) {
    if (entry === "node_modules" || entry === ".next") continue;
    const full = path.join(dir, entry);
    if (statSync(full).isDirectory()) walk(full, out);
    else if (/\.tsx?$/.test(entry)) out.push(full);
  }
  return out;
}

/** Hosts referenced as external <script src> in first-party source. */
function externalScriptHosts(): string[] {
  const hosts = new Set<string>();
  for (const dir of SCAN_DIRS) {
    for (const file of walk(path.join(APP_ROOT, dir))) {
      const src = readFileSync(file, "utf8");
      for (const match of src.match(SCRIPT_URL_RE) ?? []) {
        hosts.add(new URL(match).host);
      }
    }
  }
  return [...hosts];
}

function scriptSrcDirective(): string {
  const config = readFileSync(path.join(APP_ROOT, "next.config.ts"), "utf8");
  const line = config
    .split("\n")
    .find((l) => l.includes('"script-src'));
  if (!line) throw new Error("script-src directive not found in next.config.ts");
  return line;
}

/** Mirrors the CSP host-matching rule: exact host, or a *.domain wildcard. */
function isAllowed(host: string, directive: string): boolean {
  if (directive.includes(`https://${host}`)) return true;
  const parts = host.split(".");
  for (let i = 1; i < parts.length - 1; i++) {
    if (directive.includes(`https://*.${parts.slice(i).join(".")}`)) return true;
  }
  return false;
}

describe("CSP script-src covers every external script the app loads", () => {
  it("finds the scripts it is supposed to be guarding", () => {
    // Guards the guard: if the scan silently matches nothing, the test
    // below passes vacuously and the blind spot is back.
    expect(externalScriptHosts().length).toBeGreaterThan(0);
  });

  it.each(externalScriptHosts())("allows %s", (host) => {
    expect(
      isAllowed(host, scriptSrcDirective()),
      `${host} is loaded by the app but missing from script-src in next.config.ts — ` +
        `it will be blocked in the browser and the feature will fail silently in prod.`,
    ).toBe(true);
  });
});

describe("isAllowed matching rules", () => {
  const directive = `"script-src 'self' https://accounts.google.com/gsi/client https://*.razorpay.com https://*.cashfree.com",`;

  it.each([
    ["checkout.razorpay.com", true],
    ["cdn.razorpay.com", true],
    ["lumberjack.razorpay.com", true],
    // Cashfree's v3 SDK is served from sdk.cashfree.com and pulls the checkout
    // modal from payments.cashfree.com — the same runtime-subdomain problem
    // that makes enumeration unworkable for Razorpay.
    ["sdk.cashfree.com", true],
    ["payments.cashfree.com", true],
    ["accounts.google.com", true],
  ])("allows %s", (host, want) => {
    expect(isAllowed(host as string, directive)).toBe(want);
  });

  it.each([
    "evil.com",
    "razorpay.com.evil.com",
    "cashfree.com.evil.com",
    "cdn.stripe.com",
  ])(
    "rejects %s",
    (host) => {
      expect(isAllowed(host, directive)).toBe(false);
    },
  );
});
