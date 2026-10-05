"use client";

// Re-signs the artwork on a set of cart lines, once per change (#966).
//
// The URLs are short-lived signed GETs against a private bucket, so they
// cannot be stored in the cart — the cart keeps `uploadId` and this hook
// turns those into something renderable.
//
// Keyed on the SET of upload ids rather than on the lines themselves, so
// changing a quantity does not re-sign anything: the ids are what the
// request depends on. Re-signing on every quantity tap would be a
// request per keystroke on the stepper.

import { useEffect, useMemo, useState } from "react";

import {
  collectUploadIds,
  emptyPreviewLookup,
  fetchPersonalisationPreviews,
  type PreviewLookup,
} from "@/lib/personalisation-previews";

export function usePersonalisationPreviews(
  storeSlug: string,
  lines: readonly { personalisation?: readonly { uploadId?: string }[] }[],
): PreviewLookup {
  const ids = useMemo(() => collectUploadIds(lines).sort(), [lines]);
  // A stable primitive so the effect does not refire on a new array with
  // identical contents.
  const idKey = ids.join(",");

  const [lookup, setLookup] = useState<PreviewLookup>(emptyPreviewLookup);

  useEffect(() => {
    if (ids.length === 0) {
      setLookup(emptyPreviewLookup);
      return;
    }
    let live = true;
    void fetchPersonalisationPreviews(storeSlug, ids).then((next) => {
      // Guarded because a buyer can empty the cart while this is in
      // flight, and setting state then would resurrect thumbnails for
      // lines that are gone.
      if (live) setLookup(next);
    });
    return () => {
      live = false;
    };
    // ids is covered by idKey; listing it too would defeat the point.
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [storeSlug, idKey]);

  return lookup;
}
