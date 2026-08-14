import * as SecureStore from "expo-secure-store";
import { storeSchema, type Store } from "../api/schemas/stores";

const KEYS = {
  TENANT_ID: "mark8ly_tenant_id",
  STORE_ID: "mark8ly_store_id",
  ACTIVE_STORE: "mark8ly_active_store",
  DEVICE_ID: "mark8ly_device_id",
  PUSH_ENABLED: "mark8ly_push_enabled",
  PUSH_TOKEN_ID: "mark8ly_push_token_id",
} as const;

export const tokenStorage = {
  getTenantId: () => SecureStore.getItemAsync(KEYS.TENANT_ID),
  setTenantId: (id: string) => SecureStore.setItemAsync(KEYS.TENANT_ID, id),
  getStoreId: () => SecureStore.getItemAsync(KEYS.STORE_ID),
  setStoreId: (id: string) => SecureStore.setItemAsync(KEYS.STORE_ID, id),

  // The whole Store, not just its id, so a cold launch can render before
  // GET /stores answers. Re-validated on read: a payload written by an older
  // build is dropped rather than trusted.
  getActiveStore: async (): Promise<Store | null> => {
    const raw = await SecureStore.getItemAsync(KEYS.ACTIVE_STORE);
    if (!raw) return null;
    try {
      const parsed = storeSchema.safeParse(JSON.parse(raw));
      return parsed.success ? parsed.data : null;
    } catch {
      return null;
    }
  },
  setActiveStore: (store: Store) =>
    SecureStore.setItemAsync(KEYS.ACTIVE_STORE, JSON.stringify(store)),
  clearActiveStore: () => SecureStore.deleteItemAsync(KEYS.ACTIVE_STORE),
  getDeviceId: () => SecureStore.getItemAsync(KEYS.DEVICE_ID),
  setDeviceId: (id: string) => SecureStore.setItemAsync(KEYS.DEVICE_ID, id),

  // Push preference is a DEVICE-level opt-out (default on), so it survives
  // sign-out. The server-side token id is captured on registration so the
  // settings toggle can delete the registration when the user opts out.
  getPushEnabled: async () =>
    (await SecureStore.getItemAsync(KEYS.PUSH_ENABLED)) !== "0",
  setPushEnabled: (on: boolean) =>
    SecureStore.setItemAsync(KEYS.PUSH_ENABLED, on ? "1" : "0"),
  getPushTokenId: () => SecureStore.getItemAsync(KEYS.PUSH_TOKEN_ID),
  setPushTokenId: (id: string) => SecureStore.setItemAsync(KEYS.PUSH_TOKEN_ID, id),
  clearPushTokenId: () => SecureStore.deleteItemAsync(KEYS.PUSH_TOKEN_ID),

  clearAll: async () => {
    await SecureStore.deleteItemAsync(KEYS.TENANT_ID);
    await SecureStore.deleteItemAsync(KEYS.STORE_ID);
    await SecureStore.deleteItemAsync(KEYS.ACTIVE_STORE);
    // The registration belongs to the signed-out user; drop its id so the
    // next user registers fresh. The on/off preference stays (device-level).
    await SecureStore.deleteItemAsync(KEYS.PUSH_TOKEN_ID);
  },
};
