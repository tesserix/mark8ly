import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";

vi.mock("@/app/auth/idp/actions", () => ({
  startCustomerGoogleSignIn: vi.fn(),
}));

import { startCustomerGoogleSignIn } from "@/app/auth/idp/actions";
import { resolveGoogleSignInUrl } from "./google-sign-in";

const startCustomerGoogleSignInMock = vi.mocked(startCustomerGoogleSignIn);

beforeEach(() => {
  startCustomerGoogleSignInMock.mockReset();
});

afterEach(() => {
  vi.clearAllMocks();
});

describe("resolveGoogleSignInUrl", () => {
  const args = {
    storeSlug: "shop",
    intent: "signin" as const,
    dest: "/account" as const,
    origin: "https://shop.mark8ly.com",
  };

  it("calls startCustomerGoogleSignIn and returns its authUrl", async () => {
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

  it("propagates a failure result from startCustomerGoogleSignIn without inventing its own message", async () => {
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
