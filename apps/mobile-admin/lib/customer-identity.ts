/**
 * How a customer is STATED — one rule, one module, so the list row, the
 * detail screen and the Dashboard queue cannot disagree about it.
 *
 * The rule the Dashboard queue already applies (`lib/queue.ts`,
 * `orderToQueueItem`: `order.customer_name || order.customer_email`) is that a
 * customer with no name falls back to their email. That fallback is normal,
 * not exotic — guest checkout and imported customers routinely arrive with no
 * name, and the demo tenant's only real customer is one of them.
 *
 * What the queue gets right and the customer screens did not is the SECOND
 * line. `CustomerRow` fell back to the email for its primary line and then
 * rendered the email again, unconditionally, underneath — so a no-name
 * customer's row printed `mahesh.sangawar@gmail.com` twice, stacked. The
 * detail screen did the same thing a third time, in its back-header.
 *
 * So `subtitle` is OPTIONAL by construction rather than by call-site
 * discipline: it exists only when it carries something `title` does not. A
 * caller that renders `identity.subtitle` unconditionally gets nothing for a
 * no-name customer, which is the correct output — it cannot reintroduce the
 * duplicate by forgetting a guard, because there is no guard to forget.
 */
export interface CustomerIdentitySource {
  first_name?: string;
  last_name?: string;
  email: string;
}

export interface CustomerIdentity {
  /** The line that states WHO this is: the name when there is one, else the email. */
  title: string;
  /**
   * The line beneath, present ONLY when it says something `title` doesn't —
   * i.e. the email, when a real name is carrying the title. `undefined` for a
   * no-name customer, whose email is already the title.
   */
  subtitle?: string;
  /**
   * True when `title` IS the email — i.e. there is no name to state.
   *
   * Exposed because the two cases need different TYPOGRAPHY, not just
   * different strings: a name is prose that can wrap at a space, an email is a
   * single indivisible token that must not be broken mid-address. See the
   * customer detail screen's identity title.
   */
  titleIsEmail: boolean;
}

/**
 * Blank-safe: a `first_name` of `" "` is not a name. The API types both name
 * fields as optional strings, and an imported customer can carry whitespace
 * where a CSV column was empty — `Boolean(" ")` is `true`, so filtering on
 * truthiness alone would title the row with an invisible space and drop the
 * email entirely.
 */
function joinName(first: string | undefined, last: string | undefined): string {
  return [first, last]
    .map((part) => part?.trim() ?? "")
    .filter((part) => part.length > 0)
    .join(" ");
}

export function customerIdentity(source: CustomerIdentitySource): CustomerIdentity {
  const name = joinName(source.first_name, source.last_name);
  if (name) {
    return { title: name, subtitle: source.email, titleIsEmail: false };
  }
  return { title: source.email, titleIsEmail: true };
}
