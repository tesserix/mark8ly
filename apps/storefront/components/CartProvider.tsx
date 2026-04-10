"use client";

// apps/storefront/components/CartProvider.tsx
//
// Client-side cart state backed by localStorage. Wraps children in
// a React context so any client island can call useCart(). Keyed
// per store slug to prevent cross-store cart leaks.

import {
  createContext,
  useCallback,
  useContext,
  useEffect,
  useReducer,
  type ReactNode,
} from "react";
import {
  addItem,
  removeItem,
  setQty,
  subtotal,
  count,
  type CartItem,
} from "@/lib/cart";

// ---------------------------------------------------------------------------
// Context shape
// ---------------------------------------------------------------------------

interface CartContextValue {
  items: CartItem[];
  add: (item: CartItem) => void;
  remove: (productId: string, variantId: string) => void;
  updateQty: (productId: string, variantId: string, qty: number) => void;
  clear: () => void;
  count: number;
  subtotal: number;
}

const CartContext = createContext<CartContextValue | null>(null);

export function useCart(): CartContextValue {
  const ctx = useContext(CartContext);
  if (!ctx) throw new Error("useCart must be used inside <CartProvider>");
  return ctx;
}

// ---------------------------------------------------------------------------
// Reducer
// ---------------------------------------------------------------------------

type Action =
  | { type: "ADD"; item: CartItem }
  | { type: "REMOVE"; productId: string; variantId: string }
  | { type: "SET_QTY"; productId: string; variantId: string; qty: number }
  | { type: "CLEAR" }
  | { type: "HYDRATE"; items: CartItem[] };

function reducer(state: CartItem[], action: Action): CartItem[] {
  switch (action.type) {
    case "ADD":
      return addItem(state, action.item);
    case "REMOVE":
      return removeItem(state, action.productId, action.variantId);
    case "SET_QTY":
      return setQty(state, action.productId, action.variantId, action.qty);
    case "CLEAR":
      return [];
    case "HYDRATE":
      return action.items;
  }
}

// ---------------------------------------------------------------------------
// Provider
// ---------------------------------------------------------------------------

function storageKey(storeSlug: string): string {
  return `mark8ly.cart.${storeSlug}`;
}

function readFromStorage(slug: string): CartItem[] {
  if (typeof window === "undefined") return [];
  try {
    const raw = localStorage.getItem(storageKey(slug));
    if (!raw) return [];
    const parsed: unknown = JSON.parse(raw);
    if (!Array.isArray(parsed)) return [];
    return parsed as CartItem[];
  } catch {
    return [];
  }
}

function writeToStorage(slug: string, items: CartItem[]): void {
  if (typeof window === "undefined") return;
  try {
    localStorage.setItem(storageKey(slug), JSON.stringify(items));
  } catch {
    // Storage full or blocked — silently degrade.
  }
}

export interface CartProviderProps {
  storeSlug: string;
  children: ReactNode;
}

export function CartProvider({ storeSlug, children }: CartProviderProps) {
  const [items, dispatch] = useReducer(reducer, []);

  // Hydrate from localStorage on mount (client-only).
  useEffect(() => {
    dispatch({ type: "HYDRATE", items: readFromStorage(storeSlug) });
  }, [storeSlug]);

  // Persist every change back to localStorage.
  useEffect(() => {
    writeToStorage(storeSlug, items);
  }, [storeSlug, items]);

  const add = useCallback(
    (item: CartItem) => dispatch({ type: "ADD", item }),
    [],
  );
  const remove = useCallback(
    (productId: string, variantId: string) =>
      dispatch({ type: "REMOVE", productId, variantId }),
    [],
  );
  const updateQty = useCallback(
    (productId: string, variantId: string, qty: number) =>
      dispatch({ type: "SET_QTY", productId, variantId, qty }),
    [],
  );
  const clear = useCallback(() => dispatch({ type: "CLEAR" }), []);

  const value: CartContextValue = {
    items,
    add,
    remove,
    updateQty,
    clear,
    count: count(items),
    subtotal: subtotal(items),
  };

  return <CartContext value={value}>{children}</CartContext>;
}
