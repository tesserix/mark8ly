import { create } from "zustand";
import AsyncStorage from "@react-native-async-storage/async-storage";

const STORAGE_KEY = "mark8ly_cart_v1";

export interface CartLine {
  productId: string;
  variantId: string;
  handle: string;
  title: string;
  variantTitle: string;
  unitPriceAmount: string;
  currencyCode: string;
  imageUrl: string;
  quantity: number;
}

interface CartState {
  lines: CartLine[];
  hydrated: boolean;
  hydrate: () => Promise<void>;
  add: (line: Omit<CartLine, "quantity">, quantity?: number) => void;
  setQuantity: (variantId: string, quantity: number) => void;
  remove: (variantId: string) => void;
  clear: () => void;
  itemCount: () => number;
  subtotalAmount: () => number;
}

async function persist(lines: CartLine[]) {
  try {
    await AsyncStorage.setItem(STORAGE_KEY, JSON.stringify(lines));
  } catch {
    // Best-effort persistence — losing the cart on app kill is preferable
    // to crashing the app over a storage error.
  }
}

/**
 * Local cart, persisted to AsyncStorage. For signed-in customers we
 * sync this to the server in a follow-up — for the initial launch a
 * local cart already covers the guest checkout flow.
 */
export const useCartStore = create<CartState>((set, get) => ({
  lines: [],
  hydrated: false,

  hydrate: async () => {
    if (get().hydrated) return;
    try {
      const raw = await AsyncStorage.getItem(STORAGE_KEY);
      const parsed = raw ? (JSON.parse(raw) as CartLine[]) : [];
      set({ lines: parsed, hydrated: true });
    } catch {
      set({ hydrated: true });
    }
  },

  add: (line, quantity = 1) => {
    const lines = get().lines.slice();
    const existing = lines.findIndex((l) => l.variantId === line.variantId);
    if (existing >= 0) {
      lines[existing] = { ...lines[existing]!, quantity: lines[existing]!.quantity + quantity };
    } else {
      lines.push({ ...line, quantity });
    }
    set({ lines });
    persist(lines);
  },

  setQuantity: (variantId, quantity) => {
    const lines = get().lines
      .map((l) => (l.variantId === variantId ? { ...l, quantity } : l))
      .filter((l) => l.quantity > 0);
    set({ lines });
    persist(lines);
  },

  remove: (variantId) => {
    const lines = get().lines.filter((l) => l.variantId !== variantId);
    set({ lines });
    persist(lines);
  },

  clear: () => {
    set({ lines: [] });
    persist([]);
  },

  itemCount: () => get().lines.reduce((sum, l) => sum + l.quantity, 0),
  subtotalAmount: () =>
    get().lines.reduce((sum, l) => sum + l.quantity * Number(l.unitPriceAmount), 0),
}));
