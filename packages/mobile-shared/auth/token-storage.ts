import * as SecureStore from "expo-secure-store";

const KEYS = {
  TENANT_ID: "mark8ly_tenant_id",
  STORE_ID: "mark8ly_store_id",
  DEVICE_ID: "mark8ly_device_id",
  PUSH_ENABLED: "mark8ly_push_enabled",
  PUSH_TOKEN_ID: "mark8ly_push_token_id",
} as const;

export const tokenStorage = {
  getTenantId: () => SecureStore.getItemAsync(KEYS.TENANT_ID),
  setTenantId: (id: string) => SecureStore.setItemAsync(KEYS.TENANT_ID, id),
  getStoreId: () => SecureStore.getItemAsync(KEYS.STORE_ID),
  setStoreId: (id: string) => SecureStore.setItemAsync(KEYS.STORE_ID, id),
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
    // The registration belongs to the signed-out user; drop its id so the
    // next user registers fresh. The on/off preference stays (device-level).
    await SecureStore.deleteItemAsync(KEYS.PUSH_TOKEN_ID);
  },
};
