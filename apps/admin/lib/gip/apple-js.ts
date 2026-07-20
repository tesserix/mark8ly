// Browser-only wrapper around "Sign in with Apple JS".
//
// Mirrors google-gsi.ts: lazily load Apple's SDK, run the popup, and resolve
// with the Apple identity token. The caller exchanges that token via
// signInWithApple (Identity Toolkit signInWithIdp) for a GIP id_token in our
// tenant pool.
//
// Why a separate SDK rather than reusing the Google path: Apple does not
// expose a one-tap credential API, and GIP cannot mint an Apple credential
// on its own — the browser must complete Apple's own OAuth dance first.
//
// Prerequisites that live OUTSIDE this repo (see docs/apple-web-signin.md):
//   1. An Apple *Services ID* — the web client_id. The iOS bundle ID does
//      not work here; Apple treats native and web as different clients.
//   2. That Services ID configured with a return URL pointing at GIP's
//      handler, and its domain verified with Apple.
//   3. A .p8 key turned into a client secret on the GIP apple.com provider.
//      Apple caps that secret at 6 months — it must be rotated or web Apple
//      sign-in breaks with no warning.

import { publicConfig } from "@/lib/config";

const SCRIPT_URL =
  "https://appleid.cdn-apple.com/appleauth/static/jsapi/appleid/1/en_US/appleid.auth.js";

// GIP's hosted OAuth handler. Apple redirects here, not to our own domain,
// which is what keeps per-tenant admin subdomains ({slug}-admin.mark8ly.com)
// out of Apple's domain allow-list — Apple has no wildcard support, so
// registering each tenant subdomain would not scale.
const gipRedirectURI = () =>
  `https://${publicConfig.gipProjectId}.firebaseapp.com/__/auth/handler`;

interface AppleAuthResponse {
  authorization: { id_token: string; code: string; state?: string };
  // Only present on the FIRST authorization, and omitted entirely when the
  // user picks "Hide My Email".
  user?: { name?: { firstName?: string; lastName?: string }; email?: string };
}

interface AppleIDAuth {
  init(opts: {
    clientId: string;
    scope: string;
    redirectURI: string;
    state?: string;
    nonce?: string;
    usePopup: boolean;
  }): void;
  signIn(): Promise<AppleAuthResponse>;
}

declare global {
  interface Window {
    AppleID?: { auth: AppleIDAuth };
  }
}

let scriptPromise: Promise<void> | null = null;

function loadScript(): Promise<void> {
  if (typeof window === "undefined") {
    return Promise.reject(new Error("apple sign-in only available in the browser"));
  }
  if (window.AppleID?.auth) return Promise.resolve();
  if (scriptPromise) return scriptPromise;

  scriptPromise = new Promise((resolve, reject) => {
    const existing = document.querySelector<HTMLScriptElement>(
      `script[src="${SCRIPT_URL}"]`,
    );
    if (existing) {
      existing.addEventListener("load", () => resolve());
      existing.addEventListener("error", () => reject(new Error("apple sdk load failed")));
      return;
    }
    const s = document.createElement("script");
    s.src = SCRIPT_URL;
    s.async = true;
    s.defer = true;
    s.onload = () => resolve();
    s.onerror = () => {
      // Reset so a later attempt can retry rather than reusing a rejected
      // promise for the lifetime of the page.
      scriptPromise = null;
      reject(new Error("apple sdk load failed"));
    };
    document.head.appendChild(s);
  });
  return scriptPromise;
}

/** Cryptographically random nonce, hex-encoded. */
function randomNonce(): string {
  const bytes = new Uint8Array(16);
  crypto.getRandomValues(bytes);
  return Array.from(bytes, (b) => b.toString(16).padStart(2, "0")).join("");
}

/**
 * Runs Apple's popup and returns the identity token plus the nonce it was
 * bound to.
 *
 * The nonce is generated per attempt and passed to signInWithIdp so Apple's
 * token cannot be replayed. The mobile client currently hardcodes an empty
 * nonce (apps/mobile-admin/lib/social-auth.ts) — do not copy that here.
 */
export async function getAppleCredential(): Promise<{
  idToken: string;
  nonce: string;
}> {
  if (!publicConfig.appleServicesId) {
    throw new Error(
      "Apple sign-in is not configured (NEXT_PUBLIC_APPLE_SERVICES_ID missing)",
    );
  }
  if (!publicConfig.gipProjectId) {
    throw new Error("Apple sign-in is not configured (NEXT_PUBLIC_GIP_PROJECT_ID missing)");
  }

  await loadScript();
  if (!window.AppleID?.auth) {
    throw new Error("apple sdk unavailable after load");
  }

  const nonce = randomNonce();
  window.AppleID.auth.init({
    clientId: publicConfig.appleServicesId,
    scope: "name email",
    redirectURI: gipRedirectURI(),
    nonce,
    usePopup: true,
  });

  const res = await window.AppleID.auth.signIn();
  const idToken = res?.authorization?.id_token;
  if (!idToken) {
    throw new Error("apple returned no identity token");
  }
  return { idToken, nonce };
}
