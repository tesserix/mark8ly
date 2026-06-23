"use client";

// Storefront customer notification bell. Polls the customer's unread count,
// chimes on a new notification, and shows a dropdown of recent items. Renders
// nothing when the customer isn't signed in (the unread-count call 401s).

import { useCallback, useEffect, useRef, useState } from "react";
import Link from "next/link";

interface NotificationItem {
  id: string;
  type: string;
  title: string;
  message: string | null;
  is_read: boolean;
  created_at: string;
}

// Short two-tone Web Audio chime — no asset, no-op if autoplay is blocked.
let audioCtx: AudioContext | null = null;
function playChime() {
  try {
    const Ctx =
      window.AudioContext ||
      (window as unknown as { webkitAudioContext: typeof AudioContext }).webkitAudioContext;
    audioCtx = audioCtx ?? new Ctx();
    const ctx = audioCtx;
    const o = ctx.createOscillator();
    const g = ctx.createGain();
    o.connect(g);
    g.connect(ctx.destination);
    o.type = "sine";
    o.frequency.setValueAtTime(880, ctx.currentTime);
    o.frequency.setValueAtTime(1175, ctx.currentTime + 0.1);
    g.gain.setValueAtTime(0.0001, ctx.currentTime);
    g.gain.exponentialRampToValueAtTime(0.14, ctx.currentTime + 0.02);
    g.gain.exponentialRampToValueAtTime(0.0001, ctx.currentTime + 0.4);
    o.start();
    o.stop(ctx.currentTime + 0.4);
  } catch {
    /* blocked / unsupported */
  }
}

function hrefFor(type: string): string {
  if (type.startsWith("order") || type === "refund_processed") return "/account/orders";
  return "/account";
}

function timeAgo(iso: string): string {
  const d = new Date(iso);
  const s = Math.floor((Date.now() - d.getTime()) / 1000);
  if (Number.isNaN(s)) return "";
  if (s < 60) return "just now";
  if (s < 3600) return `${Math.floor(s / 60)}m ago`;
  if (s < 86400) return `${Math.floor(s / 3600)}h ago`;
  return `${Math.floor(s / 86400)}d ago`;
}

export function CustomerNotificationBell() {
  const [available, setAvailable] = useState(false);
  const [open, setOpen] = useState(false);
  const [unread, setUnread] = useState(0);
  const [items, setItems] = useState<NotificationItem[]>([]);
  const [loading, setLoading] = useState(false);
  const prevRef = useRef<number | null>(null);
  const ref = useRef<HTMLDivElement>(null);

  const fetchUnread = useCallback(async () => {
    try {
      const res = await fetch("/api/account/notifications/unread-count", {
        cache: "no-store",
      });
      if (res.status === 401) {
        setAvailable(false);
        return;
      }
      if (!res.ok) return;
      setAvailable(true);
      const data = (await res.json()) as { unread_count?: number };
      const next = data.unread_count ?? 0;
      const prev = prevRef.current;
      prevRef.current = next;
      if (prev !== null && next > prev) playChime();
      setUnread(next);
    } catch {
      /* transient */
    }
  }, []);

  useEffect(() => {
    let id: ReturnType<typeof setInterval> | null = null;
    const start = () => {
      if (id === null) id = setInterval(fetchUnread, 30_000);
    };
    const stop = () => {
      if (id !== null) {
        clearInterval(id);
        id = null;
      }
    };
    void fetchUnread();
    if (document.visibilityState === "visible") start();
    const onVis = () => {
      if (document.visibilityState === "visible") {
        void fetchUnread();
        start();
      } else {
        stop();
      }
    };
    window.addEventListener("focus", onVis);
    document.addEventListener("visibilitychange", onVis);
    return () => {
      stop();
      window.removeEventListener("focus", onVis);
      document.removeEventListener("visibilitychange", onVis);
    };
  }, [fetchUnread]);

  useEffect(() => {
    function onClick(e: MouseEvent) {
      if (ref.current && !ref.current.contains(e.target as Node)) setOpen(false);
    }
    if (open) document.addEventListener("mousedown", onClick);
    return () => document.removeEventListener("mousedown", onClick);
  }, [open]);

  async function toggle() {
    const next = !open;
    setOpen(next);
    if (next) {
      setLoading(true);
      try {
        const res = await fetch("/api/account/notifications", { cache: "no-store" });
        if (res.ok) {
          const data = (await res.json()) as { notifications?: NotificationItem[] };
          setItems(data.notifications ?? []);
        }
      } catch {
        /* transient */
      } finally {
        setLoading(false);
      }
    }
  }

  async function markAllRead() {
    try {
      await fetch("/api/account/notifications/read-all", { method: "PATCH" });
      setUnread(0);
      setItems((prev) => prev.map((n) => ({ ...n, is_read: true })));
    } catch {
      /* transient */
    }
  }

  if (!available) return null;

  return (
    <div className="relative" ref={ref}>
      <button
        type="button"
        onClick={toggle}
        aria-label={`Notifications${unread > 0 ? ` (${unread} unread)` : ""}`}
        className="relative inline-flex h-11 w-11 items-center justify-center rounded-md text-[color:var(--storefront-text,var(--ink-900))] opacity-70 transition-opacity hover:opacity-100"
      >
        <svg width="18" height="18" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="2" strokeLinecap="round" strokeLinejoin="round" aria-hidden="true">
          <path d="M6 8a6 6 0 0 1 12 0c0 7 3 9 3 9H3s3-2 3-9" />
          <path d="M10.3 21a1.94 1.94 0 0 0 3.4 0" />
        </svg>
        {unread > 0 && (
          <span className="absolute right-2 top-2 h-2 w-2 rounded-full bg-[color:var(--storefront-accent,var(--moss-700))]" />
        )}
      </button>

      {open && (
        <div className="absolute right-0 top-full z-50 mt-2 max-h-96 w-[calc(100vw-3rem)] overflow-y-auto rounded-md border border-[color:var(--ink-900)] border-opacity-10 bg-[color:var(--paper-50,#fff)] shadow-lg sm:w-80">
          <div className="flex items-center justify-between border-b border-[color:var(--ink-900)] border-opacity-10 px-4 py-3">
            <p className="text-sm font-medium text-[color:var(--ink-900)]">Notifications</p>
            {unread > 0 && (
              <button type="button" onClick={markAllRead} className="text-xs font-medium text-[color:var(--storefront-accent,var(--moss-700))] hover:underline">
                Mark all read
              </button>
            )}
          </div>
          {loading ? (
            <div className="px-4 py-8 text-center text-sm opacity-60">Loading…</div>
          ) : items.length === 0 ? (
            <div className="px-4 py-8 text-center text-sm opacity-60">No notifications yet.</div>
          ) : (
            <ul className="divide-y divide-[color:var(--ink-900)] divide-opacity-10">
              {items.map((n) => (
                <li key={n.id}>
                  <Link
                    href={hrefFor(n.type)}
                    onClick={() => setOpen(false)}
                    className={`block px-4 py-3 transition-colors hover:bg-[color:var(--ink-900)] hover:bg-opacity-[0.03] ${n.is_read ? "" : "bg-[color:var(--moss-700)] bg-opacity-[0.05]"}`}
                  >
                    <div className="flex items-start justify-between gap-2">
                      <p className="truncate text-sm font-medium text-[color:var(--ink-900)]">{n.title}</p>
                      {!n.is_read && <span className="mt-1.5 h-1.5 w-1.5 shrink-0 rounded-full bg-[color:var(--storefront-accent,var(--moss-700))]" />}
                    </div>
                    {n.message && <p className="mt-0.5 line-clamp-2 text-xs opacity-70">{n.message}</p>}
                    <p className="mt-1 text-[10px] opacity-50">{timeAgo(n.created_at)}</p>
                  </Link>
                </li>
              ))}
            </ul>
          )}
        </div>
      )}
    </div>
  );
}
