import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";

let headerMap: Map<string, string>;

vi.mock("next/headers", () => ({
  headers: async () => ({
    get: (key: string) => headerMap.get(key.toLowerCase()) ?? null,
  }),
}));

vi.mock("@/lib/auth/auth-bff", async () => {
  const actual = await vi.importActual<typeof import("@/lib/auth/auth-bff")>(
    "@/lib/auth/auth-bff",
  );
  return {
    ...actual,
    startZitadelIDPIntent: vi.fn(),
  };
});

import { startZitadelIDPIntent, AuthBffError } from "@/lib/auth/auth-bff";
import { startAdminGoogleSignIn } from "./actions";

const startZitadelIDPIntentMock = vi.mocked(startZitadelIDPIntent);

beforeEach(() => {
  // The real canonical host — the admin /login page (and this flow) is
  // reachable only here, not on a per-tenant `{slug}-admin.mark8ly.com`
  // subdomain.
  headerMap = new Map([["host", "admin.mark8ly.com"]]);
});

afterEach(() => {
  vi.clearAllMocks();
});

describe("startAdminGoogleSignIn", () => {
  it("builds the return_url from the request host and the supplied auth_request_id", async () => {
    startZitadelIDPIntentMock.mockResolvedValue("https://zitadel.example/idp/authorize");

    const result = await startAdminGoogleSignIn("ar-1");

    expect(result).toEqual({ ok: true, authUrl: "https://zitadel.example/idp/authorize" });
    expect(startZitadelIDPIntentMock).toHaveBeenCalledWith(
      "https://admin.mark8ly.com/auth/idp/finish?auth_request_id=ar-1",
    );
  });

  it("refuses without an auth_request_id rather than sending an empty one", async () => {
    const result = await startAdminGoogleSignIn("");

    expect(result.ok).toBe(false);
    expect(startZitadelIDPIntentMock).not.toHaveBeenCalled();
  });

  it("returns a generic message and logs the code when auth-bff rejects the request", async () => {
    startZitadelIDPIntentMock.mockRejectedValue(
      new AuthBffError(400, "invalid_return_url", "http_400"),
    );
    const errSpy = vi.spyOn(console, "error").mockImplementation(() => {});

    const result = await startAdminGoogleSignIn("ar-1");

    expect(result.ok).toBe(false);
    if (!result.ok) {
      expect(result.message).not.toContain("invalid_return_url");
      expect(result.message.length).toBeGreaterThan(0);
    }
    errSpy.mockRestore();
  });

  it("returns a generic message when there is no request host", async () => {
    headerMap = new Map();

    const result = await startAdminGoogleSignIn("ar-1");

    expect(result.ok).toBe(false);
    expect(startZitadelIDPIntentMock).not.toHaveBeenCalled();
  });
});
