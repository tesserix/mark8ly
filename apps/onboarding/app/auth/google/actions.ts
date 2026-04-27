// apps/onboarding/app/auth/google/actions.ts
"use server";

import { mintExchangeCode } from "@repo/ui/auth/exchange-code";

const SESSION_ENCRYPT_KEY = process.env.SESSION_ENCRYPT_KEY ?? "";

export interface MintInput {
  idToken: string;
  storeSlug: string;
  returnTo: string;
  intent: "signin" | "signup";
}

export type MintResult =
  | { ok: true; redirectUrl: string }
  | { ok: false; error: string };

const SLUG_RE = /^[a-z0-9][a-z0-9-]*[a-z0-9]$/;
const HOST_RE = /^[a-zA-Z0-9.-]+$/;

export async function mintCustomerExchangeCode(
  input: MintInput,
): Promise<MintResult> {
  if (!SESSION_ENCRYPT_KEY) {
    return { ok: false, error: "Session key not configured." };
  }
  if (!SLUG_RE.test(input.storeSlug) || input.storeSlug.length > 63) {
    return { ok: false, error: "Invalid store_slug." };
  }
  if (input.intent !== "signin" && input.intent !== "signup") {
    return { ok: false, error: "Invalid intent." };
  }

  let returnUrl: URL;
  try {
    returnUrl = new URL(input.returnTo);
  } catch {
    return { ok: false, error: "Invalid return_to." };
  }
  if (returnUrl.protocol !== "https:" && returnUrl.protocol !== "http:") {
    return { ok: false, error: "Invalid return_to protocol." };
  }
  const returnHost = returnUrl.hostname;
  if (!HOST_RE.test(returnHost) || returnHost.length > 253) {
    return { ok: false, error: "Invalid return_to host." };
  }

  const code = mintExchangeCode(
    {
      idToken: input.idToken,
      storeSlug: input.storeSlug,
      returnTo: input.returnTo,
      intent: input.intent,
    },
    SESSION_ENCRYPT_KEY,
    30,
  );

  // Build the storefront /auth/google/finish URL on the returnTo's host.
  // The finish handler will reject the code if storeSlug doesn't match
  // the resolved store for that host (defense in depth).
  const finishUrl = new URL("/auth/google/finish", `${returnUrl.protocol}//${returnUrl.host}`);
  finishUrl.searchParams.set("code", code);
  return { ok: true, redirectUrl: finishUrl.toString() };
}
