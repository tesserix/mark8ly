"use client";

import { useEffect, useId } from "react";

import { useUnsavedNavigationGuard } from "@/components/shell/UnsavedChangesGuard";

/**
 * Guards unsaved work on both routes out of a page.
 *
 * `beforeunload` covers refresh, tab close and leaving the site. Its dialog
 * is the browser's own and cannot be styled — that is a browser guarantee,
 * not a choice we get to make here.
 *
 * App Router navigation fires no beforeunload at all, so the same dirty
 * state is also reported to UnsavedChangesProvider, which intercepts
 * same-origin link clicks and asks with the system's dialog. Without it,
 * clicking any sidebar link discarded the edits silently — the failure this
 * hook looked like it was already preventing.
 *
 * @param isDirty true when the form has unsaved edits
 * @param isSubmitting true while a save is in flight (suppresses the
 *                    warning so the browser doesn't nag during the POST)
 */
export function useUnsavedGuard(
  isDirty: boolean,
  isSubmitting: boolean = false,
): void {
  // A stable per-instance key, so two dirty forms on one page each hold
  // their own entry rather than overwriting one another.
  useUnsavedNavigationGuard(useId(), isDirty, isSubmitting);

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
