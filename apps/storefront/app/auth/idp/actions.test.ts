import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";

let headerMap: Map<string, string>;

vi.mock("next/headers", () => ({
  headers: async () => ({
    get: (key: string) => headerMap.get(key.toLowerCase()) ?? null,
  }),
}));

vi.mock("@/lib/auth/auth-bff-customer", async () => {
  const actual = await vi.importActual<
    typeof import("@/lib/auth/auth-bff-customer")
  >("@/lib/auth/auth-bff-customer");
  return { ...actual, startCustomerIDPIntent: vi.fn() };
});

import { startCustomerIDPIntent, AuthBffCustomerError } from "@/lib/auth/auth-bff-customer";
import { startCustomerGoogleSignIn } from "./actions";

const startCustomerIDPIntentMock = vi.mocked(startCustomerIDPIntent);

beforeEach(() => {
  headerMap = new Map([["host", "shop.mark8ly.com"]]);
  startCustomerIDPIntentMock.mockReset();
});

afterEach(() => {
  vi.clearAllMocks();
});

describe("startCustomerGoogleSignIn", () => {
  it("builds the finish route URL on this request's own host, carrying the requested dest", async () => {
    startCustomerIDPIntentMock.mockResolvedValue("https://zitadel.example.com/idp/auth/1");

    await startCustomerGoogleSignIn("/account");

    expect(startCustomerIDPIntentMock).toHaveBeenCalledWith(
      "https://shop.mark8ly.com/auth/idp/finish?dest=%2Faccount",
    );
  });

  it("carries a different allowlisted dest through unchanged", async () => {
    startCustomerIDPIntentMock.mockResolvedValue("https://zitadel.example.com/idp/auth/1");

    await startCustomerGoogleSignIn("/account/security");

    expect(startCustomerIDPIntentMock).toHaveBeenCalledWith(
      "https://shop.mark8ly.com/auth/idp/finish?dest=%2Faccount%2Fsecurity",
    );
  });

  it("falls back to /account for a dest outside the allowlist", async () => {
    startCustomerIDPIntentMock.mockResolvedValue("https://zitadel.example.com/idp/auth/1");

    // @ts-expect-error — exercising the runtime guard against a caller
    // that bypasses the type system.
    await startCustomerGoogleSignIn("https://evil.example.com");

    expect(startCustomerIDPIntentMock).toHaveBeenCalledWith(
      "https://shop.mark8ly.com/auth/idp/finish?dest=%2Faccount",
    );
  });

  it("returns { ok: true, authUrl } on success", async () => {
    startCustomerIDPIntentMock.mockResolvedValue("https://zitadel.example.com/idp/auth/1");

    const result = await startCustomerGoogleSignIn("/account");

    expect(result).toEqual({
      ok: true,
      authUrl: "https://zitadel.example.com/idp/auth/1",
    });
  });

  it("never lets AuthBffCustomerError's internal detail reach the returned message", async () => {
    startCustomerIDPIntentMock.mockRejectedValue(
      new AuthBffCustomerError(503, "zitadel_unavailable"),
    );

    const result = await startCustomerGoogleSignIn("/account");

    expect(result.ok).toBe(false);
    if (!result.ok) {
      expect(result.message).not.toContain("503");
      expect(result.message).not.toContain("zitadel_unavailable");
      expect(result.message.toLowerCase()).not.toContain("auth-bff");
    }
  });

  it("returns a generic failure when no request host is available", async () => {
    headerMap = new Map();

    const result = await startCustomerGoogleSignIn("/account");

    expect(result.ok).toBe(false);
    expect(startCustomerIDPIntentMock).not.toHaveBeenCalled();
  });
});
