import { create } from "zustand";

export interface AuthUser {
  uid: string;
  email: string;
  displayName: string | null;
}

interface AuthState {
  isAuthenticated: boolean;
  user: AuthUser | null;
  isLoading: boolean;
  setUser: (user: AuthUser | null) => void;
  setLoading: (loading: boolean) => void;
  reset: () => void;
}

const initialState = {
  isAuthenticated: false,
  user: null,
  isLoading: false,
} as const;

export const useAuthStore = create<AuthState>((set) => ({
  ...initialState,
  setUser: (user) =>
    set({
      user,
      isAuthenticated: user !== null,
    }),
  setLoading: (isLoading) => set({ isLoading }),
  reset: () => set({ ...initialState }),
}));
