"use client";

import { useState, useTransition } from "react";
import { useRouter } from "next/navigation";
import { RoleBadge } from "@repo/ui/role-badge";

import type { Membership } from "@/lib/api/platform-api";
import { switchToTenant } from "@/app/pick-tenant/actions";

interface PickTenantFormProps {
  tenants: Membership[];
}

/**
 * PickTenantForm — landing for users with more than one tenant.
 *
 * Each tenant the user can access is rendered as a hairline-bordered
 * row (no card chrome). Clicking fires switchToTenant which re-mints
 * the session cookie with the target tenant_id, then redirects to
 * /dashboard. The list comes pre-sorted by platform-api so we render
 * in-order without client-side sorting.
 */
export function PickTenantForm({ tenants }: PickTenantFormProps) {
  const router = useRouter();
  const [error, setError] = useState<string | null>(null);
  const [pendingId, setPendingId] = useState<string | null>(null);
  const [isPending, startTransition] = useTransition();

  function handlePick(tenantId: string) {
    setError(null);
    setPendingId(tenantId);
    startTransition(async () => {
      const result = await switchToTenant(tenantId);
      if (!result.ok) {
        setError(result.message);
        setPendingId(null);
        return;
      }
      router.push("/dashboard");
      router.refresh();
    });
  }

  return (
    <div className="w-full">
      <ul className="divide-y divide-border-subtle border-y border-border-subtle">
        {tenants.map((tenant) => {
          const busy = isPending && pendingId === tenant.tenant_id;
          return (
            <li key={tenant.tenant_id}>
              <button
                type="button"
                onClick={() => handlePick(tenant.tenant_id)}
                disabled={isPending}
                className="flex w-full items-center justify-between gap-4 py-5 text-left transition-colors hover:bg-paper-100 disabled:cursor-not-allowed disabled:opacity-60"
              >
                <div className="min-w-0">
                  <p className="font-serif text-2xl font-medium tracking-tight text-foreground">
                    {tenant.name}
                  </p>
                  <p className="mt-1 truncate text-sm text-foreground-secondary">
                    {tenant.slug}.mark8ly.com
                  </p>
                </div>
                <div className="flex flex-col items-end gap-2 text-right">
                  <RoleBadge role={tenant.role} />
                  {busy && (
                    <span className="text-xs text-moss-700">Switching…</span>
                  )}
                </div>
              </button>
            </li>
          );
        })}
      </ul>
      {error && (
        <p role="alert" className="mt-4 text-sm text-danger">
          {error}
        </p>
      )}
    </div>
  );
}
