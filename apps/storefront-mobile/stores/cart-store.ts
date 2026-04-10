import { create } from "zustand";

export interface CartItem {
  productId: string;
  variantId: string;
  handle: string;
  title: string;
  priceAmount: number;
  currencyCode: string;
  qty: number;
  imageUrl?: string;
}

interface CartState {
  items: CartItem[];
  count: () => number;
  subtotal: () => string;
  addItem: (item: Omit<CartItem, "qty"> & { qty?: number }) => void;
  removeItem: (variantId: string) => void;
  updateQty: (variantId: string, qty: number) => void;
  clear: () => void;
}

export const useCartStore = create<CartState>()((set, get) => ({
  items: [],

  count: () => get().items.reduce((sum, item) => sum + item.qty, 0),

  subtotal: () =>
    get()
      .items.reduce((sum, item) => sum + item.priceAmount * item.qty, 0)
      .toFixed(2),

  addItem: (incoming) => {
    const qty = incoming.qty ?? 1;
    set((state) => {
      const existing = state.items.find((i) => i.variantId === incoming.variantId);
      if (existing) {
        return {
          items: state.items.map((i) =>
            i.variantId === incoming.variantId ? { ...i, qty: i.qty + qty } : i
          ),
        };
      }
      return {
        items: [...state.items, { ...incoming, qty }],
      };
    });
  },

  removeItem: (variantId) =>
    set((state) => ({
      items: state.items.filter((i) => i.variantId !== variantId),
    })),

  updateQty: (variantId, qty) => {
    if (qty <= 0) {
      get().removeItem(variantId);
      return;
    }
    set((state) => ({
      items: state.items.map((i) =>
        i.variantId === variantId ? { ...i, qty } : i
      ),
    }));
  },

  clear: () => set({ items: [] }),
}));
