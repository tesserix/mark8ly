"use client";

import { useState, useTransition } from "react";
import { useRouter } from "next/navigation";

import type { Membership } from "@/lib/api/platform-api";
import { switchToTenant } from "@/app/pick-tenant/actions";

interface PickTenantFormProps {
  tenants: Membership[];
}

/**
 * Phase P — PickTenantForm.
 *
 * Renders each tenant the user has access to as a clickable card.
 * Clicking fires the switchToTenant server action (which re-mints
 * the session cookie with the target tenant_id) and redirects to
 * /dashboard on success.
 *
 * The list comes pre-sorted by platform-api (owner first, then by
 * created_at), so we render in-order without client-side sorting.
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
    <div className="admin-surface space-y-4 rounded-[2rem] p-5 sm:p-6">
      {tenants.map((tenant) => {
        const busy = isPending && pendingId === tenant.tenant_id;
        return (
          <button
            key={tenant.tenant_id}
            type="button"
            onClick={() => handlePick(tenant.tenant_id)}
            disabled={isPending}
            className="w-full rounded-[1.5rem] border border-border/80 bg-white/82 p-5 text-left shadow-[0_16px_34px_rgba(76,52,24,0.06)] transition-[transform,border-color,box-shadow] hover:-translate-y-0.5 hover:border-primary/50 hover:shadow-[0_20px_40px_rgba(76,52,24,0.1)] disabled:cursor-not-allowed disabled:opacity-60"
          >
            <div className="flex items-center justify-between gap-4">
              <div className="min-w-0">
                <p className="font-serif text-xl font-medium text-foreground">
                  {tenant.name}
                </p>
                <p className="mt-1 truncate text-sm text-muted-foreground">
                  {tenant.slug}.mark8ly.com
                </p>
              </div>
              <div className="flex flex-col items-end gap-1">
                <span className="rounded-full border border-border/70 bg-muted/50 px-2.5 py-1 text-xs font-medium uppercase tracking-wide text-muted-foreground">
                  {tenant.role}
                </span>
                {busy && (
                  <span className="text-xs text-muted-foreground">
                    Switching...
                  </span>
                )}
              </div>
            </div>
          </button>
        );
      })}
      {error && (
        <div
          role="alert"
          className="rounded-2xl border border-destructive/20 bg-destructive/5 px-4 py-3 text-sm text-destructive"
        >
          {error}
        </div>
      )}
    </div>
  );
}
