import { afterEach, beforeEach, describe, expect, it } from "vitest";

import { getAuthProvider, isGoogleSignInOffered } from "./provider";

// Matches apps/storefront/app/sign-in/actions.ts's AUTH_PROVIDER rule
// exactly: only the literal string "zitadel" flips the flag. This module
// reads process.env at call time (not module-evaluation time), so unlike
// actions.zitadel.test.ts there is no need to vi.resetModules()/dynamic
// import per test — setting the env var before each call is enough.

beforeEach(() => {
  delete process.env.NEXT_PUBLIC_AUTH_PROVIDER;
});

afterEach(() => {
  delete process.env.NEXT_PUBLIC_AUTH_PROVIDER;
});

describe("getAuthProvider", () => {
  it('flag "zitadel" resolves to "zitadel"', () => {
    process.env.NEXT_PUBLIC_AUTH_PROVIDER = "zitadel";
    expect(getAuthProvider()).toBe("zitadel");
  });

  it("flag unset resolves to \"gip\"", () => {
    expect(getAuthProvider()).toBe("gip");
  });

  it('flag "" (empty string) resolves to "gip"', () => {
    process.env.NEXT_PUBLIC_AUTH_PROVIDER = "";
    expect(getAuthProvider()).toBe("gip");
  });

  it('flag "Zitadel" (wrong case) resolves to "gip" — the comparison must stay an exact literal match', () => {
    process.env.NEXT_PUBLIC_AUTH_PROVIDER = "Zitadel";
    expect(getAuthProvider()).toBe("gip");
  });

  it('flag "true" resolves to "gip"', () => {
    process.env.NEXT_PUBLIC_AUTH_PROVIDER = "true";
    expect(getAuthProvider()).toBe("gip");
  });
});

describe("isGoogleSignInOffered", () => {
  it('flag "zitadel": Google sign-in is NOT offered', () => {
    process.env.NEXT_PUBLIC_AUTH_PROVIDER = "zitadel";
    expect(isGoogleSignInOffered()).toBe(false);
  });

  it("flag unset: Google sign-in is offered", () => {
    expect(isGoogleSignInOffered()).toBe(true);
  });

  it('flag "" (empty string): Google sign-in is offered', () => {
    process.env.NEXT_PUBLIC_AUTH_PROVIDER = "";
    expect(isGoogleSignInOffered()).toBe(true);
  });

  it('flag "Zitadel" (wrong case): Google sign-in is offered — pinned so a future "simplification" of the comparison to something looser (e.g. case-insensitive) gets caught here', () => {
    process.env.NEXT_PUBLIC_AUTH_PROVIDER = "Zitadel";
    expect(isGoogleSignInOffered()).toBe(true);
  });

  it('flag "true": Google sign-in is offered', () => {
    process.env.NEXT_PUBLIC_AUTH_PROVIDER = "true";
    expect(isGoogleSignInOffered()).toBe(true);
  });
});
