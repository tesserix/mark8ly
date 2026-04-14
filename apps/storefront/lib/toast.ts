// Micro toast helper — dispatches a CustomEvent the <Toaster> island
// listens to. No external dependency so it works in the Next.js
// standalone build without bundling gymnastics.
//
// Usage from any client component:
//   import { toast } from "@/lib/toast";
//   toast({ title: "Added to cart", tone: "success" });

export type ToastTone = "success" | "error" | "info" | "warn";

export interface ToastInput {
  title: string;
  description?: string;
  tone?: ToastTone;
  /** Milliseconds before auto-dismiss. Defaults to 3500. */
  duration?: number;
}

export const TOAST_EVENT = "mark8ly:toast";

export function toast(t: ToastInput): void {
  if (typeof window === "undefined") return;
  window.dispatchEvent(new CustomEvent<ToastInput>(TOAST_EVENT, { detail: t }));
}
