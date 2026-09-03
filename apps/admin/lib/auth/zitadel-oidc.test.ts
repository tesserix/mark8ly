import { describe, it, expect } from "vitest";
import { isTrustedZitadelHostedUrl, isTrustedCallbackUrl } from "./zitadel-oidc";

// Minor fix (whole-branch review, phase 3a): SignInForm's handoff branch
// did `window.location.assign(data.handoffUrl)` on a server-supplied URL
// with no validation. This pins the one legitimate target: Zitadel's own
// hosted-login origin, taken from the configured issuer.
describe("isTrustedZitadelHostedUrl", () => {
  const issuer = "https://auth.tesserix.app";

  it("accepts a URL on the issuer's own origin", () => {
    expect(isTrustedZitadelHostedUrl("https://auth.tesserix.app/ui/v2/login", issuer)).toBe(
      true,
    );
  });

  it("rejects a URL on a different origin", () => {
    expect(isTrustedZitadelHostedUrl("https://evil.example.com/steal", issuer)).toBe(false);
  });

  it("rejects a non-https URL even on the right host", () => {
    expect(isTrustedZitadelHostedUrl("http://auth.tesserix.app/ui/v2/login", issuer)).toBe(
      false,
    );
  });

  it("rejects a malformed URL instead of throwing", () => {
    expect(isTrustedZitadelHostedUrl("not-a-url", issuer)).toBe(false);
  });

  it("rejects everything when the issuer is unconfigured", () => {
    expect(isTrustedZitadelHostedUrl("https://auth.tesserix.app/ui/v2/login", "")).toBe(false);
  });
});

// Follow-up fix: `callbackUrl` was forwarded to window.location.assign
// with no validation at all — an open redirect on a freshly-authenticated
// session, since /auth/callback's state check only runs if the browser
// ever lands there. The legitimate target is this app's own origin.
describe("isTrustedCallbackUrl", () => {
  const appOrigin = "https://admin.mark8ly.com";

  it("accepts a URL on the app's own origin", () => {
    expect(
      isTrustedCallbackUrl("https://admin.mark8ly.com/auth/callback?state=abc", appOrigin),
    ).toBe(true);
  });

  it("rejects a URL on a foreign origin", () => {
    expect(isTrustedCallbackUrl("https://evil.example.com/steal", appOrigin)).toBe(false);
  });

  it("rejects a malformed URL instead of throwing", () => {
    expect(isTrustedCallbackUrl("not-a-url", appOrigin)).toBe(false);
  });

  it("rejects everything when appOrigin is unconfigured", () => {
    expect(isTrustedCallbackUrl("https://admin.mark8ly.com/auth/callback", "")).toBe(false);
  });
});
