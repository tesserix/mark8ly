import { create } from "zustand";
import { tokenStorage } from "../auth/token-storage";
import type { Store } from "../api/types";

interface TenantStoreState {
  tenantId: string | null;
  activeStore: Store | null;
  stores: Store[];
  hydrated: boolean;
  setTenantId: (id: string | null) => void;
  setActiveStore: (store: Store | null) => void;
  setStores: (stores: Store[]) => void;
  hydrate: () => Promise<void>;
  /**
   * Drops the currently selected store and forgets it from secure
   * storage. Used when the API rejects the active store id (membership
   * revoked, store deleted) so the next render re-runs the resolver.
   */
  clearActiveStore: () => Promise<void>;
  clear: () => Promise<void>;
}

export const useTenantStore = create<TenantStoreState>((set, get) => ({
  tenantId: null,
  activeStore: null,
  stores: [],
  hydrated: false,
  setTenantId: (id) => {
    set({ tenantId: id });
    if (id) tokenStorage.setTenantId(id).catch(() => undefined);
  },
  setActiveStore: (store) => {
    set({ activeStore: store });
    if (store) {
      tokenStorage.setStoreId(store.id).catch(() => undefined);
      tokenStorage.setTenantId(store.id).catch(() => undefined);
      tokenStorage.setActiveStore(store).catch(() => undefined);
    }
  },
  setStores: (stores) => set({ stores }),
  clearActiveStore: async () => {
    set({ activeStore: null, tenantId: null });
    await tokenStorage.clearAll();
  },
  hydrate: async () => {
    if (get().hydrated) return;
    try {
      const [tenantId, storeId, activeStore] = await Promise.all([
        tokenStorage.getTenantId(),
        tokenStorage.getStoreId(),
        tokenStorage.getActiveStore(),
      ]);
      // The remembered id is authoritative — a persisted Store that no longer
      // matches it is stale (store switched, membership revoked) and the
      // resolver has to re-decide.
      const restored = activeStore && activeStore.id === storeId ? activeStore : null;
      set({
        tenantId: tenantId ?? storeId ?? null,
        activeStore: restored,
        hydrated: true,
      });
    } catch {
      set({ hydrated: true });
    }
  },
  clear: async () => {
    set({ tenantId: null, activeStore: null, stores: [] });
    await tokenStorage.clearAll();
  },
}));
