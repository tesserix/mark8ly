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
   *  Default opens wss://{host}/api/v1/storefront/otto/conversations/:id/ws
   *  — routed directly to Otto by Istio, bypassing the Next.js proxy. */
  buildWsUrl?: (conversationId: string) => string;
  /** Displayed in the launcher pill. */
  launcherLabel?: string;
  /** Shown in the header when a conversation is in progress. */
  productName?: string;
  /** Welcome copy shown before the customer sends their first message. */
  intro?: string;
  /** Optional customer name prefill (for logged-in users). When both
   *  name and email are provided the OTP step is skipped entirely — the
   *  host has already vouched for the identity. */
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

// The widget moves through three phases:
//   collect    — customer enters name/email/message (anonymous only)
//   verify     — customer enters the 6-digit code we just emailed
//   chat       — thread is live; WebSocket is open
// Logged-in customers skip "collect" and "verify" entirely.
type Phase = "collect" | "verify" | "chat";

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
  intro = "Leave a message and someone from our team will be with you shortly. You'll see their reply here in real time.",
  customerName,
  customerEmail,
  style,
  theme,
}: OttoWidgetProps) {
  const api = useMemo(() => buildOttoApi(apiBaseUrl), [apiBaseUrl]);
  // The WS ticket endpoint is always the same-origin REST proxy, so the
  // Next.js layer can attach our auth cookie + internal-auth header.
  const ticketUrl = useMemo(() => {
    const base = apiBaseUrl.replace(/\/+$/, "");
    return (conversationId: string) =>
      `${base}/conversations/${encodeURIComponent(conversationId)}/ws-ticket`;
  }, [apiBaseUrl]);

  // Logged-in users (both name + email prefilled) skip verification.
  const isLoggedIn = Boolean(
    customerName?.trim() && customerEmail?.trim(),
  );
  const initialPhase: Phase = isLoggedIn ? "chat" : "collect";

  const [open, setOpen] = useState(false);
  const [phase, setPhase] = useState<Phase>(initialPhase);
  const [conversation, setConversation] = useState<Conversation | null>(null);
  const [messages, setMessages] = useState<Message[]>([]);
  const [name, setName] = useState(customerName ?? "");
  const [email, setEmail] = useState(customerEmail ?? "");
  const [pendingMessage, setPendingMessage] = useState("");
  const [chatDraft, setChatDraft] = useState("");
  const [otpDigits, setOtpDigits] = useState<string[]>(() =>
    new Array(6).fill(""),
  );
  const [maskedEmail, setMaskedEmail] = useState("");
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState<string | null>(null);
  const messagesRef = useRef<HTMLDivElement | null>(null);
  const otpInputsRef = useRef<HTMLInputElement[]>([]);

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

  const wsTicketUrl = conversation ? ticketUrl(conversation.id) : null;
  useOttoChannel({
    url: wsUrl,
    ticketUrl: wsTicketUrl,
    onEvent: handleEvent,
  });

  // Resume on mount — if the otto_session cookie points at an open
  // thread, restore it so page reloads don't wipe the conversation.
  // Silent failure is fine: a missing / expired cookie returns
  // {conversation: null} and the widget keeps its default state.
  useEffect(() => {
    let cancelled = false;
    (async () => {
      try {
        const res = await api.resume();
        if (cancelled || !res.conversation) return;
        setConversation(res.conversation);
        setMessages(res.messages ?? []);
        setPhase("chat");
      } catch {
        /* silent — widget falls back to the collect/chat phase defaults */
      }
    })();
    return () => {
      cancelled = true;
    };
  }, [api]);

  useEffect(() => {
    const el = messagesRef.current;
    if (!el) return;
    el.scrollTop = el.scrollHeight;
  }, [messages.length, open, phase]);

  // Auto-focus the first OTP input when we transition to the verify phase.
  useEffect(() => {
    if (phase === "verify") {
      otpInputsRef.current[0]?.focus();
    }
  }, [phase]);

  const resetError = () => setError(null);

  const otpValue = useMemo(() => otpDigits.join(""), [otpDigits]);
  const otpComplete = otpValue.length === 6 && /^\d{6}$/.test(otpValue);

  // startConversationNow is declared first because both the collect and
  // verify submit handlers below need it in their useCallback deps —
  // referencing a later const would trip the temporal dead zone at
  // render time.
  const startConversationNow = useCallback(
    async (input: { otpCode: string | undefined; message: string }) => {
      const res = await api.startConversation({
        message: input.message,
        otp_code: input.otpCode,
        name: name.trim() || undefined,
        email: email.trim() || undefined,
      });
      setConversation(res.conversation);
      setMessages([res.first_message]);
      setPendingMessage("");
      setChatDraft("");
      setPhase("chat");
    },
    [api, email, name],
  );

  // ── Phase 1: collect ─────────────────────────────────────────────────
  const submitCollect = useCallback(
    async (e: FormEvent) => {
      e.preventDefault();
      if (busy) return;
      const trimmedEmail = email.trim();
      const msg = pendingMessage.trim();
      if (!trimmedEmail || !msg) {
        setError("Email and message are both required.");
        return;
      }
      setBusy(true);
      setError(null);
      try {
        if (isLoggedIn) {
          // This branch is only reachable when props change mid-session,
          // but handle it for safety.
          await startConversationNow({ otpCode: undefined, message: msg });
          return;
        }
        const res = await api.requestOtp({
          email: trimmedEmail,
          name: name.trim() || undefined,
          store_name: productName,
        });
        setMaskedEmail(res.masked_to);
        setOtpDigits(new Array(6).fill(""));
        setPhase("verify");
      } catch (err) {
        setError((err as Error).message || "Could not send the code, try again.");
      } finally {
        setBusy(false);
      }
    },
    [api, busy, email, isLoggedIn, name, pendingMessage, productName, startConversationNow],
  );

  // ── Phase 2: verify ──────────────────────────────────────────────────
  const submitVerify = useCallback(
    async (e: FormEvent) => {
      e.preventDefault();
      if (busy || !otpComplete) return;
      setBusy(true);
      setError(null);
      try {
        await startConversationNow({
          otpCode: otpValue,
          message: pendingMessage.trim(),
        });
      } catch (err) {
        setError((err as Error).message || "Could not verify that code.");
        // Blank the OTP so the customer can retype cleanly.
        setOtpDigits(new Array(6).fill(""));
        otpInputsRef.current[0]?.focus();
      } finally {
        setBusy(false);
      }
    },
    [busy, otpComplete, otpValue, pendingMessage, startConversationNow],
  );

  // Re-request a fresh OTP (new challenge, reset cooldown).
  const resendOtp = useCallback(async () => {
    if (busy) return;
    setBusy(true);
    setError(null);
    try {
      const res = await api.requestOtp({
        email: email.trim(),
        name: name.trim() || undefined,
        store_name: productName,
      });
      setMaskedEmail(res.masked_to);
      setOtpDigits(new Array(6).fill(""));
      otpInputsRef.current[0]?.focus();
    } catch (err) {
      setError((err as Error).message || "Could not resend the code.");
    } finally {
      setBusy(false);
    }
  }, [api, busy, email, name, productName]);

  // ── Phase 3: chat ────────────────────────────────────────────────────
  // Logged-in users: submitting from the chat phase (their first message
  // is entered here too) must call startConversation directly if they
  // haven't opened a thread yet.
  const submitChat = useCallback(
    async (e: FormEvent) => {
      e.preventDefault();
      if (busy) return;
      const text = chatDraft.trim();
      if (!text) return;
      setBusy(true);
      setError(null);
      try {
        if (!conversation) {
          // Logged-in path — no OTP.
          await startConversationNow({ otpCode: undefined, message: text });
        } else {
          const res = await api.sendMessage(conversation.id, text);
          setMessages((prev) =>
            prev.some((m) => m.id === res.message.id)
              ? prev
              : [...prev, res.message],
          );
          setChatDraft("");
        }
      } catch (err) {
        setError((err as Error).message || "Message did not send.");
      } finally {
        setBusy(false);
      }
    },
    [api, busy, chatDraft, conversation, startConversationNow],
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
    : isLoggedIn
      ? `Hi ${firstWord(customerName)} — we reply within a few minutes`
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
              {conversation?.case_id && (
                <span
                  className="otto-widget__case-id"
                  title="Quote this reference if you contact us again"
                >
                  Case {conversation.case_id}
                </span>
              )}
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

          {/* COLLECT */}
          {phase === "collect" && (
            <>
              <div className="otto-widget__intro">
                <strong>Start a conversation</strong>
                <p style={{ marginTop: 6, marginBottom: 0 }}>{intro}</p>
              </div>
              <form className="otto-widget__form" onSubmit={submitCollect}>
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
                    placeholder="Email"
                    type="email"
                    required
                    value={email}
                    onChange={(e) => setEmail(e.target.value)}
                    disabled={busy}
                    aria-label="Your email"
                  />
                </div>
                <textarea
                  className="otto-widget__textarea"
                  placeholder="Type your message..."
                  value={pendingMessage}
                  onChange={(e) => setPendingMessage(e.target.value)}
                  disabled={busy}
                  aria-label="Message"
                />
                {error && <div className="otto-widget__error">{error}</div>}
                <button
                  type="submit"
                  className="otto-widget__submit"
                  disabled={busy || !email.trim() || !pendingMessage.trim()}
                >
                  {busy ? "Sending code..." : "Continue"}
                </button>
                <p className="otto-widget__fineprint">
                  We&apos;ll email you a 6-digit code to confirm it&apos;s really you.
                </p>
              </form>
            </>
          )}

          {/* VERIFY */}
          {phase === "verify" && (
            <>
              <div className="otto-widget__intro">
                <strong>Check your inbox</strong>
                <p style={{ marginTop: 6, marginBottom: 0 }}>
                  We sent a 6-digit code to <strong>{maskedEmail || email}</strong>.
                  Enter it below to continue.
                </p>
              </div>
              <form className="otto-widget__form" onSubmit={submitVerify}>
                <div
                  className="otto-widget__otp-row"
                  role="group"
                  aria-label="6-digit verification code"
                >
                  {otpDigits.map((digit, idx) => (
                    <input
                      key={idx}
                      ref={(el) => {
                        if (el) otpInputsRef.current[idx] = el;
                      }}
                      className="otto-widget__otp-cell"
                      inputMode="numeric"
                      autoComplete={idx === 0 ? "one-time-code" : "off"}
                      maxLength={1}
                      value={digit}
                      onChange={(e) => {
                        const v = e.target.value.replace(/\D/g, "").slice(0, 1);
                        setOtpDigits((prev) => {
                          const next = [...prev];
                          next[idx] = v;
                          return next;
                        });
                        if (v && idx < 5) otpInputsRef.current[idx + 1]?.focus();
                      }}
                      onKeyDown={(e) => {
                        if (e.key === "Backspace" && !otpDigits[idx] && idx > 0) {
                          otpInputsRef.current[idx - 1]?.focus();
                        }
                      }}
                      onPaste={(e) => {
                        const pasted = e.clipboardData
                          .getData("text")
                          .replace(/\D/g, "")
                          .slice(0, 6);
                        if (pasted.length === 0) return;
                        e.preventDefault();
                        const next = new Array(6).fill("");
                        for (let i = 0; i < pasted.length; i++) {
                          next[i] = pasted[i];
                        }
                        setOtpDigits(next);
                        const nextIdx = Math.min(pasted.length, 5);
                        otpInputsRef.current[nextIdx]?.focus();
                      }}
                      disabled={busy}
                      aria-label={`Digit ${idx + 1}`}
                    />
                  ))}
                </div>
                {error && <div className="otto-widget__error">{error}</div>}
                <button
                  type="submit"
                  className="otto-widget__submit"
                  disabled={busy || !otpComplete}
                >
                  {busy ? "Verifying..." : "Start chat"}
                </button>
                <div className="otto-widget__verify-actions">
                  <button
                    type="button"
                    className="otto-widget__link"
                    onClick={resendOtp}
                    disabled={busy}
                  >
                    Resend code
                  </button>
                  <button
                    type="button"
                    className="otto-widget__link"
                    onClick={() => {
                      setPhase("collect");
                      setOtpDigits(new Array(6).fill(""));
                      resetError();
                    }}
                    disabled={busy}
                  >
                    Edit email
                  </button>
                </div>
              </form>
            </>
          )}

          {/* CHAT */}
          {phase === "chat" && (
            <>
              <div className="otto-widget__messages" ref={messagesRef}>
                {messages.length === 0 && !conversation && (
                  <p className="otto-widget__empty">
                    {isLoggedIn
                      ? "Your messages will appear here."
                      : "Your messages will appear here."}
                  </p>
                )}
                {messages.map((m) => (
                  <MessageBubble key={m.id} message={m} />
                ))}
              </div>
              <form className="otto-widget__form" onSubmit={submitChat}>
                <textarea
                  className="otto-widget__textarea"
                  placeholder={
                    conversation?.status === "closed"
                      ? "This conversation is closed."
                      : "Type your message..."
                  }
                  value={chatDraft}
                  onChange={(e) => setChatDraft(e.target.value)}
                  disabled={busy || conversation?.status === "closed"}
                  onKeyDown={(e) => {
                    if (e.key === "Enter" && !e.shiftKey) {
                      e.preventDefault();
                      void submitChat(e as unknown as FormEvent);
                    }
                  }}
                  aria-label="Message"
                />
                {error && <div className="otto-widget__error">{error}</div>}
                <button
                  type="submit"
                  className="otto-widget__submit"
                  disabled={
                    busy ||
                    !chatDraft.trim() ||
                    conversation?.status === "closed"
                  }
                >
                  {busy ? "Sending..." : conversation ? "Send" : "Start chat"}
                </button>
              </form>
            </>
          )}
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

function firstWord(name: string | undefined): string {
  if (!name) return "there";
  return name.trim().split(/\s+/)[0] || "there";
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
