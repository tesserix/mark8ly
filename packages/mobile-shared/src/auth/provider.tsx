import React, {
  createContext,
  useContext,
  useEffect,
  useCallback,
  type ReactNode,
} from "react";
import { useAuthStore, type AuthUser } from "../stores/auth-store";
import {
  saveToken,
  saveUser,
  clearAuthStorage,
  getToken,
  getSavedUser,
} from "./token-storage";

interface AuthContextValue {
  signIn: (token: string, user: AuthUser) => Promise<void>;
  signOut: () => Promise<void>;
}

const AuthContext = createContext<AuthContextValue | null>(null);

interface AuthProviderProps {
  children: ReactNode;
}

export function AuthProvider({ children }: AuthProviderProps) {
  const { setUser, setLoading } = useAuthStore();

  // Hydrate auth state from secure store on mount
  useEffect(() => {
    let cancelled = false;

    async function hydrate() {
      setLoading(true);
      try {
        const [token, user] = await Promise.all([getToken(), getSavedUser()]);
        if (!cancelled && token && user) {
          setUser(user);
        }
      } finally {
        if (!cancelled) {
          setLoading(false);
        }
      }
    }

    void hydrate();

    return () => {
      cancelled = true;
    };
  }, [setUser, setLoading]);

  const signIn = useCallback(
    async (token: string, user: AuthUser): Promise<void> => {
      await Promise.all([saveToken(token), saveUser(user)]);
      setUser(user);
    },
    [setUser]
  );

  const signOut = useCallback(async (): Promise<void> => {
    await clearAuthStorage();
    setUser(null);
  }, [setUser]);

  return (
    <AuthContext.Provider value={{ signIn, signOut }}>
      {children}
    </AuthContext.Provider>
  );
}

export function useAuth(): AuthContextValue {
  const ctx = useContext(AuthContext);
  if (!ctx) {
    throw new Error("useAuth must be used within an AuthProvider");
  }
  return ctx;
}
