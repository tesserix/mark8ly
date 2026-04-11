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
 * In-shell tenant switcher for users with 2+ memberships.
 *
 * Picking a tenant fires `switchToTenant` which re-mints the session
 * cookie through auth-bff. Because the admin tenant is derived from the
 * subdomain (`{slug}-admin.mark8ly.com`), a simple reload would have
 * middleware auto-switch back to the subdomain's tenant — so we
 * redirect to the target tenant's `{slug}-admin.{rootDomain}` subdomain
 * instead. The cookie is scoped to `.mark8ly.com` and carries over.
 */
export function TenantSwitcher({
  memberships,
  currentTenantId,
  label = "Switch store",
  className,
}: TenantSwitcherProps) {
  const [error, setError] = useState<string | null>(null);
  const [pendingId, setPendingId] = useState<string | null>(null);
  const [isPending, startTransition] = useTransition();

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

      if (typeof window !== "undefined") {
        const host = window.location.host;
        const rootDomain = host
          .replace(/^[^.]+-admin\./, "")
          .replace(/^admin\./, "");
        const isProdLike = rootDomain.includes("mark8ly.com");
        if (res.slug && isProdLike) {
          window.location.href = `https://${res.slug}-admin.${rootDomain}/dashboard`;
          return;
        }
      }
      // Local dev fallback: subdomain pattern may not apply.
      window.location.reload();
    });
  }

  return (
    <div className={className}>
      <p className="mb-2 text-[11px] font-semibold uppercase tracking-[0.16em] text-foreground-tertiary">
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
          aria-label="Switch store"
          className="w-full"
        >
          <SelectValue placeholder="Switch store" />
        </SelectTrigger>
        <SelectContent>
          {memberships.map((m) => {
            const active = m.tenant_id === currentTenantId;
            const busy = isPending && pendingId === m.tenant_id;
            return (
              <SelectItem
                key={m.tenant_id}
                value={m.tenant_id}
                className="py-2"
              >
                <div className="flex w-full items-start justify-between gap-3">
                  <div className="min-w-0 flex-1">
                    <p className="truncate text-sm font-medium text-foreground">
                      {m.name}
                    </p>
                    <p className="truncate text-[11px] uppercase tracking-[0.12em] text-foreground-tertiary">
                      {m.role}
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
