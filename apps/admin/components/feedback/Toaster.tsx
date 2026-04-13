"use client";

// apps/admin/components/feedback/Toaster.tsx
//
// Lightweight in-house toaster — no external deps. Mounted once at the
// admin route-group root via `<Toaster />` and consumed via the
// `useToast()` hook.
//
//   const { toast } = useToast();
//   toast.success("Saved");
//   toast.error("Couldn't save", "Try again in a moment.");
//   toast.info("Heads up");
//
// Toasts render in the bottom-right, stack vertically, auto-dismiss at
// 4s (errors stick longer at 6s). Each toast can be dismissed manually.

import {
  createContext,
  useCallback,
  useContext,
  useEffect,
  useMemo,
  useRef,
  useState,
  type ReactNode,
} from "react";
import { CheckCircle2, AlertTriangle, Info, X } from "lucide-react";

type ToastVariant = "success" | "error" | "info";

interface ToastItem {
  id: string;
  variant: ToastVariant;
  title: string;
  description?: string;
  durationMs: number;
}

interface ToastApi {
  success: (title: string, description?: string) => void;
  error: (title: string, description?: string) => void;
  info: (title: string, description?: string) => void;
  dismiss: (id: string) => void;
}

interface ToastContextValue {
  toast: ToastApi;
}

const ToastContext = createContext<ToastContextValue | null>(null);

const DEFAULT_DURATIONS: Record<ToastVariant, number> = {
  success: 4000,
  info: 4000,
  error: 6000,
};

export function Toaster({ children }: { children: ReactNode }) {
  const [items, setItems] = useState<ToastItem[]>([]);
  const timersRef = useRef<Map<string, ReturnType<typeof setTimeout>>>(new Map());

  const dismiss = useCallback((id: string) => {
    setItems((prev) => prev.filter((t) => t.id !== id));
    const timer = timersRef.current.get(id);
    if (timer) {
      clearTimeout(timer);
      timersRef.current.delete(id);
    }
  }, []);

  const push = useCallback(
    (variant: ToastVariant, title: string, description?: string) => {
      const id =
        typeof crypto !== "undefined" && "randomUUID" in crypto
          ? crypto.randomUUID()
          : `t-${Date.now()}-${Math.random().toString(36).slice(2, 8)}`;
      const durationMs = DEFAULT_DURATIONS[variant];
      setItems((prev) => [...prev, { id, variant, title, description, durationMs }]);
      const timer = setTimeout(() => dismiss(id), durationMs);
      timersRef.current.set(id, timer);
    },
    [dismiss],
  );

  const api = useMemo<ToastApi>(
    () => ({
      success: (title, description) => push("success", title, description),
      error: (title, description) => push("error", title, description),
      info: (title, description) => push("info", title, description),
      dismiss,
    }),
    [push, dismiss],
  );

  useEffect(() => {
    const timers = timersRef.current;
    return () => {
      for (const timer of timers.values()) clearTimeout(timer);
      timers.clear();
    };
  }, []);

  return (
    <ToastContext.Provider value={{ toast: api }}>
      {children}
      <ToastViewport items={items} onDismiss={dismiss} />
    </ToastContext.Provider>
  );
}

export function useToast(): ToastContextValue {
  const ctx = useContext(ToastContext);
  if (!ctx) {
    // Dev-time helper — caller forgot to wrap in <Toaster>. Return a
    // no-op so we never crash in production if a path mounts outside
    // the admin shell.
    return {
      toast: {
        success: () => {},
        error: () => {},
        info: () => {},
        dismiss: () => {},
      },
    };
  }
  return ctx;
}

function ToastViewport({
  items,
  onDismiss,
}: {
  items: ToastItem[];
  onDismiss: (id: string) => void;
}) {
  if (items.length === 0) return null;
  return (
    <div
      aria-live="polite"
      aria-atomic="false"
      className="pointer-events-none fixed bottom-4 right-4 z-[100] flex w-full max-w-sm flex-col gap-2 sm:bottom-6 sm:right-6"
    >
      {items.map((t) => (
        <ToastCard key={t.id} item={t} onDismiss={onDismiss} />
      ))}
    </div>
  );
}

function ToastCard({
  item,
  onDismiss,
}: {
  item: ToastItem;
  onDismiss: (id: string) => void;
}) {
  const Icon =
    item.variant === "success"
      ? CheckCircle2
      : item.variant === "error"
        ? AlertTriangle
        : Info;

  const accent =
    item.variant === "success"
      ? "border-l-[3px] border-l-[color:var(--moss-700,#3a5a40)]"
      : item.variant === "error"
        ? "border-l-[3px] border-l-[color:var(--signal,#C23B22)]"
        : "border-l-[3px] border-l-[color:var(--ink-900,#1f1f1f)]";

  const iconColor =
    item.variant === "success"
      ? "text-[color:var(--moss-700,#3a5a40)]"
      : item.variant === "error"
        ? "text-[color:var(--signal,#C23B22)]"
        : "text-[color:var(--ink-900,#1f1f1f)] opacity-70";

  return (
    <div
      role={item.variant === "error" ? "alert" : "status"}
      className={[
        "pointer-events-auto flex items-start gap-3 rounded-md bg-[color:var(--background-elevated,white)] px-4 py-3 shadow-[0_8px_24px_rgba(0,0,0,0.10),0_2px_6px_rgba(0,0,0,0.06)] ring-1 ring-[color:var(--ink-900,#1f1f1f)]/5",
        "animate-[toastIn_180ms_ease-out]",
        accent,
      ].join(" ")}
    >
      <Icon className={`mt-0.5 h-4 w-4 shrink-0 ${iconColor}`} aria-hidden="true" />
      <div className="min-w-0 flex-1">
        <p className="text-sm font-medium text-[color:var(--ink-900,#1f1f1f)]">
          {item.title}
        </p>
        {item.description && (
          <p className="mt-0.5 text-xs text-[color:var(--ink-900,#1f1f1f)] opacity-70">
            {item.description}
          </p>
        )}
      </div>
      <button
        type="button"
        onClick={() => onDismiss(item.id)}
        aria-label="Dismiss notification"
        className="-mr-1 inline-flex h-6 w-6 shrink-0 items-center justify-center rounded text-[color:var(--ink-900,#1f1f1f)] opacity-50 transition-opacity hover:opacity-90 focus-visible:outline-2 focus-visible:outline-offset-2 focus-visible:outline-[color:var(--moss-700,#3a5a40)]"
      >
        <X className="h-3.5 w-3.5" aria-hidden="true" />
      </button>
      <style jsx>{`
        @keyframes toastIn {
          from { opacity: 0; transform: translateY(8px); }
          to   { opacity: 1; transform: translateY(0); }
        }
      `}</style>
    </div>
  );
}
