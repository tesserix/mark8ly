"use client";

import { useCallback } from "react";
import { OttoInbox } from "@repo/otto-widget";

import { useToast } from "@/components/feedback/Toaster";

import "@repo/otto-widget/styles/inbox.css";

interface LiveChatInboxProps {
  currentUserId: string;
}

// Client wrapper for the reusable OttoInbox. Pins the proxy base to the
// admin /api/admin/otto namespace so REST calls inherit the session headers
// the admin middleware injects, and bridges the widget's toast events
// into the admin-wide <Toaster> so the look matches everything else
// (products, orders, settings).
export function LiveChatInbox({ currentUserId }: LiveChatInboxProps) {
  const { toast } = useToast();
  const handleToast = useCallback(
    (tone: "success" | "error" | "info", title: string, description?: string) => {
      if (tone === "success") toast.success(title, description);
      else if (tone === "error") toast.error(title, description);
      else toast.info(title, description);
    },
    [toast],
  );
  return (
    <OttoInbox
      apiBaseUrl="/api/admin/otto"
      buildInboxWsUrl={buildInboxWsUrl}
      buildConversationWsUrl={buildConversationWsUrl}
      currentUserId={currentUserId}
      onToast={handleToast}
    />
  );
}

function buildInboxWsUrl(): string {
  if (typeof window === "undefined") return "";
  const proto = window.location.protocol === "https:" ? "wss:" : "ws:";
  return `${proto}//${window.location.host}/api/v1/admin/otto/ws`;
}

function buildConversationWsUrl(id: string): string {
  if (typeof window === "undefined") return "";
  const proto = window.location.protocol === "https:" ? "wss:" : "ws:";
  return `${proto}//${window.location.host}/api/v1/admin/otto/conversations/${encodeURIComponent(id)}/ws`;
}
