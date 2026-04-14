"use client";

// Minimal toast host. Mount once near the root layout — it listens for
// `mark8ly:toast` CustomEvents and renders stacked notifications that
// auto-dismiss. Tone → colour, keyboard & aria handled.

import { useEffect, useState } from "react";
import { TOAST_EVENT, type ToastInput, type ToastTone } from "@/lib/toast";

interface ToastRecord extends ToastInput {
  id: string;
}

const TONE_STYLES: Record<ToastTone, string> = {
  success: "border-[color:var(--moss-700)]/30 bg-[color:var(--moss-700)]/10 text-[color:var(--moss-700)]",
  error: "border-red-300 bg-red-50 text-red-800",
  info: "border-[color:var(--ink-900)]/15 bg-[color:var(--paper-200)] text-[color:var(--ink-900)]",
  warn: "border-amber-300 bg-amber-50 text-amber-800",
};

export function Toaster() {
  const [toasts, setToasts] = useState<ToastRecord[]>([]);

  useEffect(() => {
    function onToast(evt: Event) {
      const ce = evt as CustomEvent<ToastInput>;
      const record: ToastRecord = {
        id: `${Date.now()}-${Math.random().toString(36).slice(2, 8)}`,
        tone: "info",
        duration: 3500,
        ...ce.detail,
      };
      setToasts((prev) => [...prev, record]);
      window.setTimeout(() => {
        setToasts((prev) => prev.filter((t) => t.id !== record.id));
      }, record.duration ?? 3500);
    }
    window.addEventListener(TOAST_EVENT, onToast);
    return () => window.removeEventListener(TOAST_EVENT, onToast);
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
          className={`pointer-events-auto w-full max-w-sm rounded-md border px-4 py-3 shadow-sm backdrop-blur transition-opacity ${TONE_STYLES[t.tone ?? "info"]}`}
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
