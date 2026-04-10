import * as SecureStore from "expo-secure-store";

const KEYS = {
  TENANT_ID: "mark8ly_tenant_id",
  STORE_ID: "mark8ly_store_id",
  DEVICE_ID: "mark8ly_device_id",
} as const;

export const tokenStorage = {
  getTenantId: () => SecureStore.getItemAsync(KEYS.TENANT_ID),
  setTenantId: (id: string) => SecureStore.setItemAsync(KEYS.TENANT_ID, id),
  getStoreId: () => SecureStore.getItemAsync(KEYS.STORE_ID),
  setStoreId: (id: string) => SecureStore.setItemAsync(KEYS.STORE_ID, id),
  getDeviceId: () => SecureStore.getItemAsync(KEYS.DEVICE_ID),
  setDeviceId: (id: string) => SecureStore.setItemAsync(KEYS.DEVICE_ID, id),
  clearAll: async () => {
    await SecureStore.deleteItemAsync(KEYS.TENANT_ID);
    await SecureStore.deleteItemAsync(KEYS.STORE_ID);
  },
};
