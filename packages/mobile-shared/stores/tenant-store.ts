import { create } from "zustand";
import type { Store } from "../api/types";

interface TenantStoreState {
  tenantId: string | null;
  activeStore: Store | null;
  stores: Store[];
  setTenantId: (id: string) => void;
  setActiveStore: (store: Store) => void;
  setStores: (stores: Store[]) => void;
}

export const useTenantStore = create<TenantStoreState>((set) => ({
  tenantId: null,
  activeStore: null,
  stores: [],
  setTenantId: (id) => set({ tenantId: id }),
  setActiveStore: (store) => set({ activeStore: store }),
  setStores: (stores) => set({ stores }),
}));
