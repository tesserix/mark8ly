"use client";

import { useState } from "react";

export interface LinkedProvider {
  providerId: string;
  email?: string;
}

export interface LinkedProvidersPanelProps {
  providers: LinkedProvider[];
  onLinkGoogle: () => void | Promise<void>;
  onUnlink: (providerId: string) => Promise<void>;
  variant?: "admin" | "storefront";
  /** Disable all controls while the parent is fetching/refreshing providers. */
  pending?: boolean;
  /**
   * Omit the "Link Google account" action entirely (not just disable it).
   * Used by consumers where Google sign-in currently routes through an
   * identity store that is mid-migration and must not be offered yet.
   * Defaults to false so existing consumers are unaffected.
   */
  hideLinkGoogle?: boolean;
}

const PROVIDER_LABELS: Record<string, string> = {
  password: "Email & password",
  "google.com": "Google",
};

function labelFor(providerId: string): string {
  return PROVIDER_LABELS[providerId] ?? providerId;
}

export function LinkedProvidersPanel({
  providers,
  onLinkGoogle,
  onUnlink,
  variant = "admin",
  pending = false,
  hideLinkGoogle = false,
}: LinkedProvidersPanelProps) {
  const [busy, setBusy] = useState<string | null>(null);
  const [linkingGoogle, setLinkingGoogle] = useState(false);

  const isStorefront = variant === "storefront";
  const hasGoogle = providers.some((p) => p.providerId === "google.com");

  async function handleUnlink(providerId: string): Promise<void> {
    if (providers.length <= 1) return; // last-provider guard
    setBusy(providerId);
    try {
      await onUnlink(providerId);
    } finally {
      setBusy(null);
    }
  }

  async function handleLinkGoogle(): Promise<void> {
    setLinkingGoogle(true);
    try {
      await onLinkGoogle();
    } finally {
      setLinkingGoogle(false);
    }
  }

  const dividerClasses = isStorefront
    ? "divide-[color:var(--storefront-text,var(--ink-900))]/15"
    : "divide-border-subtle";
  const labelClasses = isStorefront
    ? "text-[color:var(--storefront-text,var(--ink-900))]"
    : "text-foreground";
  const subClasses = isStorefront
    ? "text-[color:var(--storefront-text,var(--ink-900))]/65"
    : "text-foreground-secondary";
  const unlinkClasses = isStorefront
    ? "text-[color:var(--storefront-text,var(--ink-900))]/80 decoration-[color:var(--storefront-text,var(--ink-900))]/30"
    : "text-foreground-secondary decoration-border-subtle";
  const linkBtnClasses = isStorefront
    ? "border-[color:var(--storefront-text,var(--ink-900))]/20 bg-[color:var(--storefront-surface)] text-[color:var(--storefront-text,var(--ink-900))] hover:border-[color:var(--storefront-text,var(--ink-900))]/40"
    : "border-border bg-background-elevated text-foreground hover:border-border-strong";

  return (
    <div className="space-y-4">
      <ul className={`divide-y ${dividerClasses}`}>
        {providers.map((p) => (
          <li
            key={p.providerId}
            className="flex items-center justify-between py-3"
          >
            <div className="min-w-0">
              <span className={`block text-sm font-medium ${labelClasses}`}>
                {labelFor(p.providerId)}
              </span>
              {p.email && (
                <span className={`block truncate text-xs ${subClasses}`}>
                  {p.email}
                </span>
              )}
            </div>
            {providers.length > 1 && (
              <button
                type="button"
                onClick={() => handleUnlink(p.providerId)}
                disabled={busy === p.providerId || pending}
                className={`shrink-0 text-xs underline underline-offset-4 ${unlinkClasses} hover:opacity-100 disabled:cursor-not-allowed disabled:opacity-50`}
              >
                {busy === p.providerId ? "Unlinking…" : "Unlink"}
              </button>
            )}
          </li>
        ))}
      </ul>

      {!hasGoogle && !hideLinkGoogle && (
        <button
          type="button"
          onClick={handleLinkGoogle}
          disabled={linkingGoogle || pending}
          className={`inline-flex h-11 w-full items-center justify-center gap-3 rounded-md border px-6 text-sm font-medium transition-colors disabled:cursor-not-allowed disabled:opacity-50 ${linkBtnClasses}`}
        >
          <svg width="18" height="18" viewBox="0 0 18 18" aria-hidden="true">
            <path
              d="M17.64 9.205c0-.638-.057-1.252-.164-1.841H9v3.481h4.844a4.14 4.14 0 0 1-1.796 2.716v2.259h2.908c1.702-1.567 2.684-3.875 2.684-6.615z"
              fill="#4285F4"
            />
            <path
              d="M9 18c2.43 0 4.467-.806 5.956-2.18l-2.908-2.259c-.806.54-1.837.86-3.048.86-2.344 0-4.328-1.584-5.036-3.711H.957v2.332A8.997 8.997 0 0 0 9 18z"
              fill="#34A853"
            />
            <path
              d="M3.964 10.71A5.41 5.41 0 0 1 3.682 9c0-.593.102-1.17.282-1.71V4.958H.957A8.996 8.996 0 0 0 0 9c0 1.452.348 2.827.957 4.042l3.007-2.332z"
              fill="#FBBC05"
            />
            <path
              d="M9 3.58c1.321 0 2.508.454 3.44 1.345l2.582-2.58C13.463.891 11.426 0 9 0A8.997 8.997 0 0 0 .957 4.958L3.964 7.29C4.672 5.163 6.656 3.58 9 3.58z"
              fill="#EA4335"
            />
          </svg>
          {linkingGoogle ? "Opening Google…" : "Link Google account"}
        </button>
      )}
    </div>
  );
}
