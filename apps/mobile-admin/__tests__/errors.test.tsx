import {
  AuthCancelledError,
  LastSignInMethodError,
  authErrorMessage,
} from "@repo/mobile-shared/auth/errors";

function withCode(code: string): Error {
  return Object.assign(new Error("raw native text that must never be shown"), { code });
}

describe("authErrorMessage", () => {
  it("returns null for a cancelled sheet", () => {
    expect(authErrorMessage(new AuthCancelledError())).toBeNull();
  });

  it("returns null for a raw Apple ERR_REQUEST_CANCELED (safety net)", () => {
    expect(authErrorMessage(withCode("ERR_REQUEST_CANCELED"))).toBeNull();
  });

  it("maps ERR_REQUEST_UNKNOWN to iCloud guidance", () => {
    expect(authErrorMessage(withCode("ERR_REQUEST_UNKNOWN"))).toMatch(/signed in to iCloud/i);
  });

  it("maps LastSignInMethodError to the guard copy", () => {
    expect(authErrorMessage(new LastSignInMethodError())).toMatch(/only sign-in method/i);
  });

  it("maps a TAGGED reauth failure with { method: 'password' } to wrong-password copy", () => {
    expect(authErrorMessage(withCode("auth/reauth-failed"), { method: "password" })).toMatch(
      /password is incorrect/i,
    );
  });

  it("maps a TAGGED reauth failure WITH a social provider to account copy, not password copy", () => {
    const msg = authErrorMessage(withCode("auth/reauth-failed"), {
      method: "social",
      provider: "google.com",
    });
    expect(msg).toMatch(/couldn't verify that account/i);
    expect(msg).not.toMatch(/password/i);
  });

  it("maps a TAGGED reauth failure with NO ctx to neutral copy — never the password guess", () => {
    const msg = authErrorMessage(withCode("auth/reauth-failed"));
    expect(msg).toMatch(/couldn't verify your account/i);
    expect(msg).not.toMatch(/password/i);
  });

  it("gives an UNTAGGED invalid-credential different copy from a tagged reauth failure", () => {
    const untagged = authErrorMessage(withCode("auth/invalid-credential"));
    const tagged = authErrorMessage(withCode("auth/reauth-failed"));
    expect(untagged).toMatch(/check your details/i);
    expect(untagged).not.toEqual(tagged);
  });

  it("maps credential-already-in-use", () => {
    expect(authErrorMessage(withCode("auth/credential-already-in-use"))).toMatch(
      /already linked to a different Mark8ly account/i,
    );
  });

  it("maps provider-already-linked", () => {
    expect(authErrorMessage(withCode("auth/provider-already-linked"))).toMatch(
      /already linked to your account/i,
    );
  });

  it("maps requires-recent-login", () => {
    expect(authErrorMessage(withCode("auth/requires-recent-login"))).toMatch(/sign out and sign in/i);
  });

  it("maps network-request-failed", () => {
    expect(authErrorMessage(withCode("auth/network-request-failed"))).toMatch(/no connection/i);
  });

  it("maps too-many-requests", () => {
    expect(authErrorMessage(withCode("auth/too-many-requests"))).toMatch(/too many attempts/i);
  });

  it("maps invalid-email", () => {
    expect(authErrorMessage(withCode("auth/invalid-email"))).toMatch(/valid email address/i);
  });

  it("maps missing-password", () => {
    expect(authErrorMessage(withCode("auth/missing-password"))).toMatch(/enter your password/i);
  });

  it("maps user-disabled", () => {
    expect(authErrorMessage(withCode("auth/user-disabled"))).toMatch(
      /account has been disabled/i,
    );
  });

  it("NEVER leaks a raw error message", () => {
    const msg = authErrorMessage(new Error("RequestUnknownException: AppleAuthentication.swift:61"));
    expect(msg).toBe("Something went wrong. Try again.");
    expect(msg).not.toMatch(/swift|Exception/i);
  });

  it("does not throw when the rejection is null or undefined", () => {
    expect(() => authErrorMessage(null)).not.toThrow();
    expect(() => authErrorMessage(undefined)).not.toThrow();
    expect(authErrorMessage(null)).toBe("Something went wrong. Try again.");
  });

  const ALL_MAPPED_CODES = [
    "ERR_REQUEST_UNKNOWN",
    "auth/reauth-failed",
    "auth/invalid-credential",
    "auth/credential-already-in-use",
    "auth/provider-already-linked",
    "auth/requires-recent-login",
    "auth/network-request-failed",
    "auth/too-many-requests",
    "auth/invalid-email",
    "auth/missing-password",
    "auth/user-disabled",
  ];

  it("never tells ANY mapped error to start over because it 'expired'", () => {
    // GIP collapses wrong-password into auth/invalid-credential. Any "expired"
    // copy makes a mistyped password unrecoverable. This guard must cover the
    // whole table, not a hand-picked subset — a defect already shipped once
    // because an earlier version of this guard skipped most of the codes.
    for (const code of ALL_MAPPED_CODES) {
      expect(authErrorMessage(withCode(code))).not.toMatch(/expired/i);
    }
  });
});
