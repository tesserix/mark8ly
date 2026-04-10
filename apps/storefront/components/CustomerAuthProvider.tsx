"use client";

import { createContext, useContext, type ReactNode } from "react";

interface CustomerAuthState {
  isAuthenticated: boolean;
  displayName: string | null;
  email: string | null;
  loginUrl: string;
  logoutUrl: string;
}

const CustomerAuthContext = createContext<CustomerAuthState>({
  isAuthenticated: false,
  displayName: null,
  email: null,
  loginUrl: "#",
  logoutUrl: "#",
});

export function useCustomerAuth(): CustomerAuthState {
  return useContext(CustomerAuthContext);
}

interface CustomerAuthProviderProps {
  children: ReactNode;
  value: CustomerAuthState;
}

export function CustomerAuthProvider({
  children,
  value,
}: CustomerAuthProviderProps) {
  return (
    <CustomerAuthContext.Provider value={value}>
      {children}
    </CustomerAuthContext.Provider>
  );
}
