"use client";

import { useState, useTransition } from "react";
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
} from "@tesserix/web";

import type { Membership } from "@/lib/api/platform-api";
import { switchToTenant } from "@/app/pick-tenant/actions";

interface TenantSwitcherProps {
  memberships: Membership[];
  currentTenantId: string;
  label?: string;
  className?: string;
  /**
   * Compact header variant — drops the eyebrow label above the trigger
   * and uses a tighter trigger. Used in the admin topbar.
   */
  compact?: boolean;
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
  label = "Switch company",
  className,
  compact = false,
}: TenantSwitcherProps) {
  const [error, setError] = useState<string | null>(null);
  const [pendingId, setPendingId] = useState<string | null>(null);
  const [isPending, startTransition] = useTransition();

  // Resolve the current membership so the trigger can render a clean
  // single-line name instead of whatever happens to live inside the
  // active SelectItem (which includes role + checkmark chrome).
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
      {!compact && (
        <p className="mb-1.5 text-[11px] font-semibold uppercase tracking-[0.16em] text-foreground-tertiary">
          {label}
        </p>
      )}
      <Select
        value={currentTenantId}
        onValueChange={handleSwitch}
        disabled={isPending}
      >
        <SelectTrigger
          id="tenant-switcher"
          data-testid="tenant-switcher"
          aria-label={label}
          className={
            compact
              ? "h-9 min-w-[10rem] text-xs"
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
          {memberships.map((m) => {
            const busy = isPending && pendingId === m.tenant_id;
            return (
              <SelectItem
                key={m.tenant_id}
                value={m.tenant_id}
                className="py-2 pr-8"
              >
                <div className="flex min-w-0 flex-col">
                  <span className="truncate text-sm font-medium text-foreground">
                    {m.name}
                  </span>
                  <span className="truncate text-[10px] uppercase tracking-[0.14em] text-foreground-tertiary">
                    {m.role}
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
