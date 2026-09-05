import { useTenantStore } from "@repo/mobile-shared/stores/tenant-store";
import { tokenStorage } from "@repo/mobile-shared/auth/token-storage";

jest.mock("expo-secure-store", () => {
  const mem: Record<string, string> = {};
  return {
    __mem: mem,
    getItemAsync: jest.fn(async (k: string) => mem[k] ?? null),
    setItemAsync: jest.fn(async (k: string, v: string) => {
      mem[k] = v;
    }),
    deleteItemAsync: jest.fn(async (k: string) => {
      delete mem[k];
    }),
  };
});

const store = (id: string) => ({
  id,
  name: "S",
  slug: "s",
  country_code: "IN",
  currency_code: "INR",
  status: "active",
});

const freshLaunch = () =>
  useTenantStore.setState({ tenantId: null, activeStore: null, stores: [], hydrated: false });

beforeEach(() => {
  const mem = (jest.requireMock("expo-secure-store") as { __mem: Record<string, string> }).__mem;
  for (const k of Object.keys(mem)) delete mem[k];
  freshLaunch();
});

// THE regression, and it only shows across a restart — which is why it went
// unnoticed. Selecting a store persisted that STORE's id into the tenant
// slot; the next cold launch hydrates it as the tenant.
//
// Harmless while GIP's token carried the real tenant claim server-side. The
// moment the client sends X-Acting-Tenant-Id (#686) it states a store id as
// its tenant, fails every FGA membership check, and is refused 404 "no
// store" while genuinely being a member — a bug that would look like a
// backend fault, on the second launch only.
it("does not persist the store id into the tenant slot", async () => {
  await tokenStorage.setTenantId("tenant-uuid");
  useTenantStore.getState().setActiveStore(store("store-uuid"));
  await Promise.resolve();

  expect(await tokenStorage.getTenantId()).toBe("tenant-uuid");
});

it("hydrates the real tenant id after a cold launch, not the store id", async () => {
  await tokenStorage.setTenantId("tenant-uuid");
  useTenantStore.getState().setActiveStore(store("store-uuid"));
  await Promise.resolve();

  freshLaunch();
  await useTenantStore.getState().hydrate();

  expect(useTenantStore.getState().tenantId).toBe("tenant-uuid");
  expect(useTenantStore.getState().activeStore?.id).toBe("store-uuid");
});

// A wrong tenant is worse than no tenant: the client then sends a header it
// believes is correct and gets an unexplained 404, instead of resolving the
// tenant properly at sign-in.
it("hydrates a null tenant rather than falling back to the store id", async () => {
  useTenantStore.getState().setActiveStore(store("store-uuid"));
  await Promise.resolve();

  freshLaunch();
  await useTenantStore.getState().hydrate();

  expect(useTenantStore.getState().tenantId).toBeNull();
});

it("keeps the tenant id in memory when the active store changes", () => {
  useTenantStore.getState().setTenantId("tenant-uuid");
  useTenantStore.getState().setActiveStore(store("store-a"));
  useTenantStore.getState().setActiveStore(store("store-b"));

  expect(useTenantStore.getState().tenantId).toBe("tenant-uuid");
});
