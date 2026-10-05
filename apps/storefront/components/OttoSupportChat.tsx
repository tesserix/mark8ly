"use client";

import { useEffect, useRef } from "react";

import { OttoWidget } from "@tesserix/otto-widget";

import "@tesserix/otto-widget/styles/otto.css";
// Local overrides (loaded after the widget CSS): form scrolling inside the
// fixed-height panel, and the phone-sized launcher and panel (#995).
import "./otto-widget-overrides.css";

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
  const rootRef = useRef<HTMLDivElement>(null);

  useSupportDialogKeyboard(rootRef);

  return (
    <div ref={rootRef} data-support-chat="">
      <OttoWidget
        apiBaseUrl="/api/otto"
        buildWsUrl={(id) => buildConversationWsUrl(id)}
        productName={storeName ?? "Support"}
        tenantId="mark8ly"
        customerId={auth.email ?? undefined}
        customerName={auth.displayName ?? undefined}
        customerEmail={auth.email ?? undefined}
      />
    </div>
  );
}

// @tesserix/otto-widget 0.6.0 swaps the launcher for the panel and back by
// unmounting one and mounting the other, and listens for no keys. So on a
// keyboard: opening drops focus on <body>, Escape does nothing, and closing
// drops focus on <body> again. The widget exposes no hooks for any of this,
// so the host adds it from outside: Escape clicks the panel's own close
// button, and a MutationObserver moves focus into the panel when it
// appears and back to the launcher when it goes.
function useSupportDialogKeyboard(
  rootRef: React.RefObject<HTMLDivElement | null>,
) {
  useEffect(() => {
    const root = rootRef.current;
    if (!root) return;

    let focusPanelOnOpen = false;
    let focusLauncherOnClose = false;

    const onKeyDown = (event: KeyboardEvent) => {
      if (event.key !== "Escape") return;
      const close = root.querySelector<HTMLButtonElement>(
        ".otto-widget__close",
      );
      if (!close) return;
      event.stopPropagation();
      focusLauncherOnClose = true;
      close.click();
    };

    // Capture phase so the flag is set before the widget's own handler
    // unmounts the element that was clicked.
    const onClick = (event: MouseEvent) => {
      const target = event.target as Element | null;
      if (target?.closest(".otto-widget__close")) focusLauncherOnClose = true;
      if (target?.closest(".otto-widget__launcher")) focusPanelOnOpen = true;
    };

    const observer = new MutationObserver(() => {
      if (focusPanelOnOpen) {
        const panel = root.querySelector<HTMLElement>(".otto-widget__panel");
        if (panel) {
          focusPanelOnOpen = false;
          const first = panel.querySelector<HTMLElement>(
            "input, textarea, select, button",
          );
          first?.focus();
        }
      }
      if (focusLauncherOnClose) {
        const launcher = root.querySelector<HTMLButtonElement>(
          ".otto-widget__launcher",
        );
        if (launcher) {
          focusLauncherOnClose = false;
          launcher.focus();
        }
      }
    });

    root.addEventListener("keydown", onKeyDown);
    root.addEventListener("click", onClick, true);
    observer.observe(root, { childList: true, subtree: true });

    return () => {
      root.removeEventListener("keydown", onKeyDown);
      root.removeEventListener("click", onClick, true);
      observer.disconnect();
    };
  }, [rootRef]);
}

// The WebSocket path leaves the Next.js process and goes directly to the
// otto service via the Istio gateway. Same host, different path prefix.
function buildConversationWsUrl(id: string): string {
  if (typeof window === "undefined") return "";
  const proto = window.location.protocol === "https:" ? "wss:" : "ws:";
  return `${proto}//${window.location.host}/api/v1/storefront/otto/conversations/${encodeURIComponent(id)}/ws`;
}
