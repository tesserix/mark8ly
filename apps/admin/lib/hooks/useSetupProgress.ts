"use client";

import { useEffect, useRef, useState } from "react";

import type { SetupProgress } from "@/lib/api/marketplace-api";

const MAX_SSE_RETRIES = 5;
const POLL_INTERVAL_MS = 30_000;

/**
 * Live store-setup progress. Subscribes to the setup-progress SSE stream
 * (proxied through /api/admin so the BFF session applies) and falls back
 * to 30s polling if the stream can't be established. Returns the latest
 * SetupProgress, seeded with the server-rendered snapshot so there is no
 * flash of empty state.
 *
 * The subscription tears itself down once every step is complete — the
 * server also ends the stream at 100%, so a finished store costs nothing.
 */
export function useSetupProgress(
  storeId: string,
  initial: SetupProgress,
): SetupProgress {
  const [progress, setProgress] = useState<SetupProgress>(initial);
  const doneRef = useRef(initial.completed_steps === initial.total_steps);

  useEffect(() => {
    if (doneRef.current) return;

    let stopped = false;
    let source: EventSource | null = null;
    let retryTimer: ReturnType<typeof setTimeout> | null = null;
    let pollTimer: ReturnType<typeof setInterval> | null = null;
    let retries = 0;

    const apply = (next: SetupProgress) => {
      setProgress(next);
      if (next.completed_steps === next.total_steps) {
        doneRef.current = true;
        teardown();
      }
    };

    const teardown = () => {
      stopped = true;
      source?.close();
      source = null;
      if (retryTimer) clearTimeout(retryTimer);
      if (pollTimer) clearInterval(pollTimer);
    };

    const startPolling = () => {
      if (stopped || pollTimer) return;
      pollTimer = setInterval(async () => {
        try {
          const res = await fetch(
            `/api/admin/stores/${storeId}/setup-progress`,
            { cache: "no-store" },
          );
          if (!res.ok) return;
          apply((await res.json()) as SetupProgress);
        } catch {
          // transient network failure — next tick retries
        }
      }, POLL_INTERVAL_MS);
    };

    const connect = () => {
      if (stopped) return;
      source = new EventSource(
        `/api/admin/stores/${storeId}/setup-progress/stream`,
      );
      source.onopen = () => {
        // A successful (re)connect clears the backoff budget, so routine
        // server-side stream rotation (30-min max age) never degrades a
        // healthy client to polling.
        retries = 0;
      };
      source.addEventListener("progress", (event) => {
        try {
          apply(JSON.parse((event as MessageEvent).data) as SetupProgress);
        } catch {
          // malformed frame — ignore, the next one supersedes it
        }
      });
      source.onerror = () => {
        source?.close();
        source = null;
        if (stopped) return;
        retries += 1;
        if (retries <= MAX_SSE_RETRIES) {
          // Exponential backoff capped at 15s: 1s, 2s, 4s, 8s, 15s.
          const delay = Math.min(1000 * 2 ** (retries - 1), 15_000);
          retryTimer = setTimeout(connect, delay);
        } else {
          startPolling();
        }
      };
    };

    connect();
    return teardown;
  }, [storeId]);

  return progress;
}
