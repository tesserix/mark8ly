"use client";

import { useState, useTransition } from "react";
import { Check } from "lucide-react";
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from "@tesserix/web";

import type { Store } from "@/lib/api/platform-api";
import { switchToStore } from "@/app/(admin)/settings/stores/actions";

interface StoreSwitcherProps {
  stores: Store[];
  currentStoreId: string;
  label?: string;
  className?: string;
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
}: StoreSwitcherProps) {
  const [error, setError] = useState<string | null>(null);
  const [pendingId, setPendingId] = useState<string | null>(null);
  const [isPending, startTransition] = useTransition();

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
      <p className="mb-2 text-[11px] font-semibold uppercase tracking-[0.16em] text-foreground-tertiary">
        {label}
      </p>
      <Select
        value={currentStoreId}
        onValueChange={handleSwitch}
        disabled={isPending}
      >
        <SelectTrigger
          id="store-switcher"
          data-testid="store-switcher"
          aria-label="Switch store"
          className="w-full"
        >
          <SelectValue placeholder="Switch store" />
        </SelectTrigger>
        <SelectContent>
          {stores.map((s) => {
            const active = s.id === currentStoreId;
            const busy = isPending && pendingId === s.id;
            return (
              <SelectItem key={s.id} value={s.id} className="py-2">
                <div className="flex w-full items-start justify-between gap-3">
                  <div className="min-w-0 flex-1">
                    <p className="truncate text-sm font-medium text-foreground">
                      {s.name}
                    </p>
                    <p className="truncate text-[11px] uppercase tracking-[0.12em] text-foreground-tertiary">
                      {s.slug}.mark8ly.com · {s.currency_code}
                    </p>
                    {busy && (
                      <p className="mt-0.5 text-[11px] text-[color:var(--moss-700)]">
                        Switching…
                      </p>
                    )}
                  </div>
                  {active && (
                    <Check
                      className="mt-0.5 h-4 w-4 shrink-0 text-[color:var(--moss-700)]"
                      aria-hidden="true"
                    />
                  )}
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
