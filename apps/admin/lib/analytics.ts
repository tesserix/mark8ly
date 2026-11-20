"use client";

import { useEffect } from "react";

// Key merchant actions, so analytics can show where a new seller stops (page
// views alone can't: a deleted tenant's rows are purged, so the only record of
// what they did is here). Names are snake_case and never carry personal data.
export type AdminEvent =
  | "product_create_submitted"
  | "product_create_failed"
  | "storefront_theme_saved"
  | "domain_added"
  | "payment_provider_saved";

type OpenPanelQueue = (method: string, ...args: unknown[]) => void;

function op(): OpenPanelQueue | undefined {
  return (window as unknown as { op?: OpenPanelQueue }).op;
}

/** Records an admin action. A no-op when analytics isn't configured or loaded. */
export function trackEvent(name: AdminEvent, properties?: Record<string, string | number | boolean>) {
  op()?.("track", name, properties);
}

/**
 * Ties this browser's analytics to the tenant (and current store) by ID only,
 * so sessions can be grouped per seller without sending an email or name.
 * The OpenPanel script loads after a config fetch, so this waits for it.
 */
export function useAnalyticsTenant(tenantId: string | undefined, storeId: string | undefined) {
  useEffect(() => {
    if (!tenantId) return;
    let tries = 0;
    const timer = setInterval(() => {
      const send = op();
      if (send) {
        send("identify", { profileId: tenantId });
        send("setGlobalProperties", { tenant_id: tenantId, store_id: storeId ?? "" });
      }
      if (send || ++tries >= 20) clearInterval(timer);
    }, 500);
    return () => clearInterval(timer);
  }, [tenantId, storeId]);
}
