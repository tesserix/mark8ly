"use server";

// Server action for storefront customer account creation.
// Reuses the same session-minting logic as customerSignIn — the only
// difference is that GIP signUp was called on the client instead of
// signInWithPassword. By the time we get here the GIP account already
// exists and we just need to set the cookie + register the profile.

import { customerSignIn } from "@/app/sign-in/actions";

interface CustomerSignUpInput {
  idToken: string;
  uid: string;
  storeSlug: string;
}

type Result =
  | { ok: true }
  | { ok: false; code: string; message: string };

export async function customerSignUp(
  input: CustomerSignUpInput,
): Promise<Result> {
  // Delegate to the sign-in action — the flow is identical once the
  // GIP account exists. The sign-in action sets the cookie and
  // registers the customer profile in marketplace-api.
  return customerSignIn(input);
}
