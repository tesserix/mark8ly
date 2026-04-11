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
  onRemove?: () => Promise<void> | void;
  onTest?: () => Promise<{ success: boolean; error?: string }>;
  /** The inline configuration form rendered when expanded. */
  children?: ReactNode;
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
}: ProviderCardProps) {
  const [expanded, setExpanded] = useState(false);
  const [confirmRemove, setConfirmRemove] = useState(false);
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
    startRemoveTransition(async () => {
      await onRemove();
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
    <article className="rounded-[6px] bg-white">
      {/* Header row */}
      <div className="flex items-start justify-between gap-4 px-6 py-5">
        <div className="flex flex-col gap-1.5">
          <div className="flex items-center gap-3">
            <h3 className="font-[family-name:var(--font-source-serif),'Source_Serif_4',serif] text-lg font-medium text-[color:var(--ink-900)]">
              {formatProviderName(providerName)}
            </h3>
            {configured ? (
              <>
                <StatusPill active={isActive} />
                <ModeBadge mode={mode} />
              </>
            ) : (
              <span className="inline-flex items-center gap-1.5 rounded-full bg-[color:var(--ink-900)]/5 px-2.5 py-0.5 text-xs font-medium text-[color:var(--ink-900)]/50">
                <span
                  className="h-1.5 w-1.5 rounded-full bg-[color:var(--ink-900)]/30"
                  aria-hidden="true"
                />
                Not configured
              </span>
            )}
          </div>
          {configured && maskedKey && (
            <p className="text-sm text-[color:var(--ink-900)]/60 font-mono">
              {maskedKey}
            </p>
          )}
        </div>

        <div className="flex items-center gap-2 shrink-0">
          {onTest && configured && (
            <button
              type="button"
              onClick={handleTest}
              disabled={testing || !isActive}
              className="rounded-[6px] border border-[color:var(--ink-900)]/10 px-3 py-1.5 text-sm font-medium text-[color:var(--ink-900)] transition-colors hover:border-[color:var(--ink-900)]/25 disabled:cursor-not-allowed disabled:opacity-40 focus-visible:outline-2 focus-visible:outline-offset-2 focus-visible:outline-[color:var(--moss-700)]"
            >
              {testing ? "Testing..." : "Test connection"}
            </button>
          )}
          <button
            type="button"
            onClick={handleConfigure}
            className="rounded-[6px] bg-[color:var(--ink-900)] px-3 py-1.5 text-sm font-medium text-white transition-opacity hover:opacity-90 focus-visible:outline-2 focus-visible:outline-offset-2 focus-visible:outline-[color:var(--moss-700)]"
          >
            {configureLabel}
          </button>
          {onRemove && configured && (
            <button
              ref={confirmBtnRef}
              type="button"
              onClick={handleRemove}
              disabled={removing}
              className="rounded-[6px] border border-[color:var(--ink-900)]/10 px-3 py-1.5 text-sm font-medium text-red-700 transition-colors hover:border-red-300 hover:bg-red-50 disabled:cursor-not-allowed disabled:opacity-40 focus-visible:outline-2 focus-visible:outline-offset-2 focus-visible:outline-[color:var(--moss-700)]"
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
        {testResult && (
          <div className="px-6 pb-4">
            {testResult.success ? (
              <div
                role="status"
                className="animate-in fade-in duration-300 rounded-[6px] border border-[color:var(--moss-700)]/20 bg-[color:var(--moss-700)]/5 px-4 py-2.5 text-sm text-[color:var(--moss-700)]"
              >
                Connection successful.
              </div>
            ) : (
              <div
                role="alert"
                className="rounded-[6px] border border-red-200 bg-red-50 px-4 py-2.5 text-sm text-red-800"
              >
                Connection failed{testResult.error ? `: ${testResult.error}` : "."}
              </div>
            )}
          </div>
        )}
      </div>

      {/* Confirm remove warning */}
      {confirmRemove && !removing && (
        <div className="px-6 pb-4">
          <div
            role="alert"
            className="rounded-[6px] border border-amber-200 bg-amber-50 px-4 py-2.5 text-sm text-amber-800"
          >
            Are you sure? This will remove the {formatProviderName(providerName)}{" "}
            configuration. Click &quot;Confirm remove&quot; again to proceed, or
            configure/close to cancel.
          </div>
        </div>
      )}

      {/* Expandable form with smooth height transition */}
      <div
        className="grid transition-[grid-template-rows] duration-300 ease-in-out"
        style={{ gridTemplateRows: expanded ? "1fr" : "0fr" }}
      >
        <div className="overflow-hidden">
          {expanded && (
            <>
              <hr className="border-t border-[color:var(--ink-900)]/6 mx-6" />
              <div className="px-6 py-5">{children}</div>
            </>
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
      className={`inline-block rounded-[4px] px-2 py-0.5 text-[11px] uppercase tracking-wider ${
        isLive
          ? "bg-[color:var(--moss-700)]/10 font-bold text-[color:var(--moss-700)]"
          : "bg-amber-50 font-semibold text-amber-700"
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
  // Only the providers that appear in supported_countries seed (migration
  // 000008) are listed — everything else the admin could theoretically send
  // falls through to the title-case fallback. Keep in sync with the seed.
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
  return names[provider.toLowerCase()] ?? provider.charAt(0).toUpperCase() + provider.slice(1);
}
