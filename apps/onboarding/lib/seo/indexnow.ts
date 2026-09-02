/**
 * IndexNow ownership key.
 *
 * ── WHAT THIS DOES, AND WHAT IT DOES NOT ─────────────────────────────────
 * This is the ownership proof, and ONLY the ownership proof. On its own it
 * changes nothing: no search engine is notified of anything by the mere
 * existence of a key file. IndexNow is a push protocol — a URL is submitted
 * when something POSTs it to a participating endpoint, and nothing in this
 * repository does that yet. Adding the key is step one of two; until step
 * two exists, Bing, Yandex and Naver learn about our changes exactly as
 * slowly as they did before (tesserix/mark8ly#603).
 *
 * Do not describe this as "IndexNow support" in a changelog. It is the
 * prerequisite for it.
 *
 * When the submitter is written, it needs:
 *
 *   POST https://api.indexnow.org/indexnow
 *   { "host": "mark8ly.com",
 *     "key": INDEXNOW_KEY,
 *     "urlList": ["https://mark8ly.com/...", ...] }
 *
 * driven off a real content change — the sitemap's LAST_MODIFIED map is the
 * natural trigger — not off every deploy. Submitting unchanged URLs on a
 * schedule is what gets a host rate-limited (429) and is precisely the
 * meaningless-signal problem #603 was about in the first place.
 *
 * ── WHERE THE FILE LIVES ─────────────────────────────────────────────────
 * The protocol's Option 1: a UTF-8 text file named `{key}.txt` at the root
 * of the host, containing the key and nothing else. So the served path is
 * `/e34fd3ce8f958568aee4d51b05b53271.txt`, and the file is a static asset in
 * `public/` rather than a route — a verification file that a search engine
 * must be able to fetch should not depend on the app rendering correctly.
 *
 * Note that `/indexnow.txt` and `/.well-known/indexnow.txt` are NOT part of
 * the protocol, despite being intuitive places to look. Hosting the key at
 * either would fail verification. Option 2 (an arbitrary location declared
 * per-request via `keyLocation`) exists, but the spec explicitly recommends
 * Option 1, and Option 2 additionally restricts which URLs a key may submit
 * to the directory the key file sits in.
 *
 * ── ON SECRECY ───────────────────────────────────────────────────────────
 * The spec says only you and the search engines should know the key. That is
 * aspirational for any static site: the file is by definition publicly
 * fetchable, and the filename is the key. Treat it as an identifier, not a
 * credential. It is checked in for the same reason robots.txt is — it is
 * part of the public surface. If it ever needs rotating, generate a new one,
 * add the new file, and only delete the old file once no submitter references
 * it.
 *
 * indexnow-key.spec.ts pins the constant to the file on disk, so the two
 * cannot drift apart — a mismatch means silent 403s on every submission.
 */
export const INDEXNOW_KEY = "e34fd3ce8f958568aee4d51b05b53271";

/** Public URL of the key file, per the protocol's Option 1. */
export const INDEXNOW_KEY_FILE = `/${INDEXNOW_KEY}.txt`;
