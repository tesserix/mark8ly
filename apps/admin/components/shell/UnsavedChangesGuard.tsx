"use client";

// UnsavedChangesGuard — the half of the unsaved-changes story that
// `beforeunload` cannot cover.
//
// There were two guards and between them they protected the wrong things.
// `beforeunload` fires for refresh, tab close and leaving the site, and its
// dialog is drawn by the browser — unstyleable by design, and the one people
// complain about. In-app navigation fires it not at all: App Router
// transitions the client, so clicking any sidebar link with a dirty form
// unmounted it and the edits were gone. Loud where it could not be styled,
// silent where it actually cost work.
//
// This closes the silent half. A capture-phase click listener intercepts
// same-origin link navigations while any form reports itself dirty, and asks
// with the system's own dialog.
//
// Capture phase, deliberately: it has to run before Next's Link handler
// starts the transition. An anchor that wants to own its own confirmation
// opts out with data-unsaved-guard="off".

import * as React from "react";
import { useRouter } from "next/navigation";

import { AlertDialog } from "@tesserix/web";

interface UnsavedChangesContextValue {
  /** Report a form's dirtiness. Keyed so several forms can coexist. */
  setDirty: (key: string, dirty: boolean) => void;
}

const UnsavedChangesContext =
  React.createContext<UnsavedChangesContextValue | null>(null);

/**
 * Registers this form's dirty state with the navigation guard.
 *
 * Safe to call outside the provider — it becomes a no-op, so a form rendered
 * on its own (a test, a standalone page) behaves exactly as before.
 */
export function useUnsavedNavigationGuard(
  key: string,
  isDirty: boolean,
  isSubmitting: boolean,
): void {
  const ctx = React.useContext(UnsavedChangesContext);
  const setDirty = ctx?.setDirty;

  React.useEffect(() => {
    if (!setDirty) return;
    setDirty(key, isDirty && !isSubmitting);
    // Clearing on unmount matters: a form that navigated away is no longer
    // holding anything, and a stale "dirty" would block every later click.
    return () => setDirty(key, false);
  }, [setDirty, key, isDirty, isSubmitting]);
}

/** True when this click should be left alone rather than intercepted. */
function isPlainLinkNavigation(e: MouseEvent): HTMLAnchorElement | null {
  // Anything but an unmodified primary click is the user asking for a new
  // tab or window, which does not unmount the form.
  if (e.defaultPrevented) return null;
  if (e.button !== 0) return null;
  if (e.metaKey || e.ctrlKey || e.shiftKey || e.altKey) return null;

  const anchor = (e.target as HTMLElement | null)?.closest?.("a");
  if (!anchor) return null;
  if (anchor.dataset.unsavedGuard === "off") return null;
  if (anchor.hasAttribute("download")) return null;
  if (anchor.target && anchor.target !== "_self") return null;

  const href = anchor.getAttribute("href");
  if (!href || href.startsWith("#")) return null;

  let url: URL;
  try {
    url = new URL(anchor.href, window.location.href);
  } catch {
    return null;
  }
  // A different origin unloads the document, so beforeunload already asks.
  if (url.origin !== window.location.origin) return null;
  // Same page: nothing unmounts.
  if (url.pathname === window.location.pathname && url.search === window.location.search) {
    return null;
  }
  return anchor;
}

export function UnsavedChangesProvider({
  children,
}: {
  children: React.ReactNode;
}) {
  const router = useRouter();
  const dirtyRef = React.useRef<Map<string, boolean>>(new Map());
  const [pendingHref, setPendingHref] = React.useState<string | null>(null);

  const setDirty = React.useCallback((key: string, dirty: boolean) => {
    if (dirty) dirtyRef.current.set(key, true);
    else dirtyRef.current.delete(key);
  }, []);

  React.useEffect(() => {
    const onClick = (e: MouseEvent) => {
      if (dirtyRef.current.size === 0) return;
      const anchor = isPlainLinkNavigation(e);
      if (!anchor) return;

      e.preventDefault();
      e.stopPropagation();
      const url = new URL(anchor.href, window.location.href);
      setPendingHref(url.pathname + url.search);
    };

    document.addEventListener("click", onClick, true);
    return () => document.removeEventListener("click", onClick, true);
  }, []);

  const value = React.useMemo(() => ({ setDirty }), [setDirty]);

  return (
    <UnsavedChangesContext.Provider value={value}>
      {children}
      <AlertDialog
        isOpen={pendingHref !== null}
        onClose={() => setPendingHref(null)}
        title="Leave without saving?"
        message="Your unsaved changes will be lost."
        type="confirm"
        confirmLabel="Leave"
        cancelLabel="Stay on this page"
        onConfirm={() => {
          const href = pendingHref;
          // The forms this guard protects unmount on navigation and clear
          // their own entry, but clearing here too means a confirmed leave
          // can never be blocked a second time by a form that is slow to
          // tear down.
          dirtyRef.current.clear();
          setPendingHref(null);
          if (href) router.push(href);
        }}
        onCancel={() => setPendingHref(null)}
      />
    </UnsavedChangesContext.Provider>
  );
}
