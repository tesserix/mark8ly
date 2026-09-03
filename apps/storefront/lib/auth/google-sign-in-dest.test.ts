import { describe, expect, it } from "vitest";

import { isGoogleSignInDest } from "./google-sign-in-dest";

describe("isGoogleSignInDest", () => {
  it("accepts the known destinations", () => {
    expect(isGoogleSignInDest("/account")).toBe(true);
    expect(isGoogleSignInDest("/account/security")).toBe(true);
  });

  it("rejects an arbitrary path (open-redirect guard)", () => {
    expect(isGoogleSignInDest("https://evil.example.com")).toBe(false);
    expect(isGoogleSignInDest("//evil.example.com")).toBe(false);
    expect(isGoogleSignInDest("/account/orders")).toBe(false);
    expect(isGoogleSignInDest("")).toBe(false);
  });

  it("rejects non-string values", () => {
    expect(isGoogleSignInDest(undefined)).toBe(false);
    expect(isGoogleSignInDest(null)).toBe(false);
    expect(isGoogleSignInDest(42)).toBe(false);
  });
});
