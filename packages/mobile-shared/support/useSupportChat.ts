// useSupportChat — the React Native hook that drives a live support thread
// with durability guarantees:
//   - resume-or-create, load history
//   - real-time delivery over the otto WebSocket (reconnect + app-state)
//   - a DURABLE SEND OUTBOX: outgoing messages are persisted to device
//     storage and re-sent with backoff until otto confirms them, so nothing
//     is lost across network drops, app backgrounding, or cold starts
//   - a POLLING FALLBACK: while the socket is down, inbound messages are
//     recovered by polling REST, so a missed live frame is never permanent
//
// Pure logic (events, outbox, request shaping) lives in sibling modules and
// is unit-tested; this file is the RN-bound glue. Shared by both apps.
import { useCallback, useEffect, useRef, useState } from "react";
import { AppState, type AppStateStatus } from "react-native";

import { SupportError, type SupportClient } from "./client";
import { mergeMessage, mergeMessages, parseOttoEvent } from "./events";
import { openSSE, type SSEHandle } from "./sse";
import {
  addItem,
  backoffMs,
  isRetryable,
  loadOutbox,
  markItem,
  outboxItemToMessage,
  removeItem,
  saveOutbox,
  type KVStorage,
  type OutboxItem,
} from "./outbox";
import type { CreateConversationInput, SupportConversation, SupportMessage } from "./types";

export type SupportChatStatus = "idle" | "loading" | "connecting" | "ready" | "closed" | "error";

export type DisplayMessage = SupportMessage & { pending?: boolean; failed?: boolean };

export interface UseSupportChatOptions {
  client: SupportClient;
  /** Resume the customer's open thread on mount. Default true. */
  autoResume?: boolean;
  /** Persistent storage for the send outbox (AsyncStorage / SecureStore).
   *  Omit for an in-memory outbox (no cross-restart durability). */
  storage?: KVStorage;
  /** Poll interval (ms) used as the fallback while the socket is down. */
  pollIntervalMs?: number;
}

export interface UseSupportChat {
  conversation: SupportConversation | null;
  messages: DisplayMessage[];
  status: SupportChatStatus;
  connected: boolean;
  /** True while any queued message is still unconfirmed. */
  hasPending: boolean;
  error: string | null;
  startConversation: (input: CreateConversationInput) => Promise<void>;
  sendMessage: (body: string) => Promise<void>;
  retryFailed: () => void;
  closeConversation: () => Promise<void>;
  refresh: () => Promise<void>;
}

const MAX_BACKOFF_MS = 15_000;
const DEFAULT_POLL_MS = 5_000;
// Consecutive WebSocket connect failures before falling back to SSE.
const WS_FALLBACK_THRESHOLD = 3;

function newClientMsgId(): string {
  return `cmid-${Date.now().toString(36)}-${Math.random().toString(36).slice(2, 10)}`;
}

function nowIso(): string {
  return new Date().toISOString();
}

function displayMessages(server: SupportMessage[], outbox: OutboxItem[]): DisplayMessage[] {
  const pending = outbox.map(outboxItemToMessage) as DisplayMessage[];
  return mergeMessages(server, pending as SupportMessage[]) as DisplayMessage[];
}

export function useSupportChat({
  client,
  autoResume = true,
  storage,
  pollIntervalMs = DEFAULT_POLL_MS,
}: UseSupportChatOptions): UseSupportChat {
  const [conversation, setConversation] = useState<SupportConversation | null>(null);
  const [serverMessages, setServerMessages] = useState<SupportMessage[]>([]);
  const [outbox, setOutbox] = useState<OutboxItem[]>([]);
  const [status, setStatus] = useState<SupportChatStatus>("idle");
  const [connected, setConnected] = useState(false);
  const [error, setError] = useState<string | null>(null);

  const socketRef = useRef<WebSocket | null>(null);
  const sseRef = useRef<SSEHandle | null>(null);
  const preferSSERef = useRef(false);
  const wsFailuresRef = useRef(0);
  const reconnectRef = useRef<ReturnType<typeof setTimeout> | null>(null);
  const retryRef = useRef<ReturnType<typeof setTimeout> | null>(null);
  const attemptsRef = useRef(0);
  const closedByUsRef = useRef(false);
  const convIdRef = useRef<string | null>(null);
  const connectedRef = useRef(false);
  const outboxRef = useRef<OutboxItem[]>([]);
  const drainingRef = useRef(false);

  const setConnectedBoth = (v: boolean) => {
    connectedRef.current = v;
    setConnected(v);
  };

  // ── Outbox helpers ────────────────────────────────────────────────
  const persistOutbox = useCallback(
    (next: OutboxItem[]) => {
      outboxRef.current = next;
      setOutbox(next);
      if (storage && convIdRef.current) void saveOutbox(storage, convIdRef.current, next);
    },
    [storage],
  );

  const scheduleDrain = (delay: number) => {
    if (retryRef.current) clearTimeout(retryRef.current);
    retryRef.current = setTimeout(() => void drainOutbox(), delay);
  };

  const drainOutbox = useCallback(async () => {
    if (drainingRef.current) return;
    drainingRef.current = true;
    try {
      // eslint-disable-next-line no-constant-condition
      while (true) {
        const next = outboxRef.current.find((i) => i.status === "queued");
        if (!next) break;
        persistOutbox(markItem(outboxRef.current, next.clientMsgId, { status: "sending" }));
        try {
          const serverMsg = await client.postMessage(next.conversationId, next.body);
          setServerMessages((prev) => mergeMessage(prev, serverMsg));
          persistOutbox(removeItem(outboxRef.current, next.clientMsgId));
        } catch (e) {
          const httpStatus = e instanceof SupportError ? e.status : null;
          const attempts = next.attempts + 1;
          if (isRetryable(httpStatus)) {
            // Transient — requeue and retry with backoff. Never dropped.
            persistOutbox(markItem(outboxRef.current, next.clientMsgId, { status: "queued", attempts }));
            scheduleDrain(backoffMs(attempts));
            break;
          }
          // Terminal (4xx) — surface for manual retry; keep the body so it's
          // still recoverable, never silently lost.
          persistOutbox(markItem(outboxRef.current, next.clientMsgId, { status: "failed", attempts }));
        }
      }
    } finally {
      drainingRef.current = false;
    }
  }, [client, persistOutbox]);

  const sendMessage = useCallback(
    async (body: string) => {
      const id = convIdRef.current;
      const trimmed = body.trim();
      if (!id || !trimmed) return;
      const item: OutboxItem = {
        clientMsgId: newClientMsgId(),
        conversationId: id,
        body: trimmed,
        createdAt: nowIso(),
        attempts: 0,
        status: "queued",
      };
      persistOutbox(addItem(outboxRef.current, item));
      void drainOutbox();
    },
    [persistOutbox, drainOutbox],
  );

  const retryFailed = useCallback(() => {
    const requeued = outboxRef.current.map((i) =>
      i.status === "failed" ? { ...i, status: "queued" as const, attempts: 0 } : i,
    );
    persistOutbox(requeued);
    void drainOutbox();
  }, [persistOutbox, drainOutbox]);

  // ── Realtime (WebSocket → SSE → polling) ──────────────────────────
  const clearReconnect = () => {
    if (reconnectRef.current) {
      clearTimeout(reconnectRef.current);
      reconnectRef.current = null;
    }
  };

  // teardownSocket tears down whichever realtime transport is active
  // (WebSocket or SSE) and stops reconnects.
  const teardownSocket = useCallback(() => {
    closedByUsRef.current = true;
    clearReconnect();
    if (socketRef.current) {
      socketRef.current.close();
      socketRef.current = null;
    }
    if (sseRef.current) {
      sseRef.current.close();
      sseRef.current = null;
    }
    setConnectedBoth(false);
  }, []);

  const connect = useCallback(
    async (conversationId: string) => {
      closedByUsRef.current = false;

      const reconnect = () => {
        const attempt = (attemptsRef.current += 1);
        const delay = Math.min(MAX_BACKOFF_MS, 1000 * 2 ** (attempt - 1));
        clearReconnect();
        reconnectRef.current = setTimeout(() => {
          if (convIdRef.current) void connect(convIdRef.current);
        }, delay);
      };
      const onOpen = () => {
        attemptsRef.current = 0;
        setConnectedBoth(true);
        setStatus("ready");
        // Reconcile anything missed while offline + flush the outbox.
        void refresh();
        void drainOutbox();
      };
      const onFrame = (raw: string) => {
        const event = parseOttoEvent(raw);
        if (event.kind === "message") {
          setServerMessages((prev) => mergeMessage(prev, event.message));
        } else if (event.kind === "conversation_closed") {
          setStatus("closed");
          setConversation((prev) => (prev ? { ...prev, status: "closed" } : prev));
          teardownSocket();
        }
      };

      let ticket;
      try {
        ticket = await client.getWsTicket(conversationId);
      } catch {
        reconnect();
        return;
      }
      if (closedByUsRef.current) return;

      // After repeated WS failures (blocked upgrades / hostile proxies) fall
      // back to SSE. Polling remains the final safety net in both modes.
      if (preferSSERef.current) {
        const handle = openSSE(client.buildSseUrl(ticket), {
          onOpen,
          onMessage: onFrame,
          onError: () => {
            setConnectedBoth(false);
            sseRef.current = null;
            if (closedByUsRef.current) return;
            reconnect();
          },
        });
        sseRef.current = handle;
        return;
      }

      const ws = new WebSocket(client.buildWsUrl(ticket));
      socketRef.current = ws;
      ws.onopen = () => {
        wsFailuresRef.current = 0;
        onOpen();
      };
      ws.onmessage = (ev) => {
        const data = (ev as { data?: unknown }).data;
        onFrame(typeof data === "string" ? data : "");
      };
      ws.onerror = () => setConnectedBoth(false);
      ws.onclose = () => {
        setConnectedBoth(false);
        socketRef.current = null;
        if (closedByUsRef.current) return;
        wsFailuresRef.current += 1;
        if (wsFailuresRef.current >= WS_FALLBACK_THRESHOLD) {
          preferSSERef.current = true; // switch transports on next attempt
        }
        reconnect();
      };
    },
    // eslint-disable-next-line react-hooks/exhaustive-deps
    [client, teardownSocket, drainOutbox],
  );

  const refresh = useCallback(async () => {
    if (!convIdRef.current) return;
    try {
      const msgs = await client.listMessages(convIdRef.current);
      setServerMessages((prev) => mergeMessages(prev, msgs));
    } catch (e) {
      setError(e instanceof Error ? e.message : "refresh failed");
    }
  }, [client]);

  const adoptConversation = useCallback(
    async (conv: SupportConversation, initial: SupportMessage[]) => {
      setConversation(conv);
      setServerMessages((prev) => mergeMessages(prev, initial));
      convIdRef.current = conv.id;
      // Resume any unsent messages persisted from a previous session.
      if (storage) {
        const persisted = await loadOutbox(storage, conv.id);
        if (persisted.length) {
          // Anything left "sending" from a killed session is requeued.
          const requeued = persisted.map((i) => (i.status === "sending" ? { ...i, status: "queued" as const } : i));
          outboxRef.current = requeued;
          setOutbox(requeued);
        }
      }
      if (conv.status === "closed") {
        setStatus("closed");
        teardownSocket();
        return;
      }
      setStatus("connecting");
      await connect(conv.id);
      void drainOutbox();
    },
    [connect, teardownSocket, storage, drainOutbox],
  );

  const startConversation = useCallback(
    async (input: CreateConversationInput) => {
      setStatus("loading");
      setError(null);
      try {
        const { conversation: conv, firstMessage } = await client.createConversation(input);
        await adoptConversation(conv, [firstMessage]);
      } catch (e) {
        setStatus("error");
        setError(e instanceof Error ? e.message : "could not start chat");
        throw e;
      }
    },
    [client, adoptConversation],
  );

  const closeConversation = useCallback(async () => {
    const id = convIdRef.current;
    if (!id) return;
    const conv = await client.close(id);
    setConversation(conv);
    setStatus("closed");
    teardownSocket();
  }, [client, teardownSocket]);

  // Resume on mount.
  useEffect(() => {
    let cancelled = false;
    if (!autoResume) {
      setStatus("idle");
      return;
    }
    setStatus("loading");
    client
      .resume()
      .then((res) => {
        if (cancelled) return;
        if (res) void adoptConversation(res.conversation, res.messages);
        else setStatus("idle");
      })
      .catch((e) => {
        if (cancelled) return;
        setStatus("idle");
        setError(e instanceof Error ? e.message : null);
      });
    return () => {
      cancelled = true;
      teardownSocket();
      if (retryRef.current) clearTimeout(retryRef.current);
    };
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, []);

  // Foreground reconnect.
  useEffect(() => {
    const onChange = (next: AppStateStatus) => {
      if (next === "active" && convIdRef.current && !socketRef.current) {
        if (!conversation || conversation.status !== "closed") void connect(convIdRef.current);
      }
    };
    const sub = AppState.addEventListener("change", onChange);
    return () => sub.remove();
  }, [connect, conversation]);

  // Polling fallback — while the socket is down, recover inbound messages
  // over REST so a dropped live frame is never permanent.
  useEffect(() => {
    const open = !!conversation && conversation.status !== "closed";
    if (!open) return;
    const timer = setInterval(() => {
      if (!connectedRef.current && convIdRef.current) void refresh();
    }, pollIntervalMs);
    return () => clearInterval(timer);
  }, [conversation, pollIntervalMs, refresh]);

  return {
    conversation,
    messages: displayMessages(serverMessages, outbox),
    status,
    connected,
    hasPending: outbox.length > 0,
    error,
    startConversation,
    sendMessage,
    retryFailed,
    closeConversation,
    refresh,
  };
}
