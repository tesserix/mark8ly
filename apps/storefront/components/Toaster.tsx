"use client";

// Minimal toast host. Mount once near the root layout — it listens for
// `mark8ly:toast` CustomEvents and renders stacked notifications that
// auto-dismiss. Tone → colour, keyboard & aria handled.

import type { CSSProperties } from "react";
import { useEffect, useRef, useState } from "react";
import { TOAST_EVENT, type ToastInput, type ToastTone } from "@/lib/toast";

interface ToastRecord extends ToastInput {
  id: string;
}

// Each tone reads from the storefront semantic status tokens, so toast
// notifications follow the merchant's theme instead of leaking the raw
// Tailwind palette.
const TONE_STYLES: Record<ToastTone, CSSProperties> = {
  success: {
    color: "var(--storefront-success)",
    backgroundColor: "var(--storefront-success-bg)",
    borderColor: "var(--storefront-success-border)",
  },
  error: {
    color: "var(--storefront-danger)",
    backgroundColor: "var(--storefront-danger-bg)",
    borderColor: "var(--storefront-danger-border)",
  },
  warn: {
    color: "var(--storefront-warning)",
    backgroundColor: "var(--storefront-warning-bg)",
    borderColor: "var(--storefront-warning-border)",
  },
  info: {
    color: "var(--storefront-text, var(--ink-900))",
    backgroundColor: "var(--storefront-surface)",
    borderColor: "var(--storefront-neutral-border)",
  },
};

export function Toaster() {
  const [toasts, setToasts] = useState<ToastRecord[]>([]);
  // Track each active auto-dismiss timer by toast id so the effect
  // cleanup can cancel them on unmount — otherwise a rapid burst of
  // toasts fired right before navigation leaves stale timers hanging.
  const timersRef = useRef<Map<string, ReturnType<typeof setTimeout>>>(new Map());

  useEffect(() => {
    const timers = timersRef.current;
    function onToast(evt: Event) {
      const ce = evt as CustomEvent<ToastInput>;
      const record: ToastRecord = {
        id: `${Date.now()}-${Math.random().toString(36).slice(2, 8)}`,
        tone: "info",
        duration: 3500,
        ...ce.detail,
      };
      setToasts((prev) => [...prev, record]);
      const id = setTimeout(() => {
        setToasts((prev) => prev.filter((t) => t.id !== record.id));
        timers.delete(record.id);
      }, record.duration ?? 3500);
      timers.set(record.id, id);
    }
    window.addEventListener(TOAST_EVENT, onToast);
    return () => {
      window.removeEventListener(TOAST_EVENT, onToast);
      timers.forEach((id) => clearTimeout(id));
      timers.clear();
    };
  }, []);

  if (toasts.length === 0) return null;

  return (
    <div
      aria-live="polite"
      aria-atomic="true"
      className="pointer-events-none fixed inset-x-0 bottom-6 z-50 flex flex-col items-center gap-2 px-4"
    >
      {toasts.map((t) => (
        <div
          key={t.id}
          role={t.tone === "error" ? "alert" : "status"}
          className="pointer-events-auto w-full max-w-sm rounded-md border px-4 py-3 shadow-sm transition-opacity"
          style={TONE_STYLES[t.tone ?? "info"]}
        >
          <p className="text-sm font-medium">{t.title}</p>
          {t.description && (
            <p className="mt-1 text-xs opacity-80">{t.description}</p>
          )}
        </div>
      ))}
    </div>
  );
}
