// Browser-only wrapper around Google Identity Services (gsi/client).
//
// Mirrors the onboarding app's helper. Loads the GSI script lazily, then
// triggers the popup/one-tap and resolves with the Google credential JWT.
// The caller exchanges that credential via signInWithGoogle (Identity
// Toolkit signInWithIdp) for a GIP id_token in our tenant pool.

import { publicConfig } from "@/lib/config";

const SCRIPT_URL = "https://accounts.google.com/gsi/client";

interface GsiCredentialResponse {
  credential: string;
}

interface GsiNotification {
  isNotDisplayed(): boolean;
  isSkippedMoment(): boolean;
  isDismissedMoment(): boolean;
  getNotDisplayedReason(): string;
  getSkippedReason(): string;
  getDismissedReason(): string;
}

interface GsiAccountsId {
  initialize(opts: {
    client_id: string;
    callback: (resp: GsiCredentialResponse) => void;
    auto_select?: boolean;
    cancel_on_tap_outside?: boolean;
    use_fedcm_for_prompt?: boolean;
  }): void;
  prompt(callback?: (n: GsiNotification) => void): void;
  cancel(): void;
}

declare global {
  interface Window {
    google?: {
      accounts: {
        id: GsiAccountsId;
      };
    };
  }
}

let scriptPromise: Promise<void> | null = null;

function loadScript(): Promise<void> {
  if (typeof window === "undefined") {
    return Promise.reject(new Error("google sign-in only available in the browser"));
  }
  if (window.google?.accounts?.id) return Promise.resolve();
  if (scriptPromise) return scriptPromise;

  scriptPromise = new Promise((resolve, reject) => {
    const existing = document.querySelector<HTMLScriptElement>(
      `script[src="${SCRIPT_URL}"]`,
    );
    if (existing) {
      existing.addEventListener("load", () => resolve());
      existing.addEventListener("error", () => reject(new Error("gsi load failed")));
      return;
    }
    const s = document.createElement("script");
    s.src = SCRIPT_URL;
    s.async = true;
    s.defer = true;
    s.onload = () => resolve();
    s.onerror = () => reject(new Error("gsi load failed"));
    document.head.appendChild(s);
  });
  return scriptPromise;
}

export async function getGoogleCredential(): Promise<{ credential: string }> {
  if (!publicConfig.googleClientId) {
    throw new Error(
      "Google sign-in is not configured (NEXT_PUBLIC_GOOGLE_CLIENT_ID missing)",
    );
  }
  await loadScript();
  const gsi = window.google!.accounts.id;

  return new Promise((resolve, reject) => {
    gsi.initialize({
      client_id: publicConfig.googleClientId,
      callback: (resp) => {
        if (resp?.credential) {
          resolve({ credential: resp.credential });
        } else {
          reject(new Error("google: empty credential"));
        }
      },
      auto_select: false,
      cancel_on_tap_outside: true,
      use_fedcm_for_prompt: true,
    });
    gsi.prompt((n) => {
      if (n.isNotDisplayed() || n.isSkippedMoment() || n.isDismissedMoment()) {
        reject(
          new Error(
            n.getNotDisplayedReason() ||
              n.getSkippedReason() ||
              n.getDismissedReason() ||
              "google: prompt dismissed",
          ),
        );
      }
    });
  });
}
