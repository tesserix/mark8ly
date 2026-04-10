import { createContext, useContext, useEffect, useState, type ReactNode } from "react";
import type { FirebaseAuthTypes } from "@react-native-firebase/auth";
import { type GIPAuth, createGIPAuth } from "./gip";
import { tokenStorage } from "./token-storage";

interface AuthState {
  user: FirebaseAuthTypes.User | null;
  loading: boolean;
  signIn: (email: string, password: string) => Promise<void>;
  signOut: () => Promise<void>;
  getToken: () => Promise<string | null>;
}

const AuthContext = createContext<AuthState | null>(null);

interface AuthProviderProps {
  tenantId: string;
  children: ReactNode;
}

export function AuthProvider({ tenantId, children }: AuthProviderProps) {
  const [gipAuth] = useState<GIPAuth>(() => createGIPAuth({ tenantId }));
  const [user, setUser] = useState<FirebaseAuthTypes.User | null>(null);
  const [loading, setLoading] = useState(true);

  useEffect(() => {
    const unsubscribe = gipAuth.onAuthStateChanged((firebaseUser) => {
      setUser(firebaseUser);
      setLoading(false);
    });
    return unsubscribe;
  }, [gipAuth]);

  const signIn = async (email: string, password: string) => {
    await gipAuth.signIn(email, password);
  };

  const signOut = async () => {
    await tokenStorage.clearAll();
    await gipAuth.signOut();
  };

  const getToken = () => gipAuth.getIdToken();

  return (
    <AuthContext.Provider value={{ user, loading, signIn, signOut, getToken }}>
      {children}
    </AuthContext.Provider>
  );
}

export function useAuth(): AuthState {
  const ctx = useContext(AuthContext);
  if (!ctx) throw new Error("useAuth must be used within AuthProvider");
  return ctx;
}
