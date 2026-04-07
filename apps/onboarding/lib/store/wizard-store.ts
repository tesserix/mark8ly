"use client";

// Zustand store for the onboarding wizard.
//
// Lives entirely client-side. Server actions don't read from it — the wizard
// passes whatever fields it needs as explicit args. The store is the source
// of truth for the UI.
//
// Persisted to sessionStorage so a refresh in the middle of the wizard
// doesn't blow away progress. Persistence is per-tab (sessionStorage, not
// localStorage) — closing the tab is "I'm done with this attempt."

import { create } from "zustand";
import { persist, createJSONStorage } from "zustand/middleware";

export type WizardStep = "email" | "verify" | "business" | "submitting" | "done";

interface WizardState {
  step: WizardStep;
  // Step 1
  email: string;
  sessionId: string;
  // Step 2 — populated as a side effect of OTP verification
  gipUid: string;
  gipIdToken: string;
  gipRefreshToken: string;
  gipExpiresAt: number; // unix seconds
  // Step 3
  businessName: string;
  slug: string;
  countryCode: string;
  currencyCode: string;
  timezone: string;
  // Step 4 — populated by the complete call
  tenantId: string;

  setStep: (step: WizardStep) => void;
  setEmail: (email: string) => void;
  setSessionId: (id: string) => void;
  setGip: (gip: {
    uid: string;
    idToken: string;
    refreshToken: string;
    expiresIn: number;
  }) => void;
  setBusinessFields: (fields: {
    businessName: string;
    slug: string;
    countryCode: string;
    currencyCode: string;
    timezone: string;
  }) => void;
  setTenantId: (id: string) => void;
  reset: () => void;
}

const initial = {
  step: "email" as WizardStep,
  email: "",
  sessionId: "",
  gipUid: "",
  gipIdToken: "",
  gipRefreshToken: "",
  gipExpiresAt: 0,
  businessName: "",
  slug: "",
  countryCode: "",
  currencyCode: "",
  timezone: "",
  tenantId: "",
};

export const useWizardStore = create<WizardState>()(
  persist(
    (set) => ({
      ...initial,
      setStep: (step) => set({ step }),
      setEmail: (email) => set({ email }),
      setSessionId: (sessionId) => set({ sessionId }),
      setGip: (gip) =>
        set({
          gipUid: gip.uid,
          gipIdToken: gip.idToken,
          gipRefreshToken: gip.refreshToken,
          gipExpiresAt: Math.floor(Date.now() / 1000) + gip.expiresIn,
        }),
      setBusinessFields: (fields) =>
        set({
          businessName: fields.businessName,
          slug: fields.slug,
          countryCode: fields.countryCode,
          currencyCode: fields.currencyCode,
          timezone: fields.timezone,
        }),
      setTenantId: (tenantId) => set({ tenantId }),
      reset: () => set(initial),
    }),
    {
      name: "m8-onboarding",
      storage: createJSONStorage(() => sessionStorage),
    },
  ),
);
