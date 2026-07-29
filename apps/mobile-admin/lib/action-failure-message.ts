/**
 * What a merchant is told when a direct mutation fails.
 *
 * Before this existed, a failed activate / archive / block / close produced
 * ONE failure haptic and nothing else. On a phone with no console that is
 * undiagnosable: after the refetch the row simply looks unchanged, and the
 * merchant cannot tell whether they mis-tapped, the request never left the
 * device, or the server refused it outright. On Customers it was worse still
 * — `CustomerRow` carries no status badge, so under the default "All" filter
 * a successful block and a failed block render identically.
 *
 * Two rules govern the copy, and they are the point of the module:
 *
 *  1. **The action is always named.** Every branch produces a `title` of the
 *     form "Couldn't <action>". A message that only says something went
 *     wrong is not worth the pixels — it tells the merchant nothing they did
 *     not already infer from the haptic.
 *  2. **The server's own sentence wins wherever it is readable.** Several of
 *     these messages carry facts the client cannot reconstruct:
 *     `segment_in_use` names how many campaigns still point at the segment
 *     (see `apperrors.SegmentInUse`), and paraphrasing it would throw away
 *     the only actionable part. Server prose is passed through verbatim —
 *     only a closing full stop is appended, never a re-casing or a re-wording
 *     that could contradict what the server said.
 *
 * The one distinction the client must draw for ITSELF is "your device could
 * not reach the server" versus "the server answered and refused". The
 * merchant's next move differs — wait and retry, versus stop and look at the
 * record — so collapsing the two would defeat the whole surface.
 */

export interface ActionFailureMessage {
  /** What the merchant tried and could not do. Always names the action. */
  title: string;
  /** Why. The server's own prose verbatim when it is fit to read. */
  detail: string;
}

/**
 * `ApiError` as it arrives, matched STRUCTURALLY rather than with
 * `instanceof`.
 *
 * `instanceof` would bind this module to one physical copy of
 * `@repo/mobile-shared/api/client`. This workspace has repeatedly ended up
 * with two live copies of the same package under jest (see the five separate
 * `moduleNameMapper` pins in jest.config.js, each added after a duplicate
 * install broke a suite), and a duplicate here would fail SILENTLY: every
 * genuine server refusal would be reported to the merchant as "couldn't
 * reach the server" — the exact confusion this file exists to prevent.
 * Matching on shape cannot be broken that way, and it costs one fewer module
 * edge from a file that is imported by every list screen.
 */
interface ApiErrorShape {
  status: number;
  code: string;
  message: string;
}

function asApiError(error: unknown): ApiErrorShape | null {
  if (typeof error !== "object" || error === null) return null;
  const candidate = error as { name?: unknown; status?: unknown; code?: unknown; message?: unknown };
  if (candidate.name !== "ApiError") return null;
  if (typeof candidate.status !== "number" || typeof candidate.code !== "string") return null;
  return {
    status: candidate.status,
    code: candidate.code,
    message: typeof candidate.message === "string" ? candidate.message : "",
  };
}

/** Nothing came back at all — offline, DNS, a dropped connection. */
const UNREACHABLE =
  "Your device couldn't reach the server. Check your connection and try again.";

/**
 * The connection opened but nothing answered inside the deadline
 * (`ApiError(408, "timeout", …)` from the client's own AbortController).
 * Deliberately worded apart from `UNREACHABLE`: a Knative cold start behaves
 * very differently from a flat-dead network, and "try again" is much more
 * likely to work here.
 */
const TIMED_OUT =
  "The server took too long to answer. Check your connection and try again.";

/**
 * Copy for the codes whose meaning is stable, used when the server's own
 * prose is missing or unfit to show. These do NOT override readable server
 * prose — they are the floor, not the ceiling.
 */
const DETAIL_FOR_CODE: Readonly<Record<string, string>> = {
  invalid_transition: "Its status has already changed. Pull down to refresh, then try again.",
  campaign_not_draft: "It is no longer a draft, so it can no longer be changed.",
  segment_in_use: "It is still used by at least one campaign.",
  gift_card_expired: "It has expired.",
  no_active_store: "No store is selected. Pick a store and try again.",
};

function detailForStatus(status: number): string {
  if (status === 401) return "Your session has ended. Sign in again to continue.";
  if (status === 403) return "You don't have permission to do that.";
  if (status === 404) return "It no longer exists. Pull down to refresh.";
  if (status === 429) return "Too many requests. Wait a moment, then try again.";
  if (status >= 500) return "The server ran into a problem. Try again in a moment.";
  return "The server refused it. Pull down to refresh, then try again.";
}

/**
 * Is this server message fit for a merchant to read?
 *
 * Deliberately conservative. The cost of wrongly REJECTING good prose is a
 * slightly vaguer fallback; the cost of wrongly ACCEPTING bad prose is a zod
 * field path or a proxy stack trace rendered in an error banner on a
 * merchant's phone.
 */
function isMerchantReadable(message: string): boolean {
  const trimmed = message.trim();
  if (trimmed.length < 8 || trimmed.length > 240) return false;
  const words = trimmed.split(/\s+/);
  // Two words or fewer is never an explanation — it is a bare code
  // ("segment_in_use"), a short HTTP status text ("Bad Request", "Not Found")
  // or the client's own 401 placeholder ("Access denied"). All are true and
  // all are useless next to copy that says what to do.
  if (words.length < 3) return false;
  // Title Case throughout is an HTTP reason phrase, not prose. `request()`
  // falls back to `res.statusText` whenever the error body isn't JSON, so
  // "Too Many Requests" and "Internal Server Error" reach here routinely —
  // and each one would displace copy that actually tells the merchant what
  // to do. Every message this API writes deliberately (see
  // pkg/apperrors) is a lower-case sentence fragment, so a single
  // non-capitalised word is enough to tell the two apart.
  if (words.every((word) => /^[^a-z]*[A-Z]/.test(word))) return false;
  // Template or markup that never got interpolated.
  if (/[{}<>]/.test(trimmed)) return false;
  // A zod-style field path: "data.0.title: Required".
  if (/^\S+\s*:\s/.test(trimmed)) return false;
  return true;
}

/** Closes the server's sentence without touching a character of it. */
function asSentence(message: string): string {
  const trimmed = message.trim();
  return /[.!?]$/.test(trimmed) ? trimmed : `${trimmed}.`;
}

/**
 * @param action A lower-case verb phrase naming what the merchant tried, in
 *   the shape it reads after "Couldn't " — e.g. `"archive this product"`,
 *   `"unblock this customer"`, `"close this ticket"`.
 */
export function describeActionFailure(error: unknown, action: string): ActionFailureMessage {
  const title = `Couldn't ${action}`;
  const api = asApiError(error);

  // Anything that is not shaped like an ApiError never made it past the
  // transport: a fetch TypeError, a DNS failure, a thrown string. The request
  // may or may not have reached the server, which is precisely why the copy
  // talks about the DEVICE's connection rather than claiming nothing changed.
  if (!api) return { title, detail: UNREACHABLE };

  if (api.status === 408 || api.code === "timeout") return { title, detail: TIMED_OUT };

  // The server's own words, wherever they are fit to read. 5xx is excluded
  // (that is our bug, and its message is written for us, not the merchant)
  // and so is 401, whose message is a client-side placeholder that would
  // otherwise displace the one instruction that helps: sign in again.
  if (api.status < 500 && api.status !== 401 && isMerchantReadable(api.message)) {
    return { title, detail: asSentence(api.message) };
  }

  const mapped = api.status < 500 ? DETAIL_FOR_CODE[api.code] : undefined;
  if (mapped) return { title, detail: mapped };

  return { title, detail: detailForStatus(api.status) };
}
