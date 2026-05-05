// Thin browser-only wrapper around Google Identity Services (gsi/client).
//
// We do NOT pull in the firebase JS SDK. Google's GSI script is ~30 KB and
// already cached on most browsers — it gives us a Google credential
// (a JWT id_token from Google), which we then exchange via Identity
// Toolkit signInWithIdp for a GIP id_token in our tenant pool.
//
// Usage:
//
//   const { credential } = await getGoogleCredential();
//   const gip = await signInWithGoogle(credential);
//
// The script is loaded on demand the first time `getGoogleCredential` is
// called so the home/onboarding pages don't pay the cost up front.

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
  renderButton(
    parent: HTMLElement,
    options: {
      type?: "standard" | "icon";
      theme?: "outline" | "filled_blue" | "filled_black";
      size?: "large" | "medium" | "small";
      text?: "signin_with" | "signup_with" | "continue_with" | "signin";
      shape?: "rectangular" | "pill" | "circle" | "square";
      logo_alignment?: "left" | "center";
      width?: number | string;
      locale?: string;
    },
  ): void;
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

/**
 * Trigger the Google one-tap / popup prompt and resolve with the Google
 * credential JWT. Caller is responsible for exchanging it via
 * signInWithGoogle (Identity Toolkit signInWithIdp).
 */
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
      // The user dismissed or the browser blocked one-tap. Surface a
      // clean error so the UI can stay on the form.
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

export interface MountGoogleButtonOptions {
  buttonContainer: HTMLElement;
  onCredential: (credential: string) => void;
  onError?: (err: Error) => void;
  buttonText?: "signin_with" | "signup_with" | "continue_with" | "signin";
  /** When true, also fire One-Tap as a passive enhancement; failures are silent. */
  tryOneTap?: boolean;
}

/**
 * Render the official "Sign in with Google" button into `buttonContainer`
 * and resolve `onCredential` with the Google id_token JWT once the user
 * clicks through the popup. Use this on pages where the user is expected
 * to take a deliberate action (e.g. the cross-tenant trampoline) — One-Tap
 * is unreliable for explicit sign-ins because FedCM-era browsers may
 * silently return "unknown_reason" with no recoverable UI.
 *
 * Pass `tryOneTap: true` to additionally fire One-Tap as a convenience for
 * users with an active Google session; its failures are swallowed so the
 * rendered button always remains the primary path.
 *
 * Returns a teardown function that cancels both flows.
 */
export async function mountGoogleButton(
  opts: MountGoogleButtonOptions,
): Promise<() => void> {
  if (!publicConfig.googleClientId) {
    throw new Error(
      "Google sign-in is not configured (NEXT_PUBLIC_GOOGLE_CLIENT_ID missing)",
    );
  }
  await loadScript();
  const gsi = window.google!.accounts.id;

  let cancelled = false;

  gsi.initialize({
    client_id: publicConfig.googleClientId,
    callback: (resp) => {
      if (cancelled) return;
      if (resp?.credential) {
        opts.onCredential(resp.credential);
      } else {
        opts.onError?.(new Error("google: empty credential"));
      }
    },
    auto_select: false,
    cancel_on_tap_outside: true,
    use_fedcm_for_prompt: true,
  });

  gsi.renderButton(opts.buttonContainer, {
    type: "standard",
    theme: "outline",
    size: "large",
    text: opts.buttonText ?? "continue_with",
    shape: "pill",
    logo_alignment: "left",
    width: 280,
  });

  if (opts.tryOneTap) {
    try {
      gsi.prompt();
    } catch {
      // ignore — button is the primary path
    }
  }

  return () => {
    cancelled = true;
    try {
      gsi.cancel();
    } catch {
      // ignore
    }
  };
}
