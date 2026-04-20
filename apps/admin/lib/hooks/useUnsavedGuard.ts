"use client";

import { useEffect } from "react";

/**
 * Attach a `beforeunload` warning when the form has dirty changes so the
 * user doesn't lose work by refreshing / closing the tab / following an
 * external link. App Router client-side navigation doesn't trigger
 * beforeunload — for that, intercept the back / discard click and show a
 * confirm dialog.
 *
 * @param isDirty true when the form has unsaved edits
 * @param isSubmitting true while a save is in flight (suppresses the
 *                    warning so the browser doesn't nag during the POST)
 */
export function useUnsavedGuard(
  isDirty: boolean,
  isSubmitting: boolean = false,
): void {
  useEffect(() => {
    const handler = (e: BeforeUnloadEvent) => {
      if (!isDirty || isSubmitting) return;
      e.preventDefault();
      e.returnValue = "";
    };
    window.addEventListener("beforeunload", handler);
    return () => window.removeEventListener("beforeunload", handler);
  }, [isDirty, isSubmitting]);
}
