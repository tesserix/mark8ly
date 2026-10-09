"use client";

// apps/storefront/components/StoreProvider.tsx
//
// The resolved store, for client islands that render its name. The root
// layout already fetches the store and branding on every request; this
// hands the name down so a client page does not have to re-resolve it
// — or forget to, which is how the cart page shipped a header reading
// "Store" instead of the merchant's name.

import { createContext, useContext, type ReactNode } from "react";

interface StoreContextValue {
  /** The merchant's display name, or null when no store resolved. */
  name: string | null;
}

const StoreContext = createContext<StoreContextValue>({ name: null });

export function useStore(): StoreContextValue {
  return useContext(StoreContext);
}

export function StoreProvider({
  name,
  children,
}: {
  name: string | null;
  children: ReactNode;
}) {
  return (
    <StoreContext.Provider value={{ name }}>{children}</StoreContext.Provider>
  );
}
