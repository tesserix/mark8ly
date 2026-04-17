"use client";

import { OttoInbox } from "@repo/otto-widget";

import "@repo/otto-widget/styles/inbox.css";

interface LiveChatInboxProps {
  currentUserId: string;
}

// Client wrapper for the reusable OttoInbox. Pins the proxy base to the
// admin /api/admin/otto namespace so REST calls inherit the session headers
// the admin middleware injects.
export function LiveChatInbox({ currentUserId }: LiveChatInboxProps) {
  return (
    <OttoInbox
      apiBaseUrl="/api/admin/otto"
      buildInboxWsUrl={buildInboxWsUrl}
      buildConversationWsUrl={buildConversationWsUrl}
      currentUserId={currentUserId}
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
