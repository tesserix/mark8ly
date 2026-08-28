import { describe, expect, it } from "vitest";
import { buildCsp } from "./csp";

function directives(csp: string): Map<string, string> {
  return new Map(
    csp.split("; ").map((d) => {
      const [name, ...rest] = d.split(" ");
      return [name!, rest.join(" ")];
    }),
  );
}

describe("buildCsp", () => {
  it("carries the nonce instead of unsafe-inline", () => {
    const scriptSrc = directives(buildCsp("abc123")).get("script-src")!;
    expect(scriptSrc).toContain("'nonce-abc123'");
    expect(scriptSrc).not.toContain("'unsafe-inline'");
  });

  // strict-dynamic would drop the host list, and the payment SDKs pull
  // further scripts we cannot enumerate — some via paths that
  // strict-dynamic does not cover. The allowlist stays authoritative.
  it("keeps the host allowlist authoritative", () => {
    const scriptSrc = directives(buildCsp("n")).get("script-src")!;
    expect(scriptSrc).not.toContain("'strict-dynamic'");
    expect(scriptSrc).toContain("https://*.razorpay.com");
    expect(scriptSrc).toContain("https://accounts.google.com/gsi/client");
    expect(scriptSrc).toContain("https://analytics.tesserix.app");
  });

  it("allows eval only in development, where Next compiles with it for HMR", () => {
    expect(buildCsp("n", "production")).not.toContain("'unsafe-eval'");
    expect(buildCsp("n", "development")).toContain("'unsafe-eval'");
  });

  // Merchants inject branded CSS via <style> (sanitizeCss in app/layout.tsx).
  it("keeps inline styles allowed", () => {
    expect(directives(buildCsp("n")).get("style-src")).toContain("'unsafe-inline'");
  });

  it("keeps the payment modal frame sources and the injection guards", () => {
    const d = directives(buildCsp("n"));
    expect(d.get("frame-src")).toContain("https://*.razorpay.com");
    expect(d.get("object-src")).toBe("'none'");
    expect(d.get("base-uri")).toBe("'self'");
    expect(d.get("form-action")).toBe("'self'");
  });
});
