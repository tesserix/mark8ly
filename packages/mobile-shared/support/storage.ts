// secureStoreKV — a KVStorage adapter over expo-secure-store, used by both
// apps to persist the support send-outbox so queued messages survive an app
// cold start. SecureStore keys allow only [A-Za-z0-9._-], so keys are
// sanitised at the boundary (the outbox uses a ":" separator that's valid
// for AsyncStorage/MMKV but not SecureStore).
//
// Note: SecureStore caps values at ~2KB on iOS. A typical outbox (a few
// short queued messages) is well under that; a very large offline backlog
// would need AsyncStorage/MMKV instead.
import * as SecureStore from "expo-secure-store";

import type { KVStorage } from "./outbox";

const safeKey = (k: string): string => k.replace(/[^A-Za-z0-9._-]/g, "_");

export const secureStoreKV: KVStorage = {
  getItem: (key) => SecureStore.getItemAsync(safeKey(key)),
  setItem: (key, value) => SecureStore.setItemAsync(safeKey(key), value),
  removeItem: (key) => SecureStore.deleteItemAsync(safeKey(key)),
};
