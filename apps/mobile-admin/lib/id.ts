const ALPHABET = "ABCDEFGHIJKLMNOPQRSTUVWXYZabcdefghijklmnopqrstuvwxyz0123456789_-";

/**
 * A random id from [A-Za-z0-9_-]. Used for the refund `refund_request_id` —
 * the coordinator's idempotency scope (RefundOrderRequest). It must be stable
 * across retries of the SAME logical refund (generate once per refund attempt
 * and reuse), so callers hold it for the lifetime of a refund composer, not
 * per network call. Not security-sensitive — Math.random is sufficient.
 */
export function randomId(length = 21): string {
  let out = "";
  for (let i = 0; i < length; i += 1) {
    out += ALPHABET[Math.floor(Math.random() * ALPHABET.length)];
  }
  return out;
}
