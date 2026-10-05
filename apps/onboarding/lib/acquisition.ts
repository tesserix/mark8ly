// Acquisition attribution (#992).
//
// What brought a merchant here is captured in the browser on the first page
// they see and carried to the server with the onboarding session, so it
// survives the verification email being opened on another device and ends
// up against the tenant the session produces. Nothing in this file is
// trusted by the server — platform-api sanitises it again on arrival — but
// the allowlist starts here so a stray token in a query string never leaves
// the browser in the first place.
//
// Rules, stated once:
//   - A "touch" is one arrival: the five UTM values, the landing path with
//     its query removed, the referrer with its query removed, and the time.
//   - `first` is the earliest touch the browser remembers. `last` is the
//     most recent TAGGED touch (one with any UTM value). An untagged arrival
//     never overwrites a tagged one, so a merchant who came from a campaign
//     and returns by typing the address keeps their campaign.
//   - Memory lasts RETENTION_DAYS from `first`. After that the next arrival
//     starts a fresh record. Server-side retention is the session's own.
//   - An untagged first arrival is still recorded, with no UTM fields, so
//     "direct" and "referral" are explicit outcomes downstream rather than
//     an absence someone has to interpret.

export interface AcquisitionTouch {
  utm_source?: string;
  utm_medium?: string;
  utm_campaign?: string;
  utm_content?: string;
  utm_term?: string;
  landing_path?: string;
  referrer?: string;
  captured_at?: string;
}

export interface Acquisition {
  first?: AcquisitionTouch;
  last?: AcquisitionTouch;
}

export const ACQUISITION_STORAGE_KEY = "mark8ly:acquisition:v1";
export const RETENTION_DAYS = 90;

const UTM_KEYS = [
  "utm_source",
  "utm_medium",
  "utm_campaign",
  "utm_content",
  "utm_term",
] as const;

const MAX_FIELD_LENGTH = 200;

function clean(value: string | null | undefined): string | undefined {
  if (!value) return undefined;
  // eslint-disable-next-line no-control-regex
  const trimmed = value.replace(/[\u0000-\u001f\u007f]/g, "").trim();
  if (!trimmed) return undefined;
  return trimmed.slice(0, MAX_FIELD_LENGTH);
}

/** True when the touch carries any UTM value. */
export function isTagged(touch: AcquisitionTouch | undefined): boolean {
  if (!touch) return false;
  return UTM_KEYS.some((key) => Boolean(touch[key]));
}

/**
 * Build a touch from a page URL and the document referrer. Pure: the caller
 * supplies the clock. The referrer is dropped when it is the same host as
 * the page, since a merchant moving between our own pages is not a referral.
 */
export function touchFromLocation(
  href: string,
  referrer: string,
  now: Date,
): AcquisitionTouch {
  const touch: AcquisitionTouch = {};
  let url: URL;
  try {
    url = new URL(href);
  } catch {
    return { captured_at: now.toISOString() };
  }
  for (const key of UTM_KEYS) {
    const value = clean(url.searchParams.get(key));
    if (value) touch[key] = value;
  }
  touch.landing_path = url.pathname || "/";
  const ref = cleanReferrer(referrer, url.host);
  if (ref) touch.referrer = ref;
  touch.captured_at = now.toISOString();
  return touch;
}

function cleanReferrer(referrer: string, ownHost: string): string | undefined {
  const value = clean(referrer);
  if (!value) return undefined;
  try {
    const url = new URL(value);
    if (url.protocol !== "http:" && url.protocol !== "https:") return undefined;
    if (url.host === ownHost) return undefined;
    return `${url.protocol}//${url.host}${url.pathname}`.slice(0, MAX_FIELD_LENGTH);
  } catch {
    return undefined;
  }
}

/** True when the record's first touch is older than the retention window. */
export function isExpired(record: Acquisition | undefined, now: Date): boolean {
  const at = record?.first?.captured_at;
  if (!at) return true;
  const started = Date.parse(at);
  if (Number.isNaN(started)) return true;
  return now.getTime() - started > RETENTION_DAYS * 24 * 60 * 60 * 1000;
}

/**
 * Fold a new arrival into what the browser already remembers. Pure.
 * Returns the record to store; identical input yields the same object so a
 * caller can skip the write when nothing changed.
 */
export function mergeAcquisition(
  stored: Acquisition | undefined,
  touch: AcquisitionTouch,
  now: Date,
): Acquisition {
  if (!stored || isExpired(stored, now)) {
    return { first: touch, last: touch };
  }
  if (isTagged(touch)) {
    return { first: stored.first ?? touch, last: touch };
  }
  return stored;
}

/** Parse a stored record defensively; anything malformed reads as absent. */
export function parseStoredAcquisition(raw: string | null): Acquisition | undefined {
  if (!raw) return undefined;
  try {
    const parsed = JSON.parse(raw) as unknown;
    if (!parsed || typeof parsed !== "object") return undefined;
    const record = parsed as Acquisition;
    const first = pickTouch(record.first);
    const last = pickTouch(record.last);
    if (!first && !last) return undefined;
    return { first, last };
  } catch {
    return undefined;
  }
}

function pickTouch(value: unknown): AcquisitionTouch | undefined {
  if (!value || typeof value !== "object") return undefined;
  const source = value as Record<string, unknown>;
  const touch: AcquisitionTouch = {};
  for (const key of [...UTM_KEYS, "landing_path", "referrer", "captured_at"] as const) {
    const raw = source[key];
    if (typeof raw === "string") {
      const cleaned = clean(raw);
      if (cleaned) touch[key] = cleaned;
    }
  }
  return Object.keys(touch).length > 0 ? touch : undefined;
}

// ─── Browser-only helpers ───────────────────────────────────────────────
//
// Storage access is wrapped so a private window, a full quota or a
// disabled storage API degrades to "no attribution" rather than an error:
// analytics must never block a signup.

/** Record the current page as an arrival. Call once per page load. */
export function captureCurrentArrival(now = new Date()): void {
  if (typeof window === "undefined") return;
  try {
    const touch = touchFromLocation(window.location.href, document.referrer, now);
    const stored = parseStoredAcquisition(
      window.localStorage.getItem(ACQUISITION_STORAGE_KEY),
    );
    const next = mergeAcquisition(stored, touch, now);
    if (next !== stored) {
      window.localStorage.setItem(ACQUISITION_STORAGE_KEY, JSON.stringify(next));
    }
  } catch {
    // Storage unavailable. The signup proceeds without attribution.
  }
}

/** What the browser remembers, for sending with the session create. */
export function readStoredAcquisition(now = new Date()): Acquisition | undefined {
  if (typeof window === "undefined") return undefined;
  try {
    const stored = parseStoredAcquisition(
      window.localStorage.getItem(ACQUISITION_STORAGE_KEY),
    );
    if (!stored || isExpired(stored, now)) return undefined;
    return stored;
  } catch {
    return undefined;
  }
}
