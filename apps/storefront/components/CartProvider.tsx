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
  useMemo,
  useReducer,
  useRef,
  useState,
  type ReactNode,
} from "react";
import {
  addItem,
  removeItem,
  setQty,
  subtotal,
  count,
  type CartItem,
  type CartLineKey,
} from "@/lib/cart";
import { readCart, writeCart } from "@/lib/cart-storage";
import { placeCartHolds, releaseCartHolds } from "@/lib/api/checkout-api";

// ---------------------------------------------------------------------------
// Context shape
// ---------------------------------------------------------------------------

interface CartContextValue {
  items: CartItem[];
  add: (item: CartItem) => void;
  /**
   * key is lineKey(item) (#964) — NOT a variant id. Two personalisations
   * of one variant are two lines, so a variant no longer identifies a
   * row. Call sites get the key from the item they are rendering.
   */
  remove: (key: CartLineKey) => void;
  updateQty: (key: CartLineKey, qty: number) => void;
  clear: () => void;
  count: number;
  subtotal: number;
  /**
   * ISO timestamp when this cart's stock holds lapse, or null when nothing
   * is reserved (#232). Null is a normal state, not an error: a failed hold
   * is silent by design, because checkout enforces availability anyway.
   */
  holdExpiresAt: string | null;
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
  | { type: "REMOVE"; key: CartLineKey }
  | { type: "SET_QTY"; key: CartLineKey; qty: number }
  | { type: "CLEAR" }
  | { type: "HYDRATE"; items: CartItem[] };

function reducer(state: CartItem[], action: Action): CartItem[] {
  switch (action.type) {
    case "ADD":
      return addItem(state, action.item);
    case "REMOVE":
      return removeItem(state, action.key);
    case "SET_QTY":
      return setQty(state, action.key, action.qty);
    case "CLEAR":
      return [];
    case "HYDRATE":
      return action.items;
  }
}

// ---------------------------------------------------------------------------
// Provider
// ---------------------------------------------------------------------------

export interface CartProviderProps {
  storeSlug: string;
  children: ReactNode;
}

const PERSIST_DEBOUNCE_MS = 250;

// Long enough that a qty stepper collapses into one call, short enough that
// a shopper who adds and immediately opens checkout is already holding.
const HOLD_SYNC_DEBOUNCE_MS = 700;

export function CartProvider({ storeSlug, children }: CartProviderProps) {
  const [items, dispatch] = useReducer(reducer, []);
  // Gate the persist effect until after hydration has run — otherwise
  // the first render (items=[]) overwrites a non-empty stored cart in
  // the milliseconds before HYDRATE lands.
  const [hydrated, setHydrated] = useState(false);
  // When the current cart's stock holds expire, for the checkout countdown.
  // Null means "no hold" — either nothing is reserved yet, or the last
  // attempt failed, which is not an error the shopper needs to see.
  const [holdExpiresAt, setHoldExpiresAt] = useState<string | null>(null);
  const heldRef = useRef(false);
  const persistTimerRef = useRef<ReturnType<typeof setTimeout> | null>(null);

  // Hydrate from localStorage on mount (client-only).
  useEffect(() => {
    dispatch({ type: "HYDRATE", items: readCart(storeSlug) });
    setHydrated(true);
  }, [storeSlug]);

  // Persist every change back to localStorage, debounced so rapid
  // edits (qty stepper, bulk add) collapse into a single write.
  useEffect(() => {
    if (!hydrated) return;
    if (persistTimerRef.current !== null) {
      clearTimeout(persistTimerRef.current);
    }
    persistTimerRef.current = setTimeout(() => {
      writeCart(storeSlug, items);
      persistTimerRef.current = null;
    }, PERSIST_DEBOUNCE_MS);
    return () => {
      if (persistTimerRef.current !== null) {
        clearTimeout(persistTimerRef.current);
      }
    };
  }, [storeSlug, items, hydrated]);

  // Flush any pending write when the tab is being hidden/closed so a
  // last-minute mutation before navigation isn't lost to the debounce.
  useEffect(() => {
    if (!hydrated) return;
    const flush = () => {
      if (persistTimerRef.current !== null) {
        clearTimeout(persistTimerRef.current);
        persistTimerRef.current = null;
        writeCart(storeSlug, items);
      }
    };
    const onVisibility = () => {
      if (document.visibilityState === "hidden") flush();
    };
    window.addEventListener("pagehide", flush);
    document.addEventListener("visibilitychange", onVisibility);
    return () => {
      window.removeEventListener("pagehide", flush);
      document.removeEventListener("visibilitychange", onVisibility);
    };
  }, [storeSlug, items, hydrated]);

  // #232 — mirror the cart to server-side stock holds.
  //
  // Holds are placed AT CART-ADD for the full TTL: the shopper who adds
  // first keeps the unit. Debounced for the same reason the persist effect
  // is — a qty stepper should produce one hold call, not one per click —
  // and the API is idempotent per (cart, variant), so re-posting the whole
  // cart refreshes rather than stacking.
  //
  // Deliberately best-effort and silent. A failed hold must never block
  // shopping: checkout enforces availability regardless (#230), so the worst
  // case is the shopper finds out at checkout instead of now. Blocking the
  // add would turn a backend blip into a store that cannot sell.
  useEffect(() => {
    if (!hydrated) return;

    const variants = items
      .filter((i) => i.variantId && i.qty > 0)
      .map((i) => ({ variantId: i.variantId, qty: i.qty }));

    if (variants.length === 0) {
      // Emptied cart: give the units back rather than making the next
      // shopper wait out the TTL.
      if (heldRef.current) {
        heldRef.current = false;
        setHoldExpiresAt(null);
        void releaseCartHolds(storeSlug);
      }
      return;
    }

    const timer = setTimeout(() => {
      void placeCartHolds(storeSlug, variants).then((res) => {
        if (!res) return;
        heldRef.current = true;
        setHoldExpiresAt(res.expires_at);
      });
    }, HOLD_SYNC_DEBOUNCE_MS);
    return () => clearTimeout(timer);
  }, [storeSlug, items, hydrated]);

  const add = useCallback(
    (item: CartItem) => dispatch({ type: "ADD", item }),
    [],
  );
  const remove = useCallback(
    (key: CartLineKey) => dispatch({ type: "REMOVE", key }),
    [],
  );
  const updateQty = useCallback(
    (key: CartLineKey, qty: number) => dispatch({ type: "SET_QTY", key, qty }),
    [],
  );
  const clear = useCallback(() => dispatch({ type: "CLEAR" }), []);

  const value = useMemo<CartContextValue>(
    () => ({
      items,
      add,
      remove,
      updateQty,
      clear,
      count: count(items),
      subtotal: subtotal(items),
      holdExpiresAt,
    }),
    [items, add, remove, updateQty, clear, holdExpiresAt],
  );

  return <CartContext value={value}>{children}</CartContext>;
}
