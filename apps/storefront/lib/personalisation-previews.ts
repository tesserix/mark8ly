// apps/storefront/lib/personalisation-previews.ts
//
// Re-signing the buyer's own artwork for display (#966).
//
// # Why these cannot be cached
//
// A preview URL is a short-lived signed GET against a private bucket. It
// expires in minutes, so storing it in the cart alongside the upload id
// would persist a dead link: the cart survives in localStorage
// indefinitely, the URL does not. The cart therefore stores `uploadId`
// only, and these are re-signed on every render.
//
// # Why one request for the whole cart
//
// A cart can have several personalised lines and each needs a thumbnail.
// One round trip per line on every render is wasteful and slow, so the
// server takes a list. See POST /personalisation/previews.
//
// # Unavailable is a first-class answer, not an error
//
// Uploads are swept at 72 hours; cart lines are not. So a shopper can
// return to a cart whose photo is gone. The server reports those ids
// under `unavailable` rather than failing the batch, because the cart
// needs to know WHICH line to mark — "your photo has expired, upload it
// again" belongs on that line, not on the pay button.

export interface PreviewEntry {
  url: string;
  expiresAt: string;
}

export interface PreviewLookup {
  /** Signed URL per upload id that could be read. */
  previews: Record<string, PreviewEntry>;
  /** Upload ids that are gone, swept, or not this cart's. */
  unavailable: ReadonlySet<string>;
}

export const emptyPreviewLookup: PreviewLookup = {
  previews: {},
  unavailable: new Set<string>(),
};

/** True when this id was asked for and came back unreadable. */
export function isExpired(lookup: PreviewLookup, uploadId: string): boolean {
  return lookup.unavailable.has(uploadId);
}

export interface FetchPreviewsDeps {
  fetchJson?: typeof fetch;
}

/**
 * Signs every given upload id, in one request.
 *
 * Returns `emptyPreviewLookup` for an empty input without touching the
 * network. A transport failure also yields empty rather than throwing:
 * a cart that cannot render thumbnails is still a usable cart, and the
 * authoritative check is the server's at checkout. Crucially this does
 * NOT report every id as unavailable on a network error — that would
 * tell the buyer their photos had expired when the truth is that we
 * could not ask.
 */
export async function fetchPersonalisationPreviews(
  storeSlug: string,
  uploadIds: readonly string[],
  deps: FetchPreviewsDeps = {},
): Promise<PreviewLookup> {
  const ids = Array.from(new Set(uploadIds.filter(Boolean)));
  if (ids.length === 0) return emptyPreviewLookup;

  const doFetch = deps.fetchJson ?? fetch;
  try {
    const res = await doFetch(
      `/api/personalisation/previews?store=${encodeURIComponent(storeSlug)}`,
      {
        method: "POST",
        headers: { "Content-Type": "application/json" },
        body: JSON.stringify({ upload_ids: ids }),
        cache: "no-store",
      },
    );
    if (!res.ok) return emptyPreviewLookup;

    const body = (await res.json()) as {
      previews?: Record<string, { url?: string; expires_at?: string }>;
      unavailable?: string[];
    };

    const previews: Record<string, PreviewEntry> = {};
    for (const [id, entry] of Object.entries(body.previews ?? {})) {
      if (entry?.url) {
        previews[id] = { url: entry.url, expiresAt: entry.expires_at ?? "" };
      }
    }
    return {
      previews,
      unavailable: new Set(body.unavailable ?? []),
    };
  } catch {
    return emptyPreviewLookup;
  }
}

/** Every upload id referenced by a set of cart lines. */
export function collectUploadIds(
  lines: readonly { personalisation?: readonly { uploadId?: string }[] }[],
): string[] {
  const out = new Set<string>();
  for (const line of lines) {
    for (const entry of line.personalisation ?? []) {
      if (entry.uploadId) out.add(entry.uploadId);
    }
  }
  return Array.from(out);
}
