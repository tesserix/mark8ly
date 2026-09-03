import { afterEach, beforeEach, describe, expect, it } from "vitest";

import {
  getAuthProvider,
  isGoogleLinkOffered,
  isGoogleSignInOffered,
} from "./provider";

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
  // Google-through-Zitadel (phase 3c-2) now exists: the control is
  // offered on BOTH providers — under GIP it drives the unchanged
  // trampoline, under Zitadel it drives auth-bff's own Google IDP intent
  // (see @/lib/auth/google-sign-in). This is the flip side of the old
  // 3c-1 gate this same file used to enforce.
  it('flag "zitadel": Google sign-in IS offered (drives the new Zitadel flow)', () => {
    process.env.NEXT_PUBLIC_AUTH_PROVIDER = "zitadel";
    expect(isGoogleSignInOffered()).toBe(true);
  });

  it("flag unset: Google sign-in is offered (drives the unchanged trampoline)", () => {
    expect(isGoogleSignInOffered()).toBe(true);
  });

  it('flag "" (empty string): Google sign-in is offered', () => {
    process.env.NEXT_PUBLIC_AUTH_PROVIDER = "";
    expect(isGoogleSignInOffered()).toBe(true);
  });

  it('flag "Zitadel" (wrong case): Google sign-in is offered', () => {
    process.env.NEXT_PUBLIC_AUTH_PROVIDER = "Zitadel";
    expect(isGoogleSignInOffered()).toBe(true);
  });

  it('flag "true": Google sign-in is offered', () => {
    process.env.NEXT_PUBLIC_AUTH_PROVIDER = "true";
    expect(isGoogleSignInOffered()).toBe(true);
  });
});

describe("isGoogleLinkOffered", () => {
  // Unlike isGoogleSignInOffered, this one DOES gate on the flag:
  // SecurityClient's "Add Google" (account-linking) control needs a
  // "link this provider to my existing account" backend endpoint that
  // does not exist under Zitadel — offering it there would let the
  // control silently switch a signed-in shopper to a different,
  // self-registered account. See the whole-branch security review's
  // HIGH finding on this.
  it('flag "zitadel": Google account-linking is NOT offered', () => {
    process.env.NEXT_PUBLIC_AUTH_PROVIDER = "zitadel";
    expect(isGoogleLinkOffered()).toBe(false);
  });

  it("flag unset: Google account-linking is offered (unchanged GIP link flow)", () => {
    expect(isGoogleLinkOffered()).toBe(true);
  });

  it('flag "" (empty string): Google account-linking is offered', () => {
    process.env.NEXT_PUBLIC_AUTH_PROVIDER = "";
    expect(isGoogleLinkOffered()).toBe(true);
  });

  it('flag "Zitadel" (wrong case): Google account-linking is offered — exact literal match only', () => {
    process.env.NEXT_PUBLIC_AUTH_PROVIDER = "Zitadel";
    expect(isGoogleLinkOffered()).toBe(true);
  });

  it('flag "true": Google account-linking is offered', () => {
    process.env.NEXT_PUBLIC_AUTH_PROVIDER = "true";
    expect(isGoogleLinkOffered()).toBe(true);
  });
});
