"use client";

import { OttoWidget } from "@tesserix/otto-widget";

import "@tesserix/otto-widget/styles/otto.css";

import { useCustomerAuth } from "@/components/CustomerAuthProvider";

interface OttoSupportChatProps {
  storeName?: string;
}

// Thin wrapper so the widget props (customer name/email prefill, store
// name) can be sourced from the client auth context without leaking
// client hooks into the shared package. tenantId="mark8ly" pins the
// conversation to the marketplace SLM + MCP knowledge base inside
// Otto; mark8ly's reasons are the widget defaults so no `reasons`
// prop is needed here.
export function OttoSupportChat({ storeName }: OttoSupportChatProps) {
  const auth = useCustomerAuth();
  return (
    <OttoWidget
      apiBaseUrl="/api/otto"
      buildWsUrl={(id) => buildConversationWsUrl(id)}
      productName={storeName ?? "Support"}
      tenantId="mark8ly"
      customerName={auth.displayName ?? undefined}
      customerEmail={auth.email ?? undefined}
    />
  );
}

// The WebSocket path leaves the Next.js process and goes directly to the
// otto service via the Istio gateway. Same host, different path prefix.
function buildConversationWsUrl(id: string): string {
  if (typeof window === "undefined") return "";
  const proto = window.location.protocol === "https:" ? "wss:" : "ws:";
  return `${proto}//${window.location.host}/api/v1/storefront/otto/conversations/${encodeURIComponent(id)}/ws`;
}
