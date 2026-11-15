import { beforeEach, describe, expect, it, vi } from "vitest";

const ctx = vi.fn();
const setInStoreSettings = vi.fn();
vi.mock("next/headers", () => ({ headers: vi.fn() }));
vi.mock("next/cache", () => ({ revalidatePath: vi.fn() }));
vi.mock("@/lib/auth/serverSession", () => ({
  canInviteMembers: () => true,
  getServerSessionContext: () => ctx(),
}));
vi.mock("@/lib/api/instore-api", () => ({ setInStoreSettings: (...a: unknown[]) => setInStoreSettings(...a) }));

import { setCashierDiscountsAction } from "./actions";

const store = { id: "s1" };
beforeEach(() => {
  setInStoreSettings.mockReset().mockResolvedValue({ ok: true, data: { cashier_discounts_enabled: true } });
});

describe("setCashierDiscountsAction", () => {
  it("refuses staff", async () => {
    ctx.mockResolvedValue({ role: "staff", currentStore: store, userId: "u", tenantId: "t", email: "" });
    expect(await setCashierDiscountsAction(true)).toMatchObject({ ok: false });
    expect(setInStoreSettings).not.toHaveBeenCalled();
  });

  it("saves for an admin, on the current store", async () => {
    ctx.mockResolvedValue({ role: "admin", currentStore: store, userId: "u", tenantId: "t", email: "a@x.test" });
    expect(await setCashierDiscountsAction(true)).toEqual({ ok: true, enabled: true });
    expect(setInStoreSettings).toHaveBeenCalledWith("s1", { cashier_discounts_enabled: true }, expect.objectContaining({ userId: "u", tenantId: "t" }));
  });
});
