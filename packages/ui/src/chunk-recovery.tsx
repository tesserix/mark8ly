"use client";

import { useEffect } from "react";

/**
 * Next.js fingerprints every static chunk with the build id, and the pods
 * only ever hold the build they were built from. So a tab that loaded its
 * HTML before a deploy asks the new pods for `/_next/static/<old-hash>/…`
 * and gets a 404 — the page renders unstyled, navigation dies, and it reads
 * to the user as "slow and broken" rather than as a failed request.
 *
 * Measured on 2026-09-21: a current asset served 200 in 107ms while a stale
 * build id 404'd, during an hour with three deploys.
 *
 * Extra replicas do not help — after a rollout every pod is on the new
 * build. The only thing that repairs an already-loaded tab is reloading it,
 * which re-fetches HTML referencing chunks that exist.
 */

/** Session key. Scoped per build so a later deploy may reload again. */
const reloadKey = (buildTag: string) => `mark8ly:chunk-reload:${buildTag}`;

/**
 * isChunkLoadFailure reports whether a message is a missing-asset failure.
 *
 * Deliberately narrow: reloading the page is disruptive and must never fire
 * for an ordinary application error. Only the shapes browsers and webpack
 * actually produce when a chunk 404s are matched.
 */
export function isChunkLoadFailure(message: string | undefined | null): boolean {
  if (!message) return false;
  const m = message.toLowerCase();
  return (
    m.includes("chunkloaderror") ||
    m.includes("loading chunk") ||
    m.includes("loading css chunk") ||
    m.includes("failed to fetch dynamically imported module") ||
    m.includes("error loading dynamically imported module") ||
    m.includes("importing a module script failed")
  );
}

/** Reads sessionStorage without letting a blocked accessor throw. */
function alreadyReloaded(key: string): boolean {
  try {
    return window.sessionStorage.getItem(key) === "1";
  } catch {
    // Private mode / blocked storage: assume not reloaded. Worst case is
    // one extra reload, which still leaves the user on a working page.
    return false;
  }
}

function markReloaded(key: string): void {
  try {
    window.sessionStorage.setItem(key, "1");
  } catch {
    /* ignore — see alreadyReloaded */
  }
}

export interface ChunkRecoveryProps {
  /**
   * Identifies the current build so the once-only guard resets on the next
   * deploy. Pass the build id; a constant works but then a tab only ever
   * self-heals once.
   */
  buildTag?: string;
}

/**
 * ChunkRecovery reloads the page once when a stale chunk fails to load.
 *
 * Renders nothing. Mount it high in the tree (the root layout) so it is
 * listening before any lazy route does its first dynamic import.
 */
export function ChunkRecovery({ buildTag = "default" }: ChunkRecoveryProps): null {
  useEffect(() => {
    const key = reloadKey(buildTag);

    const recover = (message: string | undefined | null) => {
      if (!isChunkLoadFailure(message)) return;
      if (alreadyReloaded(key)) return; // never loop
      markReloaded(key);
      window.location.reload();
    };

    const onError = (e: ErrorEvent) => recover(e.message);
    const onRejection = (e: PromiseRejectionEvent) => {
      const r: unknown = e.reason;
      recover(r instanceof Error ? r.message : typeof r === "string" ? r : undefined);
    };

    window.addEventListener("error", onError);
    window.addEventListener("unhandledrejection", onRejection);
    return () => {
      window.removeEventListener("error", onError);
      window.removeEventListener("unhandledrejection", onRejection);
    };
  }, [buildTag]);

  return null;
}
