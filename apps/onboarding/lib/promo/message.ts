/**
 * What the promo field says, for every answer the server can give (#620).
 *
 * Its own module, and pure, so the sentences can be tested without rendering
 * anything. The sentences ARE the feature here: a merchant holding a working
 * code who is told it is invalid abandons a code that would have worked, and
 * that failure is invisible to any test that only checks the happy path.
 *
 * The rule every branch follows: state only what the server actually returned.
 * The number of days comes from the promo definition, so a code's terms can
 * change in the console without a deploy here.
 */
import { signupCopy } from "../copy/signup";

/** The server's answer to "what would this code grant?" */
export interface PromoCheck {
  valid: boolean;
  trial_extension_days: number;
  reject_reason: string;
}

/** What the field should show, and whether it reads as good news. */
export interface PromoMessage {
  text: string;
  tone: "accepted" | "refused" | "unknown";
}

/**
 * The message for `check`.
 *
 * `unknown` is deliberately distinct from `refused`: it means we could not
 * ASK, and it must never render as "invalid". A merchant holding a good code
 * being told it is bad because a service was down is the one outcome here that
 * costs a signup, and it is also the one that would never be noticed.
 */
export function promoMessage(check: PromoCheck | null): PromoMessage | null {
  if (check === null) {
    return { text: signupCopy.promoCheckFailed, tone: "unknown" };
  }

  if (check.valid) {
    return {
      text:
        check.trial_extension_days > 0
          ? signupCopy.promoAccepted(check.trial_extension_days)
          : // Accepted but grants nothing we can describe. Saying so beats
            // inventing a benefit the response does not contain.
            signupCopy.promoAcceptedNoTerms,
      tone: "accepted",
    };
  }

  switch (check.reject_reason) {
    case "redeem_in_billing":
      return { text: signupCopy.promoRedeemInBilling, tone: "refused" };
    case "max_redemptions_reached":
      return { text: signupCopy.promoMaxRedemptions, tone: "refused" };
    case "max_per_email_reached":
      return { text: signupCopy.promoAlreadyUsed, tone: "refused" };
    default:
      // Everything else — including invalid_or_expired, which deliberately
      // merges "no such code" with "expired" so the field cannot be used to
      // confirm which codes are real.
      return { text: signupCopy.promoInvalid, tone: "refused" };
  }
}
