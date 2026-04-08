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

import type { Membership } from "@/lib/api/platform-api";
import { switchToTenant } from "@/app/pick-tenant/actions";

interface TenantSwitcherProps {
  memberships: Membership[];
  currentTenantId: string;
  label?: string;
  className?: string;
}

/**
 * Phase P — in-shell tenant switcher.
 *
 * Only rendered when the signed-in user belongs to 2+ tenants.
 * Picking a tenant fires the switchToTenant server action, which
 * re-mints the session cookie through auth-bff and reloads the app so
 * middleware and server components pick up the new tenant context.
 */
export function TenantSwitcher({
  memberships,
  currentTenantId,
  label = "Store",
  className,
}: TenantSwitcherProps) {
  const [error, setError] = useState<string | null>(null);
  const [pendingId, setPendingId] = useState<string | null>(null);
  const [isPending, startTransition] = useTransition();

  const current = memberships.find((m) => m.tenant_id === currentTenantId);

  function handleSwitch(tenantId: string) {
    if (tenantId === currentTenantId) return;
    setError(null);
    setPendingId(tenantId);
    startTransition(async () => {
      const res = await switchToTenant(tenantId);
      if (!res.ok) {
        setError(res.message);
        setPendingId(null);
        return;
      }
      // Reload so middleware re-reads the new tenant_id from the
      // cookie and every server component re-fetches its tenant
      // context. router.refresh alone isn't enough because the
      // page's session headers are captured at render time.
      window.location.reload();
    });
  }

  return (
    <div className={className}>
      <p className="mb-2 text-[11px] font-semibold uppercase tracking-[0.18em] text-sidebar-text-muted">
        {label}
      </p>
      <Select
        value={currentTenantId}
        onValueChange={handleSwitch}
        disabled={isPending}
      >
        <SelectTrigger
          id="tenant-switcher"
          data-testid="tenant-switcher"
          className="w-full rounded-[1rem] border-white/10 bg-white/6 text-sm text-sidebar-foreground shadow-none [&>svg]:text-sidebar-text-muted"
        >
          <SelectValue placeholder={current?.name ?? "Switch store"} />
        </SelectTrigger>
        <SelectContent className="rounded-2xl border-border/80 bg-[rgba(255,252,248,0.98)] shadow-[0_24px_60px_rgba(76,52,24,0.14)]">
          {memberships.map((m) => {
            const active = m.tenant_id === currentTenantId;
            const busy = isPending && pendingId === m.tenant_id;
            return (
              <SelectItem
                key={m.tenant_id}
                value={m.tenant_id}
                className="rounded-xl py-2"
              >
                <div className="flex min-w-0 w-full items-start justify-between gap-3">
                  <div className="min-w-0 flex-1">
                    <p className="truncate text-sm font-medium text-foreground">
                      {m.name}
                    </p>
                    <p className="truncate text-xs text-muted-foreground">
                      {m.slug}.mark8ly.com · {m.role}
                    </p>
                    {busy && (
                      <p className="text-xs text-muted-foreground">Switching...</p>
                    )}
                  </div>
                  {active && (
                    <Check className="mt-0.5 h-4 w-4 shrink-0 text-primary" />
                  )}
                </div>
              </SelectItem>
            );
          })}
        </SelectContent>
      </Select>
      {error && (
        <div className="mt-2 px-3 text-xs text-destructive">{error}</div>
      )}
    </div>
  );
}
