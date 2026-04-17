"use client";

import {
  type CSSProperties,
  type FormEvent,
  useCallback,
  useEffect,
  useMemo,
  useRef,
  useState,
} from "react";
import type { Conversation, Message, WsEnvelope } from "./types";
import { useOttoChannel } from "./useOttoChannel";

/**
 * OttoInbox — the staff-side real-time support console.
 *
 * Like OttoWidget, this is host-agnostic. It calls the host app's proxy
 * routes (default `/api/otto/admin/*`) and opens a WebSocket to receive
 * new conversations and messages in real time.
 *
 * Designed to drop into any admin dashboard — the panel is a plain
 * two-pane layout and ships scoped CSS so the host app's stylesheet can't
 * conflict.
 */
export interface OttoInboxProps {
  /** Base URL for the admin otto proxy (default "/api/otto/admin"). */
  apiBaseUrl?: string;
  /** Builds the inbox-level WS URL. */
  buildInboxWsUrl?: () => string;
  /** Builds the per-conversation WS URL. */
  buildConversationWsUrl?: (conversationId: string) => string;
  /** Current staff member's user id — used to detect "mine" vs "theirs". */
  currentUserId: string;
  /** Optional style/classname passthrough for the outer wrapper. */
  style?: CSSProperties;
  className?: string;
}

const DEFAULT_INBOX_WS = () => {
  if (typeof window === "undefined") return "";
  const proto = window.location.protocol === "https:" ? "wss:" : "ws:";
  return `${proto}//${window.location.host}/api/otto/admin/ws`;
};
const DEFAULT_CONVERSATION_WS = (id: string) => {
  if (typeof window === "undefined") return "";
  const proto = window.location.protocol === "https:" ? "wss:" : "ws:";
  return `${proto}//${window.location.host}/api/otto/admin/conversations/${encodeURIComponent(id)}/ws`;
};

type InboxStatus = "pending" | "active" | "closed";

export function OttoInbox({
  apiBaseUrl = "/api/otto/admin",
  buildInboxWsUrl = DEFAULT_INBOX_WS,
  buildConversationWsUrl = DEFAULT_CONVERSATION_WS,
  currentUserId,
  style,
  className,
}: OttoInboxProps) {
  const base = useMemo(() => apiBaseUrl.replace(/\/+$/, ""), [apiBaseUrl]);
  const [statusFilter, setStatusFilter] = useState<InboxStatus>("pending");
  const [conversations, setConversations] = useState<Conversation[]>([]);
  const [selectedId, setSelectedId] = useState<string | null>(null);
  const [messages, setMessages] = useState<Message[]>([]);
  const [reply, setReply] = useState("");
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState<string | null>(null);
  const messagesRef = useRef<HTMLDivElement | null>(null);

  const selected =
    conversations.find((c) => c.id === selectedId) ?? null;

  const loadList = useCallback(
    async (next: InboxStatus) => {
      setError(null);
      try {
        const res = await fetch(
          `${base}/conversations?status=${encodeURIComponent(next)}`,
          { credentials: "include" },
        );
        if (!res.ok) throw new Error(`list failed (${res.status})`);
        const body = (await res.json()) as { conversations: Conversation[] };
        setConversations(body.conversations ?? []);
      } catch (e) {
        setError((e as Error).message);
      }
    },
    [base],
  );

  useEffect(() => {
    void loadList(statusFilter);
  }, [loadList, statusFilter]);

  // Inbox WS — new conversations + updates.
  const handleInboxEvent = useCallback(
    (env: WsEnvelope) => {
      if (env.type === "otto.conversation.created") {
        const payload = env.payload as { conversation: Conversation };
        if (statusFilter === "pending") {
          setConversations((prev) => {
            if (prev.some((c) => c.id === payload.conversation.id)) return prev;
            return [payload.conversation, ...prev];
          });
        }
      } else if (
        env.type === "otto.conversation.updated" ||
        env.type === "otto.conversation.closed"
      ) {
        const payload = env.payload as {
          conversation?: Conversation;
          conversation_id?: string;
        };
        if (payload.conversation) {
          setConversations((prev) =>
            prev.map((c) =>
              c.id === payload.conversation!.id ? payload.conversation! : c,
            ),
          );
        } else if (payload.conversation_id) {
          void loadList(statusFilter);
        }
      }
    },
    [loadList, statusFilter],
  );

  useOttoChannel({ url: buildInboxWsUrl(), onEvent: handleInboxEvent });

  // Selection: load messages when we pick a thread + subscribe to its room.
  useEffect(() => {
    if (!selectedId) {
      setMessages([]);
      return;
    }
    let cancelled = false;
    (async () => {
      try {
        const res = await fetch(
          `${base}/conversations/${encodeURIComponent(selectedId)}/messages`,
          { credentials: "include" },
        );
        if (!res.ok) throw new Error(`list msgs failed (${res.status})`);
        const body = (await res.json()) as { messages: Message[] };
        if (!cancelled) setMessages(body.messages ?? []);
      } catch (e) {
        if (!cancelled) setError((e as Error).message);
      }
    })();
    return () => {
      cancelled = true;
    };
  }, [base, selectedId]);

  const convWsUrl = selectedId ? buildConversationWsUrl(selectedId) : null;
  useOttoChannel({
    url: convWsUrl,
    onEvent: (env) => {
      if (env.type === "otto.message.created") {
        const payload = env.payload as { message: Message };
        setMessages((prev) =>
          prev.some((m) => m.id === payload.message.id)
            ? prev
            : [...prev, payload.message],
        );
      }
      if (
        env.type === "otto.conversation.updated" ||
        env.type === "otto.conversation.closed"
      ) {
        const payload = env.payload as { conversation?: Conversation };
        if (payload.conversation) {
          setConversations((prev) =>
            prev.map((c) =>
              c.id === payload.conversation!.id ? payload.conversation! : c,
            ),
          );
        }
      }
    },
  });

  useEffect(() => {
    const el = messagesRef.current;
    if (!el) return;
    el.scrollTop = el.scrollHeight;
  }, [messages.length, selectedId]);

  const accept = useCallback(
    async (id: string) => {
      setBusy(true);
      setError(null);
      try {
        const res = await fetch(
          `${base}/conversations/${encodeURIComponent(id)}/accept`,
          { method: "POST", credentials: "include" },
        );
        if (!res.ok) throw new Error(`accept failed (${res.status})`);
        const body = (await res.json()) as { conversation: Conversation };
        setConversations((prev) =>
          prev.map((c) => (c.id === body.conversation.id ? body.conversation : c)),
        );
        setSelectedId(body.conversation.id);
      } catch (e) {
        setError((e as Error).message);
      } finally {
        setBusy(false);
      }
    },
    [base],
  );

  const send = useCallback(
    async (e: FormEvent) => {
      e.preventDefault();
      if (!selected || busy) return;
      const body = reply.trim();
      if (!body) return;
      setBusy(true);
      setError(null);
      try {
        const res = await fetch(
          `${base}/conversations/${encodeURIComponent(selected.id)}/messages`,
          {
            method: "POST",
            credentials: "include",
            headers: { "Content-Type": "application/json" },
            body: JSON.stringify({ body }),
          },
        );
        if (!res.ok) throw new Error(`send failed (${res.status})`);
        const resBody = (await res.json()) as { message: Message };
        setMessages((prev) =>
          prev.some((m) => m.id === resBody.message.id)
            ? prev
            : [...prev, resBody.message],
        );
        setReply("");
      } catch (err) {
        setError((err as Error).message);
      } finally {
        setBusy(false);
      }
    },
    [base, busy, reply, selected],
  );

  const close = useCallback(async () => {
    if (!selected) return;
    setBusy(true);
    try {
      const res = await fetch(
        `${base}/conversations/${encodeURIComponent(selected.id)}/close`,
        { method: "POST", credentials: "include" },
      );
      if (!res.ok) throw new Error(`close failed (${res.status})`);
    } catch (e) {
      setError((e as Error).message);
    } finally {
      setBusy(false);
    }
  }, [base, selected]);

  const canReply =
    !!selected &&
    selected.status !== "closed" &&
    (!!selected.assignee && selected.assignee.user_id === currentUserId);

  return (
    <div className={`otto-inbox ${className ?? ""}`} style={style}>
      <aside className="otto-inbox__list">
        <div className="otto-inbox__tabs">
          {(["pending", "active", "closed"] as InboxStatus[]).map((s) => (
            <button
              key={s}
              type="button"
              className={`otto-inbox__tab ${statusFilter === s ? "is-active" : ""}`}
              onClick={() => setStatusFilter(s)}
            >
              {s.charAt(0).toUpperCase() + s.slice(1)}
            </button>
          ))}
        </div>
        {conversations.length === 0 ? (
          <div className="otto-inbox__empty">No conversations here yet.</div>
        ) : (
          conversations.map((c) => (
            <button
              key={c.id}
              type="button"
              className={`otto-inbox__row ${c.id === selectedId ? "is-active" : ""}`}
              onClick={() => setSelectedId(c.id)}
            >
              <div className="otto-inbox__row-head">
                <strong>
                  {c.customer.name ||
                    c.customer.email ||
                    "Anonymous visitor"}
                </strong>
                <span className="otto-inbox__row-time">
                  {formatRelative(c.last_message_at)}
                </span>
              </div>
              <div className="otto-inbox__row-sub">
                {c.subject || "(no subject)"}
              </div>
              <div className="otto-inbox__row-meta">
                <span className={`otto-inbox__pill otto-inbox__pill--${c.status}`}>
                  {c.status}
                </span>
                {c.unread_count_staff > 0 && (
                  <span className="otto-inbox__unread">
                    {c.unread_count_staff}
                  </span>
                )}
              </div>
            </button>
          ))
        )}
      </aside>

      <section className="otto-inbox__thread">
        {!selected ? (
          <div className="otto-inbox__placeholder">
            Pick a conversation on the left to start replying.
          </div>
        ) : (
          <>
            <header className="otto-inbox__thread-head">
              <div>
                <strong>
                  {selected.customer.name ||
                    selected.customer.email ||
                    "Anonymous visitor"}
                </strong>
                <div className="otto-inbox__thread-subtitle">
                  {selected.subject || "(no subject)"}
                </div>
              </div>
              <div className="otto-inbox__thread-actions">
                {selected.status === "pending" && (
                  <button
                    type="button"
                    className="otto-inbox__btn otto-inbox__btn--primary"
                    onClick={() => accept(selected.id)}
                    disabled={busy}
                  >
                    Accept & reply
                  </button>
                )}
                {selected.status === "active" && (
                  <button
                    type="button"
                    className="otto-inbox__btn"
                    onClick={close}
                    disabled={busy}
                  >
                    Close
                  </button>
                )}
              </div>
            </header>

            <div className="otto-inbox__messages" ref={messagesRef}>
              {messages.map((m) => (
                <div
                  key={m.id}
                  className={`otto-inbox__bubble otto-inbox__bubble--${m.sender_type}`}
                >
                  <div className="otto-inbox__bubble-body">{m.body}</div>
                  <div className="otto-inbox__bubble-meta">
                    {m.sender_name ? `${m.sender_name} · ` : ""}
                    {formatTime(m.created_at)}
                  </div>
                </div>
              ))}
            </div>

            <form className="otto-inbox__composer" onSubmit={send}>
              <textarea
                className="otto-inbox__textarea"
                value={reply}
                onChange={(e) => setReply(e.target.value)}
                placeholder={
                  canReply
                    ? "Write a reply..."
                    : selected.status === "pending"
                      ? "Accept the conversation to start replying."
                      : selected.status === "closed"
                        ? "This conversation is closed."
                        : "Only the assigned agent can reply."
                }
                disabled={!canReply || busy}
              />
              {error && <div className="otto-inbox__error">{error}</div>}
              <button
                type="submit"
                className="otto-inbox__btn otto-inbox__btn--primary"
                disabled={!canReply || busy || !reply.trim()}
              >
                Send
              </button>
            </form>
          </>
        )}
      </section>
    </div>
  );
}

function formatTime(iso: string): string {
  try {
    const d = new Date(iso);
    const hh = d.getHours().toString().padStart(2, "0");
    const mm = d.getMinutes().toString().padStart(2, "0");
    return `${hh}:${mm}`;
  } catch {
    return "";
  }
}

function formatRelative(iso: string): string {
  try {
    const ms = Date.now() - new Date(iso).getTime();
    const s = Math.round(ms / 1000);
    if (s < 60) return `${s}s`;
    if (s < 3600) return `${Math.round(s / 60)}m`;
    if (s < 86400) return `${Math.round(s / 3600)}h`;
    return `${Math.round(s / 86400)}d`;
  } catch {
    return "";
  }
}
