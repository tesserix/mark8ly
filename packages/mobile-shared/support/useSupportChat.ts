// useSupportChat — the React Native hook that drives a live support thread:
// resume-or-create, load history, open the otto WebSocket with reconnect +
// app-state handling, and append incoming messages. Shared verbatim by the
// storefront (customer->merchant) and admin (merchant->platform) apps; only
// the SupportClient passed in differs.
//
// Pure logic (event parsing, message merge, request shaping) lives in the
// sibling modules so it can be unit-tested without an RN runtime; this file
// is the thin RN-bound glue.
import { useCallback, useEffect, useRef, useState } from "react";
import { AppState, type AppStateStatus } from "react-native";

import type { SupportClient } from "./client";
import { mergeMessage, mergeMessages, parseOttoEvent } from "./events";
import type { CreateConversationInput, SupportConversation, SupportMessage } from "./types";

export type SupportChatStatus =
  | "idle"
  | "loading"
  | "connecting"
  | "ready"
  | "closed"
  | "error";

export interface UseSupportChatOptions {
  client: SupportClient;
  /** Resume the customer's open thread on mount. Default true. */
  autoResume?: boolean;
}

export interface UseSupportChat {
  conversation: SupportConversation | null;
  messages: SupportMessage[];
  status: SupportChatStatus;
  connected: boolean;
  error: string | null;
  startConversation: (input: CreateConversationInput) => Promise<void>;
  sendMessage: (body: string) => Promise<void>;
  closeConversation: () => Promise<void>;
  refresh: () => Promise<void>;
}

const MAX_BACKOFF_MS = 15_000;

export function useSupportChat({ client, autoResume = true }: UseSupportChatOptions): UseSupportChat {
  const [conversation, setConversation] = useState<SupportConversation | null>(null);
  const [messages, setMessages] = useState<SupportMessage[]>([]);
  const [status, setStatus] = useState<SupportChatStatus>("idle");
  const [connected, setConnected] = useState(false);
  const [error, setError] = useState<string | null>(null);

  const socketRef = useRef<WebSocket | null>(null);
  const reconnectRef = useRef<ReturnType<typeof setTimeout> | null>(null);
  const attemptsRef = useRef(0);
  const closedByUsRef = useRef(false);
  const convIdRef = useRef<string | null>(null);

  const clearReconnect = () => {
    if (reconnectRef.current) {
      clearTimeout(reconnectRef.current);
      reconnectRef.current = null;
    }
  };

  const teardownSocket = useCallback(() => {
    closedByUsRef.current = true;
    clearReconnect();
    if (socketRef.current) {
      socketRef.current.close();
      socketRef.current = null;
    }
    setConnected(false);
  }, []);

  const connect = useCallback(
    async (conversationId: string) => {
      // Closed threads don't get a socket — they're read-only.
      closedByUsRef.current = false;
      try {
        const ticket = await client.getWsTicket(conversationId);
        if (closedByUsRef.current) return;
        const ws = new WebSocket(client.buildWsUrl(ticket));
        socketRef.current = ws;

        ws.onopen = () => {
          attemptsRef.current = 0;
          setConnected(true);
          setStatus("ready");
        };
        ws.onmessage = (ev: { data: string }) => {
          const event = parseOttoEvent(typeof ev.data === "string" ? ev.data : "");
          if (event.kind === "message") {
            setMessages((prev) => mergeMessage(prev, event.message));
          } else if (event.kind === "conversation_closed") {
            setStatus("closed");
            setConversation((prev) => (prev ? { ...prev, status: "closed" } : prev));
            teardownSocket();
          }
        };
        ws.onerror = () => {
          setConnected(false);
        };
        ws.onclose = () => {
          setConnected(false);
          socketRef.current = null;
          if (closedByUsRef.current) return;
          // Reconnect with capped exponential backoff.
          const attempt = (attemptsRef.current += 1);
          const delay = Math.min(MAX_BACKOFF_MS, 1000 * 2 ** (attempt - 1));
          clearReconnect();
          reconnectRef.current = setTimeout(() => {
            if (convIdRef.current) void connect(convIdRef.current);
          }, delay);
        };
      } catch {
        // Ticket mint failed (e.g. offline) — retry on a backoff.
        const attempt = (attemptsRef.current += 1);
        const delay = Math.min(MAX_BACKOFF_MS, 1000 * 2 ** (attempt - 1));
        clearReconnect();
        reconnectRef.current = setTimeout(() => {
          if (convIdRef.current) void connect(convIdRef.current);
        }, delay);
      }
    },
    [client, teardownSocket],
  );

  const adoptConversation = useCallback(
    async (conv: SupportConversation, initial: SupportMessage[]) => {
      setConversation(conv);
      setMessages((prev) => mergeMessages(prev, initial));
      convIdRef.current = conv.id;
      if (conv.status === "closed") {
        setStatus("closed");
        teardownSocket();
        return;
      }
      setStatus("connecting");
      await connect(conv.id);
    },
    [connect, teardownSocket],
  );

  const refresh = useCallback(async () => {
    if (!convIdRef.current) return;
    try {
      const [conv, msgs] = await Promise.all([
        client.getConversation(convIdRef.current),
        client.listMessages(convIdRef.current),
      ]);
      setConversation(conv);
      setMessages((prev) => mergeMessages(prev, msgs));
    } catch (e) {
      setError(e instanceof Error ? e.message : "refresh failed");
    }
  }, [client]);

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

  const sendMessage = useCallback(
    async (body: string) => {
      const id = convIdRef.current;
      if (!id) return;
      // Optimistic append; the socket echo is de-duplicated by id.
      const sent = await client.postMessage(id, body);
      setMessages((prev) => mergeMessage(prev, sent));
    },
    [client],
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
        if (res) {
          void adoptConversation(res.conversation, res.messages);
        } else {
          setStatus("idle");
        }
      })
      .catch((e) => {
        if (cancelled) return;
        setStatus("idle"); // no open thread is not an error to the user
        setError(e instanceof Error ? e.message : null);
      });
    return () => {
      cancelled = true;
      teardownSocket();
    };
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, []);

  // Reconnect when the app returns to the foreground; the OS suspends
  // sockets in the background.
  useEffect(() => {
    const onChange = (next: AppStateStatus) => {
      if (next === "active" && convIdRef.current && !socketRef.current) {
        const conv = conversation;
        if (!conv || conv.status !== "closed") {
          void connect(convIdRef.current);
        }
      }
    };
    const sub = AppState.addEventListener("change", onChange);
    return () => sub.remove();
  }, [connect, conversation]);

  return {
    conversation,
    messages,
    status,
    connected,
    error,
    startConversation,
    sendMessage,
    closeConversation,
    refresh,
  };
}
