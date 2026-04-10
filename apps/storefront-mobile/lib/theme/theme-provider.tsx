import { createContext, useContext, type ReactNode } from "react";
import { getMerchantTheme, type MerchantTheme } from "./merchant-theme";

const ThemeContext = createContext<MerchantTheme | null>(null);

const resolvedTheme = getMerchantTheme();

export function MerchantThemeProvider({ children }: { children: ReactNode }) {
  return (
    <ThemeContext.Provider value={resolvedTheme}>
      {children}
    </ThemeContext.Provider>
  );
}

export function useTheme(): MerchantTheme {
  const ctx = useContext(ThemeContext);
  if (!ctx) {
    throw new Error("useTheme must be used within MerchantThemeProvider");
  }
  return ctx;
}
