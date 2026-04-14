"use client";

import { useState, useTransition } from "react";
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
} from "@tesserix/web";

import type { Store } from "@/lib/api/platform-api";
import { switchToStore } from "@/app/(admin)/settings/stores/actions";

interface StoreSwitcherProps {
  stores: Store[];
  currentStoreId: string;
  label?: string;
  className?: string;
  /**
   * Compact header variant — drops the eyebrow label above the trigger
   * and uses a tighter trigger. Used in the admin topbar.
   */
  compact?: boolean;
}

/**
 * In-shell store switcher for users with 2+ stores under the CURRENT tenant.
 *
 * Mirrors TenantSwitcher but for intra-tenant stores. Switching store
 * re-mints the session cookie (new store_id) and redirects to the target
 * store's `{slug}-admin.{rootDomain}` subdomain so middleware resolves
 * the new store context cleanly. Falls back to a reload on local dev
 * where the subdomain pattern may not apply.
 */
export function StoreSwitcher({
  stores,
  currentStoreId,
  label = "Switch store",
  className,
  compact = false,
}: StoreSwitcherProps) {
  const [error, setError] = useState<string | null>(null);
  const [pendingId, setPendingId] = useState<string | null>(null);
  const [isPending, startTransition] = useTransition();

  // Resolve the current store so the trigger renders only the clean
  // name (no slug / currency chrome leaking up from the SelectItem).
  const current = stores.find((s) => s.id === currentStoreId);

  function handleSwitch(storeId: string) {
    if (storeId === currentStoreId) return;
    setError(null);
    setPendingId(storeId);
    startTransition(async () => {
      const res = await switchToStore(storeId);
      if (!res.ok) {
        setError(res.message);
        setPendingId(null);
        return;
      }

      if (typeof window !== "undefined") {
        const host = window.location.host;
        const rootDomain = host
          .replace(/^[^.]+-admin\./, "")
          .replace(/^admin\./, "");
        const isProdLike = rootDomain.includes("mark8ly.com");
        const target = stores.find((s) => s.id === storeId);
        if (target?.slug && isProdLike) {
          window.location.href = `https://${target.slug}-admin.${rootDomain}/dashboard`;
          return;
        }
      }
      window.location.reload();
    });
  }

  return (
    <div className={className}>
      {!compact && (
        <p className="mb-1.5 text-[11px] font-semibold uppercase tracking-[0.16em] text-foreground-tertiary">
          {label}
        </p>
      )}
      <Select
        value={currentStoreId}
        onValueChange={handleSwitch}
        disabled={isPending}
      >
        <SelectTrigger
          id="store-switcher"
          data-testid="store-switcher"
          aria-label={label}
          className={
            compact
              ? "h-9 min-w-[9rem] max-w-[14rem] text-sm"
              : "h-10 w-full justify-between font-medium"
          }
        >
          <span className="truncate text-left text-sm text-foreground">
            {current?.name ?? label}
          </span>
        </SelectTrigger>
        <SelectContent
          align="start"
          className="min-w-[var(--radix-select-trigger-width)]"
        >
          {stores.map((s) => {
            const busy = isPending && pendingId === s.id;
            return (
              <SelectItem key={s.id} value={s.id} className="py-2 pr-8">
                <div className="flex min-w-0 flex-col">
                  <span className="truncate text-sm font-medium text-foreground">
                    {s.name}
                  </span>
                  <span className="truncate text-[10px] uppercase tracking-[0.14em] text-foreground-tertiary">
                    {s.slug}.mark8ly.com · {s.currency_code}
                    {busy && (
                      <span className="ml-2 normal-case tracking-normal text-[color:var(--moss-700)]">
                        Switching…
                      </span>
                    )}
                  </span>
                </div>
              </SelectItem>
            );
          })}
        </SelectContent>
      </Select>
      {error && (
        <p
          role="alert"
          className="mt-2 text-xs text-[color:var(--danger,#8e1a1a)]"
        >
          {error}
        </p>
      )}
    </div>
  );
}
