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
import { buildOttoApi } from "./api";
import type {
  Conversation,
  Message,
  WsEnvelope,
} from "./types";
import { useOttoChannel } from "./useOttoChannel";

// The widget talks to the host app over /api/otto (REST) and /api/otto/ws
// (WebSocket). Hosts configure both paths via props so they can mount the
// proxy wherever they like — not just in a marketplace storefront.
export interface OttoWidgetProps {
  /** REST base for the otto proxy (default "/api/otto"). */
  apiBaseUrl?: string;
  /** Function returning the WebSocket URL for a given conversation id.
   *  Default assumes the proxy exposes `/api/otto/conversations/:id/ws`
   *  over the same origin with the appropriate protocol. */
  buildWsUrl?: (conversationId: string) => string;
  /** Displayed in the launcher pill. */
  launcherLabel?: string;
  /** Shown in the header when a conversation is in progress. */
  productName?: string;
  /** Welcome copy shown before the customer sends their first message. */
  intro?: string;
  /** Optional customer name prefill (for logged-in users). */
  customerName?: string;
  /** Optional customer email prefill. */
  customerEmail?: string;
  /** Optional style override (position, offsets, z-index). */
  style?: CSSProperties;
  /** Optional theme — maps to CSS custom properties. */
  theme?: Partial<OttoTheme>;
}

export interface OttoTheme {
  primary: string;
  primaryFg: string;
  accent: string;
}

const DEFAULT_BUILD_WS_URL = (conversationId: string) => {
  if (typeof window === "undefined") return "";
  const proto = window.location.protocol === "https:" ? "wss:" : "ws:";
  return `${proto}//${window.location.host}/api/otto/conversations/${encodeURIComponent(conversationId)}/ws`;
};

/**
 * OttoWidget — the customer-facing floating support chat.
 *
 * Designed to be host-agnostic: drop it in any React 19 app, point
 * `apiBaseUrl` at a proxy that forwards to the otto service, done.
 * Session binding (which customer sees which thread) is handled by an
 * HttpOnly cookie the service issues; this component never reads it.
 */
export function OttoWidget({
  apiBaseUrl = "/api/otto",
  buildWsUrl = DEFAULT_BUILD_WS_URL,
  launcherLabel = "Chat with support",
  productName = "Support",
  intro = "Hey! Leave a message and someone from our team will be with you shortly. You'll see their reply here in real time.",
  customerName,
  customerEmail,
  style,
  theme,
}: OttoWidgetProps) {
  const api = useMemo(() => buildOttoApi(apiBaseUrl), [apiBaseUrl]);

  const [open, setOpen] = useState(false);
  const [conversation, setConversation] = useState<Conversation | null>(null);
  const [messages, setMessages] = useState<Message[]>([]);
  const [input, setInput] = useState("");
  const [name, setName] = useState(customerName ?? "");
  const [email, setEmail] = useState(customerEmail ?? "");
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState<string | null>(null);
  const messagesRef = useRef<HTMLDivElement | null>(null);

  const wsUrl = conversation ? buildWsUrl(conversation.id) : null;

  const handleEvent = useCallback((env: WsEnvelope) => {
    if (env.type === "otto.message.created") {
      const payload = env.payload as { message: Message };
      setMessages((prev) =>
        prev.some((m) => m.id === payload.message.id)
          ? prev
          : [...prev, payload.message],
      );
    } else if (
      env.type === "otto.conversation.updated" ||
      env.type === "otto.conversation.closed"
    ) {
      const payload = env.payload as { conversation?: Conversation };
      if (payload.conversation) setConversation(payload.conversation);
    }
  }, []);

  useOttoChannel({ url: wsUrl, onEvent: handleEvent });

  useEffect(() => {
    const el = messagesRef.current;
    if (!el) return;
    el.scrollTop = el.scrollHeight;
  }, [messages.length, open]);

  const resetError = () => setError(null);

  const submit = useCallback(
    async (e: FormEvent) => {
      e.preventDefault();
      if (busy) return;
      const text = input.trim();
      if (!text) return;
      setBusy(true);
      setError(null);
      try {
        if (!conversation) {
          const res = await api.startConversation({
            message: text,
            name: name || undefined,
            email: email || undefined,
          });
          setConversation(res.conversation);
          setMessages([res.first_message]);
        } else {
          const res = await api.sendMessage(conversation.id, text);
          setMessages((prev) =>
            prev.some((m) => m.id === res.message.id) ? prev : [...prev, res.message],
          );
        }
        setInput("");
      } catch (err) {
        setError((err as Error).message || "Something went wrong, try again.");
      } finally {
        setBusy(false);
      }
    },
    [api, busy, conversation, email, input, name],
  );

  const themedStyle = useMemo<CSSProperties>(() => {
    const vars: Record<string, string> = {};
    if (theme?.primary) vars["--otto-primary"] = theme.primary;
    if (theme?.primaryFg) vars["--otto-primary-fg"] = theme.primaryFg;
    if (theme?.accent) vars["--otto-accent"] = theme.accent;
    return { ...vars, ...style } as CSSProperties;
  }, [style, theme]);

  const subtitle = conversation
    ? statusSubtitle(conversation)
    : "Usually replies within a few minutes";

  return (
    <div className="otto-widget" style={themedStyle}>
      {!open ? (
        <button
          type="button"
          className="otto-widget__launcher"
          onClick={() => {
            setOpen(true);
            resetError();
          }}
          aria-label="Open support chat"
        >
          <span className="otto-widget__launcher-dot" aria-hidden="true" />
          {launcherLabel}
        </button>
      ) : (
        <section className="otto-widget__panel" role="dialog" aria-label="Support chat">
          <header className="otto-widget__header">
            <div className="otto-widget__title">
              <strong>{productName}</strong>
              <span className="otto-widget__subtitle" title={subtitle}>
                {subtitle}
              </span>
            </div>
            <button
              type="button"
              className="otto-widget__close"
              onClick={() => setOpen(false)}
              aria-label="Close chat"
            >
              ×
            </button>
          </header>

          {conversation && (
            <div
              className="otto-widget__status"
              data-tone={conversation.status === "active" ? "active" : conversation.status}
            >
              {statusLabel(conversation)}
            </div>
          )}

          {!conversation && (
            <div className="otto-widget__intro">
              <strong>Start a conversation</strong>
              <p style={{ marginTop: 6, marginBottom: 0 }}>{intro}</p>
            </div>
          )}

          <div className="otto-widget__messages" ref={messagesRef}>
            {messages.length === 0 && !conversation && (
              <p className="otto-widget__empty">
                Your messages will appear here.
              </p>
            )}
            {messages.map((m) => (
              <MessageBubble key={m.id} message={m} />
            ))}
          </div>

          <form className="otto-widget__form" onSubmit={submit}>
            {!conversation && (
              <div className="otto-widget__row">
                <input
                  className="otto-widget__input"
                  placeholder="Your name (optional)"
                  value={name}
                  onChange={(e) => setName(e.target.value)}
                  disabled={busy}
                  aria-label="Your name"
                />
                <input
                  className="otto-widget__input"
                  placeholder="Email (optional)"
                  value={email}
                  onChange={(e) => setEmail(e.target.value)}
                  disabled={busy}
                  aria-label="Your email"
                  type="email"
                />
              </div>
            )}
            <textarea
              className="otto-widget__textarea"
              placeholder={
                conversation?.status === "closed"
                  ? "This conversation is closed."
                  : "Type your message..."
              }
              value={input}
              onChange={(e) => setInput(e.target.value)}
              disabled={busy || conversation?.status === "closed"}
              onKeyDown={(e) => {
                if (e.key === "Enter" && !e.shiftKey) {
                  e.preventDefault();
                  void submit(e as unknown as FormEvent);
                }
              }}
              aria-label="Message"
            />
            {error && <div className="otto-widget__error">{error}</div>}
            <button
              type="submit"
              className="otto-widget__submit"
              disabled={busy || !input.trim() || conversation?.status === "closed"}
            >
              {busy ? "Sending..." : conversation ? "Send" : "Start chat"}
            </button>
          </form>
        </section>
      )}
    </div>
  );
}

function MessageBubble({ message }: { message: Message }) {
  const className =
    message.sender_type === "customer"
      ? "otto-widget__msg otto-widget__msg--customer"
      : message.sender_type === "staff"
        ? "otto-widget__msg otto-widget__msg--staff"
        : "otto-widget__msg otto-widget__msg--system";
  return (
    <div className={className}>
      {message.body}
      {message.sender_type !== "system" && (
        <span className="otto-widget__msg-meta">
          {message.sender_name ? `${message.sender_name} · ` : ""}
          {formatTime(message.created_at)}
        </span>
      )}
    </div>
  );
}

function statusLabel(c: Conversation): string {
  if (c.status === "pending") return "Waiting for an agent to join...";
  if (c.status === "closed") return "This conversation has been closed.";
  const who = c.assignee?.name ?? c.assignee?.email ?? "An agent";
  return `${who} is helping you now.`;
}

function statusSubtitle(c: Conversation): string {
  if (c.status === "closed") return "Closed";
  if (c.status === "pending") return "Queued — we'll be right with you";
  return c.assignee?.name ?? c.assignee?.email ?? "Agent connected";
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
