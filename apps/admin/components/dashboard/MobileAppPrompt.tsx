"use client";

import { useEffect, useState } from "react";
import { X } from "lucide-react";

import {
  AppStoreBadges,
  MOBILE_ADMIN_APP_LINKS,
} from "@repo/ui/app-store-badges";

/**
 * One-time dashboard prompt pointing merchants at the mobile admin app.
 *
 * Dismissal is permanent and scoped per tenant, so dismissing it for one
 * business does not silently dismiss it for another the same person also
 * runs. It must never reappear after dismissal — that is the whole point
 * of offering the affordance rather than a permanent banner.
 */

const STORAGE_PREFIX = "mark8ly.mobileAppPrompt.dismissed";

export function dismissalKey(tenantId: string): string {
  return `${STORAGE_PREFIX}.${tenantId}`;
}

/**
 * True when at least one store URL is configured. Without this the
 * surrounding copy would render above an empty badge list while iOS is
 * still unreleased and Android is (hypothetically) unset.
 */
function hasAnyStoreLink(): boolean {
  return (
    MOBILE_ADMIN_APP_LINKS.ios.trim().length > 0 ||
    MOBILE_ADMIN_APP_LINKS.android.trim().length > 0
  );
}

interface MobileAppPromptProps {
  tenantId: string;
}

export function MobileAppPrompt({ tenantId }: MobileAppPromptProps) {
  // `null` = not yet resolved. localStorage is unavailable during SSR, so
  // resolving in an effect keeps the server and first client render in
  // agreement instead of hydrating a mismatched tree.
  const [dismissed, setDismissed] = useState<boolean | null>(null);

  useEffect(() => {
    try {
      setDismissed(window.localStorage.getItem(dismissalKey(tenantId)) === "1");
    } catch {
      // Storage can be unavailable (private mode, blocked cookies). A
      // preference we cannot read is not an error worth surfacing to a
      // merchant — fail open and show the prompt.
      setDismissed(false);
    }
  }, [tenantId]);

  function handleDismiss() {
    setDismissed(true);
    try {
      window.localStorage.setItem(dismissalKey(tenantId), "1");
    } catch {
      // Dismissal still applies for this session; it just will not persist.
      // Breaking the dashboard over a failed preference write would be worse.
    }
  }

  if (dismissed !== false) return null;
  if (!hasAnyStoreLink()) return null;

  return (
    <section className="relative border-t border-border-subtle pt-10">
      <button
        type="button"
        onClick={handleDismiss}
        aria-label="Dismiss the mobile app suggestion"
        className="absolute right-0 top-10 inline-flex h-8 w-8 items-center justify-center rounded-md text-foreground-secondary transition-colors hover:text-foreground focus-visible:outline-2 focus-visible:outline-offset-2 focus-visible:outline-[color:var(--moss-700)]"
      >
        <X className="h-4 w-4" aria-hidden="true" />
      </button>

      <p className="eyebrow mb-3">On your phone</p>
      <h2 className="font-serif text-2xl font-medium text-foreground">
        Take the dashboard with you
      </h2>
      <p className="mt-3 max-w-prose pr-10 text-sm leading-relaxed text-foreground-secondary">
        Confirm orders and check stock from the Mark8ly Admin app, signed in
        with this same account.
      </p>
      <AppStoreBadges className="mt-6" />
    </section>
  );
}
