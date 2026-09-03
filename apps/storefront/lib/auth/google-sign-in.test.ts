import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";

vi.mock("@/app/auth/idp/actions", () => ({
  startCustomerGoogleSignIn: vi.fn(),
}));

import { startCustomerGoogleSignIn } from "@/app/auth/idp/actions";
import {
  buildTrampolineUrl,
  resolveGoogleSignInUrl,
} from "./google-sign-in";

const startCustomerGoogleSignInMock = vi.mocked(startCustomerGoogleSignIn);

beforeEach(() => {
  delete process.env.NEXT_PUBLIC_AUTH_PROVIDER;
  startCustomerGoogleSignInMock.mockReset();
});

afterEach(() => {
  delete process.env.NEXT_PUBLIC_AUTH_PROVIDER;
  vi.clearAllMocks();
});

describe("buildTrampolineUrl", () => {
  it("builds mark8ly.com/auth/google with return_to/store_slug/intent — unchanged shape", () => {
    const url = buildTrampolineUrl({
      storeSlug: "the-bondi-store",
      intent: "signin",
      dest: "/account",
      origin: "https://the-bondi-store.mark8ly.com",
    });

    const parsed = new URL(url);
    expect(parsed.origin + parsed.pathname).toBe("https://mark8ly.com/auth/google");
    expect(parsed.searchParams.get("return_to")).toBe(
      "https://the-bondi-store.mark8ly.com/account",
    );
    expect(parsed.searchParams.get("store_slug")).toBe("the-bondi-store");
    expect(parsed.searchParams.get("intent")).toBe("signin");
  });

  it("carries the intent through unchanged for signup and link", () => {
    const signup = new URL(
      buildTrampolineUrl({
        storeSlug: "shop",
        intent: "signup",
        dest: "/account",
        origin: "https://shop.mark8ly.com",
      }),
    );
    const link = new URL(
      buildTrampolineUrl({
        storeSlug: "shop",
        intent: "link",
        dest: "/account/security",
        origin: "https://shop.mark8ly.com",
      }),
    );

    expect(signup.searchParams.get("intent")).toBe("signup");
    expect(link.searchParams.get("intent")).toBe("link");
    expect(link.searchParams.get("return_to")).toBe(
      "https://shop.mark8ly.com/account/security",
    );
  });
});

describe("resolveGoogleSignInUrl", () => {
  const args = {
    storeSlug: "shop",
    intent: "signin" as const,
    dest: "/account" as const,
    origin: "https://shop.mark8ly.com",
  };

  it("flag unset: resolves to the trampoline URL and never calls startCustomerGoogleSignIn", async () => {
    const result = await resolveGoogleSignInUrl(args);

    expect(result.ok).toBe(true);
    if (result.ok) {
      expect(result.url).toContain("mark8ly.com/auth/google");
    }
    expect(startCustomerGoogleSignInMock).not.toHaveBeenCalled();
  });

  it('flag "zitadel": calls startCustomerGoogleSignIn and returns its authUrl, never the trampoline', async () => {
    process.env.NEXT_PUBLIC_AUTH_PROVIDER = "zitadel";
    startCustomerGoogleSignInMock.mockResolvedValue({
      ok: true,
      authUrl: "https://zitadel.example.com/idp/authorize/abc",
    });

    const result = await resolveGoogleSignInUrl(args);

    expect(startCustomerGoogleSignInMock).toHaveBeenCalledWith("/account");
    expect(result).toEqual({
      ok: true,
      url: "https://zitadel.example.com/idp/authorize/abc",
    });
  });

  it('an unrecognised flag value (e.g. "Zitadel") stays on the trampoline path', async () => {
    process.env.NEXT_PUBLIC_AUTH_PROVIDER = "Zitadel";

    const result = await resolveGoogleSignInUrl(args);

    expect(result.ok).toBe(true);
    if (result.ok) {
      expect(result.url).toContain("mark8ly.com/auth/google");
    }
    expect(startCustomerGoogleSignInMock).not.toHaveBeenCalled();
  });

  it("propagates a failure result from startCustomerGoogleSignIn without inventing its own message", async () => {
    process.env.NEXT_PUBLIC_AUTH_PROVIDER = "zitadel";
    startCustomerGoogleSignInMock.mockResolvedValue({
      ok: false,
      message: "Google sign-in is temporarily unavailable. Please try again shortly.",
    });

    const result = await resolveGoogleSignInUrl(args);

    expect(result).toEqual({
      ok: false,
      message: "Google sign-in is temporarily unavailable. Please try again shortly.",
    });
  });
});
