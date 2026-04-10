"use client";

import { useState, useTransition } from "react";
import { useRouter } from "next/navigation";
import { formatDistanceToNow } from "date-fns";
import { Globe, Plus, RefreshCw, Trash2, Lock } from "lucide-react";

import type { CustomDomain } from "@/lib/api/settings-tier2-api";
import {
  addDomainAction,
  removeDomainAction,
  verifyDomainAction,
} from "@/app/settings/actions";

interface DomainsSettingsClientProps {
  domains: CustomDomain[];
  editable: boolean;
}

const STATUS_STYLES: Record<string, string> = {
  active: "bg-[color:var(--moss-700)]/10 text-[color:var(--moss-700)]",
  error: "bg-[color:var(--signal)]/10 text-[color:var(--signal)]",
  pending: "bg-[color:var(--ink-900)]/10 text-[color:var(--ink-900)]/60",
  verifying: "bg-[color:var(--ink-900)]/10 text-[color:var(--ink-900)]/60",
  removing: "bg-[color:var(--warning)]/10 text-[color:var(--warning)]",
};

const SSL_STYLES: Record<string, string> = {
  active: "bg-[color:var(--moss-700)]/10 text-[color:var(--moss-700)]",
  error: "bg-[color:var(--signal)]/10 text-[color:var(--signal)]",
  pending: "bg-[color:var(--ink-900)]/10 text-[color:var(--ink-900)]/60",
};

export function DomainsSettingsClient({
  domains,
  editable,
}: DomainsSettingsClientProps) {
  return (
    <div className="space-y-10">
      {editable && <AddDomainForm />}
      <hr className="border-border" />
      <DomainsList domains={domains} editable={editable} />
    </div>
  );
}

// ─── Add Domain Form ──────────────────────────────────────────────────

function AddDomainForm() {
  const [domain, setDomain] = useState("");
  const [cfToken, setCfToken] = useState("");
  const [isPending, startTransition] = useTransition();
  const [error, setError] = useState<string | null>(null);
  const router = useRouter();

  function handleSubmit(e: React.FormEvent) {
    e.preventDefault();
    setError(null);
    startTransition(async () => {
      const result = await addDomainAction(domain, cfToken);
      if (!result.ok) {
        setError(result.message);
      } else {
        setDomain("");
        setCfToken("");
        router.refresh();
      }
    });
  }

  return (
    <section className="space-y-4">
      <div className="flex items-start gap-3">
        <Plus className="mt-0.5 h-5 w-5 text-foreground-secondary" aria-hidden="true" />
        <div className="space-y-1">
          <h2 className="font-[family-name:var(--font-source-serif),'Source_Serif_4',serif] text-2xl font-medium tracking-tight text-foreground">
            Add a custom domain
          </h2>
          <p className="text-sm text-foreground-secondary">
            Connect your own domain to your storefront. You will need a Cloudflare API token with DNS edit permissions.
          </p>
        </div>
      </div>
      <form onSubmit={handleSubmit} className="grid max-w-md gap-4">
        <div className="space-y-1.5">
          <label htmlFor="domain-name" className="text-sm font-medium text-foreground">
            Domain
          </label>
          <input
            id="domain-name"
            type="text"
            value={domain}
            onChange={(e) => setDomain(e.target.value)}
            disabled={isPending}
            className="h-10 w-full rounded-[6px] border border-border bg-white px-3 text-sm text-foreground placeholder:text-foreground-tertiary focus:border-[color:var(--moss-700)] focus:outline-none focus:ring-1 focus:ring-[color:var(--moss-700)] disabled:opacity-50"
            placeholder="shop.example.com"
          />
        </div>
        <div className="space-y-1.5">
          <label htmlFor="cf-token" className="text-sm font-medium text-foreground">
            Cloudflare API token
          </label>
          <input
            id="cf-token"
            type="password"
            value={cfToken}
            onChange={(e) => setCfToken(e.target.value)}
            disabled={isPending}
            className="h-10 w-full rounded-[6px] border border-border bg-white px-3 text-sm text-foreground placeholder:text-foreground-tertiary focus:border-[color:var(--moss-700)] focus:outline-none focus:ring-1 focus:ring-[color:var(--moss-700)] disabled:opacity-50"
            placeholder="Your Cloudflare API token"
          />
        </div>
        {error && <p className="text-sm text-[color:var(--signal)]">{error}</p>}
        <button
          type="submit"
          disabled={isPending || !domain.trim() || !cfToken.trim()}
          className="h-10 w-fit rounded-[6px] bg-[color:var(--ink-900)] px-5 text-sm font-medium text-white transition-colors hover:bg-[color:var(--ink-900)]/90 disabled:opacity-50"
        >
          {isPending ? "Adding..." : "Add domain"}
        </button>
      </form>
    </section>
  );
}

// ─── Domains List ─────────────────────────────────────────────────────

function DomainsList({
  domains,
  editable,
}: {
  domains: CustomDomain[];
  editable: boolean;
}) {
  const [isPending, startTransition] = useTransition();
  const [error, setError] = useState<string | null>(null);
  const [confirmRemoveId, setConfirmRemoveId] = useState<string | null>(null);
  const router = useRouter();

  function handleVerify(domainId: string) {
    setError(null);
    startTransition(async () => {
      const result = await verifyDomainAction(domainId);
      if (!result.ok) {
        setError(result.message);
      } else {
        router.refresh();
      }
    });
  }

  function handleRemove(domainId: string) {
    setError(null);
    startTransition(async () => {
      const result = await removeDomainAction(domainId);
      if (!result.ok) {
        setError(result.message);
      } else {
        setConfirmRemoveId(null);
        router.refresh();
      }
    });
  }

  return (
    <section className="space-y-6">
      <div className="flex items-start gap-3">
        <Globe className="mt-0.5 h-5 w-5 text-foreground-secondary" aria-hidden="true" />
        <div className="space-y-1">
          <h2 className="font-[family-name:var(--font-source-serif),'Source_Serif_4',serif] text-2xl font-medium tracking-tight text-foreground">
            Connected domains
          </h2>
          <p className="text-sm text-foreground-secondary">
            Domains currently linked to your storefront.
          </p>
        </div>
      </div>
      {error && <p className="text-sm text-[color:var(--signal)]">{error}</p>}
      {domains.length === 0 ? (
        <div className="rounded-[6px] bg-white px-6 py-10 text-center">
          <p className="text-sm text-[color:var(--ink-900)]/50">
            No custom domains configured yet. Add a domain above to get started.
          </p>
        </div>
      ) : (
        <div className="overflow-x-auto">
          <table className="w-full text-sm">
            <thead>
              <tr className="border-b border-border text-left">
                <th className="pb-3 pr-4 font-medium text-foreground-secondary">Domain</th>
                <th className="pb-3 pr-4 font-medium text-foreground-secondary">Status</th>
                <th className="pb-3 pr-4 font-medium text-foreground-secondary">SSL</th>
                <th className="pb-3 pr-4 font-medium text-foreground-secondary">Verified</th>
                <th className="pb-3 font-medium text-foreground-secondary" />
              </tr>
            </thead>
            <tbody>
              {domains.map((d) => (
                <tr key={d.id} className="border-b border-border">
                  <td className="py-3 pr-4 font-medium text-foreground">{d.domain}</td>
                  <td className="py-3 pr-4">
                    <span
                      className={`inline-flex items-center rounded-full px-2.5 py-0.5 text-xs font-medium ${STATUS_STYLES[d.status] ?? STATUS_STYLES.pending}`}
                    >
                      {d.status}
                    </span>
                    {d.error_message && (
                      <p className="mt-1 text-xs text-[color:var(--signal)]">{d.error_message}</p>
                    )}
                  </td>
                  <td className="py-3 pr-4">
                    <span
                      className={`inline-flex items-center gap-1 rounded-full px-2.5 py-0.5 text-xs font-medium ${SSL_STYLES[d.ssl_status] ?? SSL_STYLES.pending}`}
                    >
                      <Lock className="h-3 w-3" aria-hidden="true" />
                      {d.ssl_status}
                    </span>
                  </td>
                  <td className="py-3 pr-4 text-foreground-secondary">
                    {d.verified_at
                      ? formatDistanceToNow(new Date(d.verified_at), { addSuffix: true })
                      : "---"}
                  </td>
                  <td className="py-3 text-right">
                    {editable && (
                      <div className="flex items-center justify-end gap-2">
                        {(d.status === "pending" || d.status === "error") && (
                          <button
                            type="button"
                            onClick={() => handleVerify(d.id)}
                            disabled={isPending}
                            className="inline-flex items-center gap-1 text-xs font-medium text-[color:var(--moss-700)] hover:underline disabled:opacity-50"
                          >
                            <RefreshCw className="h-3 w-3" aria-hidden="true" />
                            Verify
                          </button>
                        )}
                        {confirmRemoveId === d.id ? (
                          <div className="flex items-center gap-2">
                            <button
                              type="button"
                              onClick={() => handleRemove(d.id)}
                              disabled={isPending}
                              className="text-xs font-medium text-[color:var(--danger)] hover:underline disabled:opacity-50"
                            >
                              Confirm
                            </button>
                            <button
                              type="button"
                              onClick={() => setConfirmRemoveId(null)}
                              className="text-xs font-medium text-foreground-secondary hover:underline"
                            >
                              Cancel
                            </button>
                          </div>
                        ) : (
                          <button
                            type="button"
                            onClick={() => setConfirmRemoveId(d.id)}
                            disabled={isPending}
                            className="inline-flex items-center gap-1 text-xs font-medium text-[color:var(--signal)] hover:underline disabled:opacity-50"
                          >
                            <Trash2 className="h-3 w-3" aria-hidden="true" />
                            Remove
                          </button>
                        )}
                      </div>
                    )}
                  </td>
                </tr>
              ))}
            </tbody>
          </table>
        </div>
      )}
    </section>
  );
}
