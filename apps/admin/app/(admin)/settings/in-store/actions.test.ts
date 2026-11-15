import { beforeEach, describe, expect, it, vi } from "vitest";

const ctx = vi.fn();
const setInStoreSettings = vi.fn();
vi.mock("next/cache", () => ({ revalidatePath: vi.fn() }));
vi.mock("@/lib/auth/serverSession", () => ({ getServerSessionContext: () => ctx() }));
vi.mock("@/lib/api/instore-api", () => ({ setInStoreSettings: (...a: unknown[]) => setInStoreSettings(...a) }));

import { setInStoreSettingAction } from "./actions";

const store = { id: "s1" };
const saved = { cashier_discounts_enabled: false, in_store_enabled: true };
beforeEach(() => {
  setInStoreSettings.mockReset().mockResolvedValue({ ok: true, data: saved });
});

describe("setInStoreSettingAction", () => {
  it("refuses staff", async () => {
    ctx.mockResolvedValue({ role: "staff", currentStore: store, userId: "u", tenantId: "t", email: "" });
    expect(await setInStoreSettingAction({ in_store_enabled: true })).toMatchObject({ ok: false });
    expect(setInStoreSettings).not.toHaveBeenCalled();
  });

  it("saves only what changed, on the current store", async () => {
    ctx.mockResolvedValue({ role: "owner", currentStore: store, userId: "u", tenantId: "t", email: "o@x.test" });
    expect(await setInStoreSettingAction({ in_store_enabled: true })).toEqual({ ok: true, settings: saved });
    expect(setInStoreSettings).toHaveBeenCalledWith("s1", { in_store_enabled: true }, expect.objectContaining({ userId: "u" }));
  });
});
