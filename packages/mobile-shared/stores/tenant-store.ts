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
      // Deliberately NOT setTenantId(store.id). A store id and a tenant id
      // are different identifiers; writing one into the other's slot was
      // invisible while GIP's token carried the real tenant claim
      // server-side, but the client now STATES its tenant via
      // X-Acting-Tenant-Id (#686). A store id there fails the FGA
      // membership check and is refused 404 "no store" — while the
      // merchant genuinely is a member. It only surfaced after a restart,
      // because this wrote to storage and hydrate() read it back.
      //
      // The tenant is set explicitly by the sign-in flow, which is the
      // only thing that actually learns it (from the login response).
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
        // No storeId fallback: a WRONG tenant is worse than none. With a
        // fallback the client confidently states a store id as its tenant
        // and gets an unexplained 404; with null it simply has no tenant
        // yet and the sign-in flow resolves one.
        tenantId: tenantId ?? null,
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
