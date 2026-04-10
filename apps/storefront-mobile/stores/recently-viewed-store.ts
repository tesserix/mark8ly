import { create } from "zustand";

export interface RecentProduct {
  handle: string;
  title: string;
  priceAmount: number;
  currencyCode: string;
  imageUrl?: string;
}

const MAX_RECENT = 20;

interface RecentlyViewedState {
  products: RecentProduct[];
  addProduct: (product: RecentProduct) => void;
  clear: () => void;
}

export const useRecentlyViewedStore = create<RecentlyViewedState>()((set) => ({
  products: [],

  addProduct: (product) =>
    set((state) => {
      const filtered = state.products.filter((p) => p.handle !== product.handle);
      const updated = [product, ...filtered];
      return { products: updated.slice(0, MAX_RECENT) };
    }),

  clear: () => set({ products: [] }),
}));
