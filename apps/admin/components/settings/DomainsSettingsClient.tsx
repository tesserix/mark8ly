"use client";

import { useState, useTransition } from "react";
import { useRouter } from "next/navigation";
import { formatDistanceToNow } from "date-fns";
import { Globe, Plus, RefreshCw, Trash2, Lock, Copy, Check, AlertCircle } from "lucide-react";

import type { CustomDomain, DNSMethod } from "@/lib/api/settings-tier2-api";
import {
  addDomainAction,
  removeDomainAction,
  verifyDomainAction,
  refreshDomainStatusAction,
} from "@/app/(admin)/settings/actions";
import { useToast } from "@/components/feedback/Toaster";

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
  const [method, setMethod] = useState<DNSMethod>("manual");
  const [cfToken, setCfToken] = useState("");
  const [isPending, startTransition] = useTransition();
  const [error, setError] = useState<string | null>(null);
  const router = useRouter();
  const { toast } = useToast();

  function handleSubmit(e: React.FormEvent) {
    e.preventDefault();
    setError(null);
    startTransition(async () => {
      const result = await addDomainAction(
        domain,
        method,
        method === "cloudflare" ? cfToken : undefined,
      );
      if (!result.ok) {
        setError(result.message);
        toast.error("Couldn't add domain", result.message);
      } else {
        toast.success(
          "Domain added",
          method === "manual"
            ? "Follow the CNAME instructions, then click Verify."
            : "We're provisioning via Cloudflare — this can take a minute.",
        );
        setDomain("");
        setCfToken("");
        router.refresh();
      }
    });
  }

  const canSubmit =
    domain.trim() &&
    (method === "manual" || cfToken.trim()) &&
    !isPending;

  return (
    <section className="space-y-6">
      <div className="flex items-start gap-3">
        <Plus className="mt-0.5 h-5 w-5 text-foreground-secondary" aria-hidden="true" />
        <div className="space-y-1">
          <h2 className="font-serif text-2xl font-medium tracking-tight text-foreground">
            Add a custom domain
          </h2>
          <p className="text-sm text-foreground-secondary">
            Connect your own domain to your storefront. Your customers will see your brand, not ours.
          </p>
        </div>
      </div>

      {/* Method toggle */}
      <div className="space-y-2">
        <p className="text-sm font-medium text-foreground">DNS setup method</p>
        <div className="inline-flex rounded-md border border-border bg-[color:var(--background-elevated)] p-1">
          {(["manual", "cloudflare"] as const).map((m) => (
            <button
              key={m}
              type="button"
              onClick={() => setMethod(m)}
              className={`rounded-sm px-4 py-2 text-sm font-medium transition-colors ${
                method === m
                  ? "bg-[color:var(--moss-700)] text-white"
                  : "text-foreground-secondary hover:text-foreground"
              }`}
            >
              {m === "manual" ? "Manual (CNAME)" : "Cloudflare (auto)"}
            </button>
          ))}
        </div>
        <p className="text-xs text-foreground-secondary">
          {method === "manual"
            ? "You'll add a CNAME record at your DNS provider. Works with any registrar."
            : "Automated via Cloudflare API. Requires a Cloudflare API token with DNS edit permissions."}
        </p>
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
            className="h-10 w-full rounded-md border border-border bg-[color:var(--background-elevated)] px-3 text-sm text-foreground placeholder:text-foreground-tertiary focus:border-[color:var(--moss-700)] focus:outline-none focus:ring-1 focus:ring-[color:var(--moss-700)] disabled:opacity-50"
            placeholder="shop.example.com"
          />
        </div>

        {method === "cloudflare" && (
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
              className="h-10 w-full rounded-md border border-border bg-[color:var(--background-elevated)] px-3 text-sm text-foreground placeholder:text-foreground-tertiary focus:border-[color:var(--moss-700)] focus:outline-none focus:ring-1 focus:ring-[color:var(--moss-700)] disabled:opacity-50"
              placeholder="Your Cloudflare API token"
            />
          </div>
        )}

        {error && <p role="alert" className="text-sm text-[color:var(--signal)]">{error}</p>}
        <button
          type="submit"
          disabled={!canSubmit}
          className="h-10 w-fit rounded-md bg-[color:var(--ink-900)] px-5 text-sm font-medium text-white transition-colors hover:bg-[color:var(--ink-900)]/90 disabled:opacity-50"
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
  const { toast } = useToast();

  function handleRefreshStatus(domainId: string) {
    setError(null);
    startTransition(async () => {
      const result = await refreshDomainStatusAction(domainId);
      if (!result.ok) {
        toast.error("Couldn't refresh status", result.message);
        return;
      }
      if (result.ssl_status === "active") {
        toast.success("SSL active", "Your certificate is live.");
      } else if (result.error) {
        toast.info("SSL still provisioning", result.error);
      } else {
        toast.info("SSL still provisioning", "Check back in a minute.");
      }
      router.refresh();
    });
  }

  function handleVerify(domainId: string) {
    setError(null);
    startTransition(async () => {
      const result = await verifyDomainAction(domainId);
      if (!result.ok) {
        setError(result.message);
        toast.error("Verification failed", result.message);
        return;
      }
      // Action succeeded but the resulting status tells us whether the
      // DNS check actually passed. `active` = verified; anything else
      // means DNS isn't ready yet, and `error` carries the reason.
      if (result.status === "active") {
        toast.success("Domain verified", "Your custom domain is live.");
      } else if (result.error) {
        toast.error("DNS not ready yet", result.error);
      } else {
        toast.info(
          "Still verifying",
          "DNS hasn't propagated yet. Try again in a few minutes.",
        );
      }
      router.refresh();
    });
  }

  function handleRemove(domainId: string) {
    setError(null);
    startTransition(async () => {
      const result = await removeDomainAction(domainId);
      if (!result.ok) {
        setError(result.message);
        toast.error("Couldn't remove domain", result.message);
      } else {
        toast.success("Domain removed", "The custom domain has been disconnected.");
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
          <h2 className="font-serif text-2xl font-medium tracking-tight text-foreground">
            Connected domains
          </h2>
          <p className="text-sm text-foreground-secondary">
            Domains currently linked to your storefront.
          </p>
        </div>
      </div>
      {error && <p role="alert" className="text-sm text-[color:var(--signal)]">{error}</p>}
      {domains.length === 0 ? (
        <div className="py-10 text-center">
          <p className="text-sm text-[color:var(--ink-900)]/50">
            No custom domains configured yet. Add a domain above to get started.
          </p>
        </div>
      ) : (
        <div className="space-y-4">
          {domains.map((d) => (
            <DomainCard
              key={d.id}
              domain={d}
              editable={editable}
              isPending={isPending}
              confirmRemove={confirmRemoveId === d.id}
              onVerify={() => handleVerify(d.id)}
              onRefreshStatus={() => handleRefreshStatus(d.id)}
              onRemove={() => handleRemove(d.id)}
              onConfirmRemove={() => setConfirmRemoveId(d.id)}
              onCancelRemove={() => setConfirmRemoveId(null)}
            />
          ))}
        </div>
      )}
    </section>
  );
}

function DomainCard({
  domain: d,
  editable,
  isPending,
  confirmRemove,
  onVerify,
  onRefreshStatus,
  onRemove,
  onConfirmRemove,
  onCancelRemove,
}: {
  domain: CustomDomain;
  editable: boolean;
  isPending: boolean;
  confirmRemove: boolean;
  onVerify: () => void;
  onRefreshStatus: () => void;
  onRemove: () => void;
  onConfirmRemove: () => void;
  onCancelRemove: () => void;
}) {
  return (
    <div className="rounded-md border border-border bg-[color:var(--background-elevated)] p-5">
      {/* Header row */}
      <div className="flex flex-wrap items-center justify-between gap-3">
        <div className="flex items-center gap-3">
          <p className="font-medium text-foreground">{d.domain}</p>
          <span
            className={`inline-flex items-center rounded-full px-2.5 py-0.5 text-xs font-medium ${STATUS_STYLES[d.status] ?? STATUS_STYLES.pending}`}
          >
            {d.status}
          </span>
          <span
            className={`inline-flex items-center gap-1 rounded-full px-2.5 py-0.5 text-xs font-medium ${SSL_STYLES[d.ssl_status] ?? SSL_STYLES.pending}`}
          >
            <Lock className="h-3 w-3" aria-hidden="true" />
            SSL {d.ssl_status}
          </span>
        </div>
        <p className="text-xs text-foreground-secondary">
          {d.verified_at
            ? `Verified ${formatDistanceToNow(new Date(d.verified_at), { addSuffix: true })}`
            : "Not verified"}
        </p>
      </div>

      {d.error && (
        <div className="mt-3 rounded-md border border-[color:var(--signal)]/30 bg-[color:var(--signal)]/5 p-3">
          <div className="flex items-start gap-2">
            <AlertCircle
              className="mt-0.5 h-4 w-4 shrink-0 text-[color:var(--signal)]"
              aria-hidden="true"
            />
            <div className="space-y-1">
              <p className="text-xs font-semibold text-[color:var(--signal)]">
                Verification failed
              </p>
              <p className="text-xs text-foreground">{d.error}</p>
              <p className="text-xs text-foreground-secondary">
                {errorHint(d.error)}
              </p>
            </div>
          </div>
        </div>
      )}

      {/* Manual DNS instructions — TWO CNAMEs required:
          1. Main routing CNAME (points requests to our cluster)
          2. ACME challenge CNAME (delegates SSL cert issuance to us) */}
      {d.dns_method === "manual" && d.cname_target && (
        <div className="mt-4 rounded-md border border-border bg-background p-4">
          <p className="text-xs font-semibold uppercase tracking-[0.12em] text-foreground-secondary">
            DNS setup — add both records
          </p>

          {/* Record 1: routing — two options depending on domain type */}
          <div className="mt-3 space-y-3 border-l-2 border-[color:var(--moss-700)]/40 pl-3">
            <p className="text-xs font-medium text-foreground">
              1. Route traffic to your storefront
            </p>
            <p className="text-xs text-foreground-secondary">
              Pick ONE: Option A (simpler, works for apex / subdomains) or Option B (cleaner if your DNS supports CNAME here).
            </p>

            <div className="rounded border border-border bg-[color:var(--background-elevated)] p-2.5">
              <p className="text-xs font-medium text-foreground">Option A — A record</p>
              <div className="mt-2 space-y-1.5">
                <DnsRecord label="Type" value="A" />
                <DnsRecord label="Name" value={d.domain} copyable />
                <DnsRecord label="Value" value="34.14.139.74" copyable />
              </div>
            </div>

            <div className="rounded border border-border bg-[color:var(--background-elevated)] p-2.5">
              <p className="text-xs font-medium text-foreground">Option B — CNAME</p>
              <p className="text-xs text-foreground-tertiary">
                CNAME not allowed at apex by most DNS providers — use Option A for apex domains.
              </p>
              <div className="mt-2 space-y-1.5">
                <DnsRecord label="Type" value="CNAME" />
                <DnsRecord label="Name" value={d.domain} copyable />
                <DnsRecord label="Target" value={d.cname_target} copyable />
              </div>
            </div>
          </div>

          {/* Record 2: ACME delegation */}
          <div className="mt-4 space-y-2 border-l-2 border-[color:var(--moss-700)]/40 pl-3">
            <p className="text-xs font-medium text-foreground">
              2. Delegate SSL certificate issuance
            </p>
            <DnsRecord label="Type" value="CNAME" />
            <DnsRecord label="Name" value={`_acme-challenge.${d.domain}`} copyable />
            <DnsRecord label="Target" value={`_acme-challenge.${d.domain}.acme.mark8ly.com`} copyable />
            <p className="text-xs text-foreground-tertiary">
              This lets us issue a free Let&apos;s Encrypt SSL certificate for your domain without you sharing DNS credentials.
            </p>
          </div>

          {/* Record 3 (optional): admin subdomain routing */}
          <div className="mt-4 space-y-2 border-l-2 border-[color:var(--moss-700)]/40 pl-3">
            <p className="text-xs font-medium text-foreground">
              3. Admin panel subdomain (optional)
            </p>
            <p className="text-xs text-foreground-secondary">
              Adds <code>admin.{d.domain}</code> as a branded URL for your admin dashboard. Skip if you&apos;re happy using the default admin URL.
            </p>
            <DnsRecord label="Type" value="A" />
            <DnsRecord label="Name" value={`admin.${d.domain}`} copyable />
            <DnsRecord label="Value" value="34.14.139.74" copyable />
          </div>

          <p className="mt-4 text-xs text-foreground-tertiary">
            DNS changes can take up to 48 hours to propagate. Click &quot;Verify&quot; once both records are live.
          </p>
          <p className="mt-2 text-xs text-foreground-tertiary">
            Tip: check propagation with{" "}
            <a
              href={`https://dnschecker.org/#CNAME/${encodeURIComponent(d.domain)}`}
              target="_blank"
              rel="noopener noreferrer"
              className="underline underline-offset-2"
            >
              dnschecker.org
            </a>
            .
          </p>
        </div>
      )}

      {d.dns_method === "manual" && d.status === "active" && (
        <p className="mt-3 text-xs text-[color:var(--moss-700)]">
          DNS verified. SSL certificate provisioning may take a few minutes — refresh to see status.
        </p>
      )}

      {/* Actions */}
      {editable && (
        <div className="mt-4 flex items-center gap-2 border-t border-border pt-3">
          {(d.status === "pending" || d.status === "verifying" || d.status === "error") && (
            <button
              type="button"
              onClick={onVerify}
              disabled={isPending}
              className="inline-flex items-center gap-1.5 rounded-md border border-border px-3 py-1.5 text-xs font-medium text-foreground transition-colors hover:border-[color:var(--moss-700)] hover:text-[color:var(--moss-700)] disabled:opacity-50"
            >
              <RefreshCw className="h-3 w-3" aria-hidden="true" />
              Verify
            </button>
          )}
          {d.status === "active" && d.ssl_status !== "active" && (
            <button
              type="button"
              onClick={onRefreshStatus}
              disabled={isPending}
              className="inline-flex items-center gap-1.5 rounded-md border border-border px-3 py-1.5 text-xs font-medium text-foreground transition-colors hover:border-[color:var(--moss-700)] hover:text-[color:var(--moss-700)] disabled:opacity-50"
            >
              <RefreshCw className="h-3 w-3" aria-hidden="true" />
              Refresh SSL status
            </button>
          )}
          {confirmRemove ? (
            <>
              <button
                type="button"
                onClick={onRemove}
                disabled={isPending}
                className="inline-flex items-center rounded-md bg-[color:var(--danger)] px-3 py-1.5 text-xs font-medium text-white disabled:opacity-50"
              >
                Confirm remove
              </button>
              <button
                type="button"
                onClick={onCancelRemove}
                className="px-3 py-1.5 text-xs font-medium text-foreground-secondary hover:text-foreground"
              >
                Cancel
              </button>
            </>
          ) : (
            <button
              type="button"
              onClick={onConfirmRemove}
              disabled={isPending}
              className="inline-flex items-center gap-1.5 rounded-md border border-border px-3 py-1.5 text-xs font-medium text-foreground-secondary transition-colors hover:border-[color:var(--signal)] hover:text-[color:var(--signal)] disabled:opacity-50"
            >
              <Trash2 className="h-3 w-3" aria-hidden="true" />
              Remove
            </button>
          )}
        </div>
      )}
    </div>
  );
}

/**
 * Translate a backend verification error into a merchant-friendly hint
 * on what to fix next.
 */
function errorHint(error: string): string {
  const lower = error.toLowerCase();
  if (lower.includes("cname record not found") || lower.includes("no such host")) {
    return "We couldn't find any CNAME record at your domain. Add the CNAME shown above at your DNS provider, then try Verify again.";
  }
  if (lower.includes("points to") && lower.includes("expected")) {
    return "A record exists but isn't pointing to our target. Most likely you added an A record instead of a CNAME, or the CNAME points somewhere else. Replace it with the exact CNAME shown above.";
  }
  return "Re-check the Name and Target values above, then try Verify again once your DNS provider confirms the change is live.";
}

function DnsRecord({
  label,
  value,
  copyable,
}: {
  label: string;
  value: string;
  copyable?: boolean;
}) {
  const [copied, setCopied] = useState(false);
  const copy = () => {
    navigator.clipboard.writeText(value).then(() => {
      setCopied(true);
      setTimeout(() => setCopied(false), 1500);
    });
  };

  return (
    <div className="flex items-center gap-3 text-sm">
      <span className="w-14 shrink-0 text-xs font-medium text-foreground-secondary">{label}</span>
      <code className="flex-1 rounded bg-[color:var(--ink-900)]/5 px-2 py-1 font-mono text-xs text-foreground">
        {value}
      </code>
      {copyable && (
        <button
          type="button"
          onClick={copy}
          className="shrink-0 rounded p-1 text-foreground-secondary hover:text-foreground"
          aria-label={`Copy ${label}`}
        >
          {copied ? <Check className="h-3.5 w-3.5" /> : <Copy className="h-3.5 w-3.5" />}
        </button>
      )}
    </div>
  );
}
