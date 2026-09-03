"use client";

import { useEffect, useState } from "react";
import {
  LinkedProvidersPanel,
  type LinkedProvider,
} from "@repo/ui/auth/linked-providers-panel";
import { isGoogleSignInOffered } from "@/lib/auth/provider";
import { resolveGoogleSignInUrl } from "@/lib/auth/google-sign-in";

interface SecurityClientProps {
  storeSlug: string;
}

export function SecurityClient({ storeSlug }: SecurityClientProps) {
  const [providers, setProviders] = useState<LinkedProvider[]>([]);
  const [error, setError] = useState<string | null>(null);
  const [loading, setLoading] = useState(true);
  const canLinkGoogle = isGoogleSignInOffered();

  useEffect(() => {
    let cancelled = false;
    (async () => {
      try {
        const res = await fetch("/api/account/providers", { cache: "no-store" });
        if (!res.ok) {
          const body = (await res.json().catch(() => ({}))) as { error?: string };
          throw new Error(body.error ?? `HTTP ${res.status}`);
        }
        const wrapper = (await res.json()) as {
          data: { providers: { provider_id: string; email?: string }[] };
        };
        if (cancelled) return;
        setProviders(
          wrapper.data.providers.map((p) => ({
            providerId: p.provider_id,
            email: p.email,
          })),
        );
      } catch (err) {
        if (cancelled) return;
        setError(err instanceof Error ? err.message : "Could not load providers.");
      } finally {
        if (!cancelled) setLoading(false);
      }
    })();
    return () => {
      cancelled = true;
    };
  }, []);

  async function handleLinkGoogle(): Promise<void> {
    if (typeof window === "undefined") return;
    const result = await resolveGoogleSignInUrl({
      storeSlug,
      intent: "link",
      dest: "/account/security",
      origin: window.location.origin,
    });
    if (!result.ok) {
      setError(result.message);
      return;
    }
    window.location.assign(result.url);
  }

  async function handleUnlink(_providerId: string): Promise<void> {
    setError(
      "Removing a sign-in method requires a verification step we haven't built yet — please contact support.",
    );
    throw new Error("unlink_unavailable");
  }

  return (
    <section className="rounded-lg border border-[color:var(--storefront-text,var(--ink-900))]/15 bg-[color:var(--storefront-surface)] p-6">
      <h2 className="font-[family-name:var(--storefront-heading-font,var(--font-source-serif))] text-xl font-medium text-[color:var(--storefront-text,var(--ink-900))]">
        Linked sign-in methods
      </h2>
      <p className="mt-1 text-sm text-[color:var(--storefront-text,var(--ink-900))] opacity-70">
        {canLinkGoogle
          ? "Add Google to sign in faster. Removing a sign-in method requires contacting support for now."
          : "Removing a sign-in method requires contacting support for now."}
      </p>

      <div className="mt-5">
        {loading ? (
          <p className="text-sm text-[color:var(--storefront-text,var(--ink-900))] opacity-60">
            Loading…
          </p>
        ) : (
          <LinkedProvidersPanel
            providers={providers}
            onLinkGoogle={handleLinkGoogle}
            onUnlink={handleUnlink}
            variant="storefront"
            canLinkGoogle={canLinkGoogle}
          />
        )}
        {error && (
          <p
            role="alert"
            className="mt-4 text-sm text-[color:var(--storefront-danger,#a3322a)]"
          >
            {error}
          </p>
        )}
      </div>
    </section>
  );
}
