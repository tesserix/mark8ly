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
  it("carries the nonce and strict-dynamic instead of unsafe-inline", () => {
    const scriptSrc = directives(buildCsp("abc123")).get("script-src")!;
    expect(scriptSrc).toContain("'nonce-abc123'");
    expect(scriptSrc).toContain("'strict-dynamic'");
    expect(scriptSrc).not.toContain("'unsafe-inline'");
  });

  it("keeps the sign-in and analytics hosts as the CSP2 fallback", () => {
    const scriptSrc = directives(buildCsp("abc123")).get("script-src")!;
    expect(scriptSrc).toContain("https://accounts.google.com/gsi/client");
    expect(scriptSrc).toContain("https://appleid.cdn-apple.com");
    expect(scriptSrc).toContain("https://analytics.tesserix.app");
  });

  it("allows eval only in development, where Next compiles with it for HMR", () => {
    expect(buildCsp("n", "production")).not.toContain("'unsafe-eval'");
    expect(buildCsp("n", "development")).toContain("'unsafe-eval'");
  });

  it("keeps inline styles allowed — React and Tailwind emit them", () => {
    expect(directives(buildCsp("n")).get("style-src")).toContain("'unsafe-inline'");
  });

  it("denies framing and keeps the sign-in frame sources", () => {
    const d = directives(buildCsp("n"));
    expect(d.get("frame-ancestors")).toBe("'none'");
    expect(d.get("frame-src")).toContain("https://accounts.google.com/gsi/");
    expect(d.get("object-src")).toBe("'none'");
    expect(d.get("base-uri")).toBe("'self'");
  });
});
