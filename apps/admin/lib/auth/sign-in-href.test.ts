import { describe, expect, it } from "vitest";

import { signInHref } from "./sign-in-href";

describe("signInHref", () => {
  it("builds a canonical /login link when the returnUrl is a real slug-admin URL", () => {
    expect(signInHref("https://the-bondi-store-admin.mark8ly.com/dashboard")).toBe(
      "/login?returnUrl=https%3A%2F%2Fthe-bondi-store-admin.mark8ly.com%2Fdashboard",
    );
  });

  it("returns null when there is no returnUrl", () => {
    // Canonical /login 404s without a valid returnUrl, so the caller must
    // render guidance instead of a dead link.
    expect(signInHref(null)).toBeNull();
    expect(signInHref("")).toBeNull();
  });

  it("returns null for a returnUrl the middleware would reject", () => {
    // Canonical host is not a valid bounce-back target.
    expect(signInHref("https://admin.mark8ly.com/dashboard")).toBeNull();
    // Off-platform host must never become a login redirect target.
    expect(signInHref("https://evil.example.com/")).toBeNull();
    expect(signInHref("http://the-bondi-store-admin.mark8ly.com/")).toBeNull();
  });
});
