import { describe, it, expect, vi, beforeEach } from "vitest";

const sessionState = vi.hoisted(() => ({
  headerMap: new Map<string, string>([
    ["x-session-user-id", "u1"],
    ["x-session-tenant-id", "t1"],
    ["x-session-store-id", "s1"],
  ]),
}));

vi.mock("next/headers", () => ({
  headers: async () => sessionState.headerMap,
}));

vi.mock("next/cache", () => ({
  revalidatePath: vi.fn(),
}));

vi.mock("@/lib/api/platform-api", async () => {
  const actual = await vi.importActual<typeof import("@/lib/api/platform-api")>(
    "@/lib/api/platform-api",
  );
  return {
    ...actual,
    deleteTenantAccount: vi.fn(),
  };
});

import { deleteTenantAccountAction } from "./actions";
import { deleteTenantAccount, PlatformApiError } from "@/lib/api/platform-api";

const mockedDelete = deleteTenantAccount as unknown as ReturnType<typeof vi.fn>;

const FULL_SESSION = new Map<string, string>([
  ["x-session-user-id", "u1"],
  ["x-session-tenant-id", "t1"],
  ["x-session-store-id", "s1"],
]);

describe("deleteTenantAccountAction", () => {
  beforeEach(() => {
    mockedDelete.mockReset();
    sessionState.headerMap = new Map(FULL_SESSION);
  });

  it("calls deleteTenantAccount(tenantId, userId) and returns { ok: true } on valid confirmation", async () => {
    mockedDelete.mockResolvedValue(undefined);

    const result = await deleteTenantAccountAction("DELETE");

    expect(result).toEqual({ ok: true });
    expect(mockedDelete).toHaveBeenCalledTimes(1);
    expect(mockedDelete).toHaveBeenCalledWith("t1", "u1");
  });

  it("returns validation error and does not call the client on wrong confirmation", async () => {
    const result = await deleteTenantAccountAction("delete");

    expect(result).toEqual({
      ok: false,
      code: "validation",
      message: "Confirmation text does not match.",
    });
    expect(mockedDelete).not.toHaveBeenCalled();
  });

  it("returns no_session when session headers are missing", async () => {
    sessionState.headerMap = new Map<string, string>();

    const result = await deleteTenantAccountAction("DELETE");

    expect(result).toEqual({
      ok: false,
      code: "no_session",
      message: "Session expired. Please sign in again.",
    });
    expect(mockedDelete).not.toHaveBeenCalled();
  });

  it("maps PlatformApiError to { ok:false, code, message }", async () => {
    mockedDelete.mockRejectedValue(
      new PlatformApiError(403, "forbidden", "Only the tenant owner can delete the account."),
    );

    const result = await deleteTenantAccountAction("DELETE");

    expect(result).toEqual({
      ok: false,
      code: "forbidden",
      message: "Only the tenant owner can delete the account.",
    });
  });
});
