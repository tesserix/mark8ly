"use client";

// ProviderCard — reusable client component for payment/shipping provider
// configuration cards. Renders provider identity, status, masked key,
// mode badge, and action buttons. Accepts a children slot for the
// inline configuration form that expands on "Configure".

import { useRef, useState, useTransition, type ReactNode } from "react";

// ─────────────────────────────────────────────────────────────────────────
// Types
// ─────────────────────────────────────────────────────────────────────────

interface ProviderCardProps {
  providerName: string;
  isActive: boolean;
  maskedKey: string;
  mode: "test" | "live" | string;
  /**
   * Whether this provider currently has a stored configuration. When false,
   * the card hides the Remove button and switches the Configure action label
   * to "Add credentials" — the empty-state picker behaviour.
   */
  configured?: boolean;
  onConfigure?: () => Promise<void> | void;
  /** When absent, the Remove button is hidden (nothing to remove). */
  /**
   * Returns the outcome so a failure can be shown. It used to return void,
   * which meant a rejected remove (both DELETEs were 401 for months) looked
   * exactly like a successful one: the card re-rendered unchanged and said
   * nothing. `void` is still accepted for callers with nothing to report.
   */
  onRemove?: () => Promise<{ ok: boolean; message?: string } | void> | void;
  onTest?: () => Promise<{ success: boolean; error?: string }>;
  /** The inline configuration form rendered when expanded. */
  children?: ReactNode;
  /**
   * Always-visible slot, rendered whether or not the card is expanded.
   *
   * For anything the merchant must see WITHOUT opening the card — chiefly
   * readiness warnings. Passing those as `children` hides them behind the
   * expand toggle, which is exactly how both the shipping (#511) and
   * payments (#522) panels shipped invisible: the card renders children
   * only under `{expanded && …}`, so a collapsed card showed nothing.
   */
  banner?: ReactNode;
}

// ─────────────────────────────────────────────────────────────────────────
// Component
// ─────────────────────────────────────────────────────────────────────────

export function ProviderCard({
  providerName,
  isActive,
  maskedKey,
  mode,
  configured = true,
  onConfigure,
  onRemove,
  onTest,
  children,
  banner,
}: ProviderCardProps) {
  const [expanded, setExpanded] = useState(false);
  const [confirmRemove, setConfirmRemove] = useState(false);
  const [removeError, setRemoveError] = useState<string | null>(null);
  const [removing, startRemoveTransition] = useTransition();
  const [testing, startTestTransition] = useTransition();
  const [testResult, setTestResult] = useState<{
    success: boolean;
    error?: string;
  } | null>(null);
  const confirmBtnRef = useRef<HTMLButtonElement>(null);

  function handleConfigure() {
    setExpanded((prev) => !prev);
    setConfirmRemove(false);
    setTestResult(null);
    void onConfigure?.();
  }

  function handleRemove() {
    if (!onRemove) return;
    if (!confirmRemove) {
      setConfirmRemove(true);
      // Focus the button after React re-renders with confirmation state
      requestAnimationFrame(() => confirmBtnRef.current?.focus());
      return;
    }
    setRemoveError(null);
    startRemoveTransition(async () => {
      const result = await onRemove();
      // A falsy `ok` is a real failure; `void` means the caller has nothing
      // to report and the refresh below tells the story.
      if (result && !result.ok) {
        setRemoveError(
          result.message ?? "Could not remove this configuration.",
        );
        return;
      }
      setConfirmRemove(false);
    });
  }

  function handleTest() {
    if (!onTest) return;
    setTestResult(null);
    startTestTransition(async () => {
      const result = await onTest();
      setTestResult(result);
    });
  }

  const configureLabel = expanded
    ? "Close"
    : configured
      ? "Edit"
      : "Add credentials";

  return (
    <article className="border-b border-border-subtle last:border-b-0">
      {/* Header row */}
      <div className="flex items-start justify-between gap-4 py-5">
        <div className="flex flex-col gap-1.5">
          <div className="flex items-center gap-3">
            <h3 className="font-serif text-lg font-medium text-foreground">
              {formatProviderName(providerName)}
            </h3>
            {configured ? (
              <>
                <StatusPill active={isActive} />
                <ModeBadge mode={mode} />
              </>
            ) : (
              <span className="inline-flex items-center gap-1.5 rounded-full bg-[color:var(--ink-900)]/5 px-2.5 py-0.5 text-xs font-medium text-foreground-tertiary">
                <span
                  className="h-1.5 w-1.5 rounded-full bg-[color:var(--ink-900)]/30"
                  aria-hidden="true"
                />
                Not configured
              </span>
            )}
          </div>
          {configured && maskedKey && (
            <p className="font-mono text-sm text-foreground-secondary">
              {maskedKey}
            </p>
          )}
        </div>

        <div className="flex shrink-0 items-center gap-2">
          {onTest && configured && (
            <button
              type="button"
              onClick={handleTest}
              disabled={testing || !isActive}
              className="rounded-md border border-[color:var(--ink-900)]/10 px-3 py-1.5 text-sm font-medium text-foreground transition-colors hover:border-[color:var(--ink-900)]/25 focus-visible:outline-2 focus-visible:outline-offset-2 focus-visible:outline-[color:var(--moss-700)] disabled:cursor-not-allowed disabled:opacity-40"
            >
              {testing ? "Testing..." : "Test connection"}
            </button>
          )}
          <button
            type="button"
            onClick={handleConfigure}
            className="rounded-md bg-[color:var(--ink-900)] px-3 py-1.5 text-sm font-medium text-[color:var(--primary-foreground)] transition-colors hover:bg-[color:var(--moss-700)] focus-visible:outline-2 focus-visible:outline-offset-2 focus-visible:outline-[color:var(--moss-700)]"
          >
            {configureLabel}
          </button>
          {onRemove && configured && (
            <button
              ref={confirmBtnRef}
              type="button"
              onClick={handleRemove}
              disabled={removing}
              className="rounded-md border border-[color:var(--ink-900)]/10 px-3 py-1.5 text-sm font-medium text-[color:var(--danger)] transition-colors hover:border-[color:var(--danger)]/30 hover:bg-[color:var(--danger)]/[0.04] focus-visible:outline-2 focus-visible:outline-offset-2 focus-visible:outline-[color:var(--moss-700)] disabled:cursor-not-allowed disabled:opacity-40"
            >
              {removing
                ? "Removing..."
                : confirmRemove
                  ? "Confirm remove"
                  : "Remove"}
            </button>
          )}
        </div>
      </div>

      {/* Test connection feedback */}
      <div aria-live="polite" aria-atomic="true">
        {removeError && (
          <div className="pb-4">
            <div
              role="alert"
              className="rounded-md border border-[color:var(--danger,#7B2D26)]/25 bg-[color:var(--danger,#7B2D26)]/[0.06] px-4 py-2.5 text-sm text-[color:var(--danger,#7B2D26)]"
            >
              {removeError}
            </div>
          </div>
        )}

        {testResult && (
          <div className="pb-4">
            {testResult.success ? (
              <div
                role="status"
                className="animate-in fade-in duration-300 rounded-md border border-[color:var(--moss-700)]/20 bg-[color:var(--moss-700)]/5 px-4 py-2.5 text-sm text-[color:var(--moss-700)]"
              >
                Connection successful.
              </div>
            ) : (
              <div
                role="alert"
                className="rounded-md border border-[color:var(--danger)]/20 bg-[color:var(--danger)]/[0.05] px-4 py-2.5 text-sm text-[color:var(--danger)]"
              >
                Connection failed
                {testResult.error ? `: ${testResult.error}` : "."}
              </div>
            )}
          </div>
        )}
      </div>

      {banner && <div className="pb-4">{banner}</div>}

      {/* Confirm remove warning */}
      {confirmRemove && !removing && (
        <div className="pb-4">
          <div
            role="alert"
            className="rounded-md border border-[color:var(--warning)]/30 bg-[color:var(--warning)]/[0.05] px-4 py-2.5 text-sm text-[color:var(--warning)]"
          >
            Remove {formatProviderName(providerName)}? You&rsquo;ll need to add
            it again to re-enable. Click &quot;Confirm remove&quot; to proceed,
            or close to cancel.
          </div>
        </div>
      )}

      {/* Expandable form with smooth height transition */}
      <div
        className="grid transition-[grid-template-rows] duration-300 ease-in-out"
        style={{ gridTemplateRows: expanded ? "1fr" : "0fr" }}
      >
        {/* px-1 below is not decoration: this wrapper needs overflow-hidden
            for the grid-rows expand animation, which clips anything drawn
            outside the box — including the focus-visible:outline-offset-2
            ring on the full-width inputs at the left and right edges. The
            inset gives the ring room to render instead of being sheared
            off. */}
        <div className="overflow-hidden">
          {expanded && (
            <div className="border-t border-border-subtle px-1 pb-5 pt-5">
              {children}
            </div>
          )}
        </div>
      </div>
    </article>
  );
}

// ─────────────────────────────────────────────────────────────────────────
// Sub-components
// ─────────────────────────────────────────────────────────────────────────

function StatusPill({ active }: { active: boolean }) {
  return (
    <span
      className={`inline-flex items-center gap-1.5 rounded-full px-2.5 py-0.5 text-xs font-medium ${
        active
          ? "bg-[color:var(--moss-700)]/10 text-[color:var(--moss-700)]"
          : "bg-[color:var(--ink-900)]/5 text-[color:var(--ink-900)]/50"
      }`}
    >
      <span
        className={`h-1.5 w-1.5 rounded-full ${
          active ? "bg-[color:var(--moss-700)]" : "bg-[color:var(--ink-900)]/30"
        }`}
        aria-hidden="true"
      />
      {active ? "Active" : "Inactive"}
    </span>
  );
}

function ModeBadge({ mode }: { mode: string }) {
  const isLive = mode === "live";
  return (
    <span
      className={`inline-block rounded-sm px-2 py-0.5 text-[11px] uppercase tracking-wider ${
        isLive
          ? "bg-[color:var(--moss-700)]/10 font-bold text-[color:var(--moss-700)]"
          : "bg-[color:var(--warning)]/10 font-semibold text-[color:var(--warning)]"
      }`}
    >
      {isLive ? "Live mode" : "Test mode"}
    </span>
  );
}

// ─────────────────────────────────────────────────────────────────────────
// Utility
// ─────────────────────────────────────────────────────────────────────────

function formatProviderName(provider: string): string {
  // Only the providers that appear in supported_countries (seeded by migration
  // 000008, extended by later ones) are listed;
  // everything else the admin could theoretically send falls through to the
  // title-case fallback. Keep in sync with the seed.
  const names: Record<string, string> = {
    // Payments
    stripe: "Stripe",
    razorpay: "Razorpay",
    paypal: "PayPal",
    // Shipping — 3 carriers cover all 15 supported countries
    delhivery: "Delhivery",
    shipengine: "ShipEngine",
    ninjavan: "Ninja Van",
    // Tax
    taxjar: "TaxJar",
  };
  return (
    names[provider.toLowerCase()] ??
    provider.charAt(0).toUpperCase() + provider.slice(1)
  );
}
