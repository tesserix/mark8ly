import { create } from "zustand";

/** Why the user was signed out involuntarily. Rendered once on /login. */
export type AuthNotice = "no-session" | "access-denied";

interface AuthNoticeState {
  notice: AuthNotice | null;
  setNotice: (notice: AuthNotice) => void;
  clearNotice: () => void;
}

/**
 * Carries the reason for an involuntary sign-out across the redirect to
 * /login. Deliberately not persisted: a stale reason must never surface on a
 * later, unrelated sign-in attempt.
 */
export const useAuthNoticeStore = create<AuthNoticeState>((set) => ({
  notice: null,
  setNotice: (notice) => set({ notice }),
  clearNotice: () => set({ notice: null }),
}));
