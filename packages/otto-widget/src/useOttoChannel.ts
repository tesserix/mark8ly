"use client";

import { useEffect, useRef, useState } from "react";
import type { WsEnvelope } from "./types";

export type ChannelState = "idle" | "connecting" | "open" | "closed" | "error";

export interface UseOttoChannelOptions {
  /** The full WebSocket URL (ws:// or wss://). Pass null/undefined to stay disconnected. */
  url: string | null | undefined;
  /** Called for every envelope the server sends. */
  onEvent?: (env: WsEnvelope) => void;
  /** Exponential backoff cap in milliseconds (default 10s). */
  maxBackoffMs?: number;
}

/**
 * useOttoChannel opens a WebSocket to the otto service and auto-reconnects
 * with exponential backoff. All message fan-in goes through `onEvent`.
 *
 * Intentionally doesn't ship a send() method: v1 of otto is server-push
 * only. Customers and staff send messages over REST, which lets the server
 * persist before fanning out and keeps the WS protocol one-directional.
 */
export function useOttoChannel({
  url,
  onEvent,
  maxBackoffMs = 10_000,
}: UseOttoChannelOptions) {
  const [state, setState] = useState<ChannelState>("idle");
  const socketRef = useRef<WebSocket | null>(null);
  const shouldRunRef = useRef(true);
  const onEventRef = useRef(onEvent);
  onEventRef.current = onEvent;

  useEffect(() => {
    if (!url) {
      setState("idle");
      return;
    }
    shouldRunRef.current = true;
    let attempt = 0;
    let retryTimer: ReturnType<typeof setTimeout> | null = null;

    const connect = () => {
      if (!shouldRunRef.current) return;
      setState("connecting");
      const ws = new WebSocket(url);
      socketRef.current = ws;

      ws.onopen = () => {
        attempt = 0;
        setState("open");
      };
      ws.onmessage = (ev) => {
        try {
          const env = JSON.parse(ev.data) as WsEnvelope;
          onEventRef.current?.(env);
        } catch {
          /* ignore malformed */
        }
      };
      ws.onerror = () => setState("error");
      ws.onclose = () => {
        setState("closed");
        if (!shouldRunRef.current) return;
        attempt += 1;
        const delay = Math.min(1000 * 2 ** Math.min(attempt, 4), maxBackoffMs);
        retryTimer = setTimeout(connect, delay);
      };
    };

    connect();

    return () => {
      shouldRunRef.current = false;
      if (retryTimer) clearTimeout(retryTimer);
      socketRef.current?.close();
      socketRef.current = null;
    };
  }, [url, maxBackoffMs]);

  return { state };
}
