"use client";

import { useState, useTransition } from "react";
import { useRouter } from "next/navigation";
import { Input, Label } from "@tesserix/web";

import type { Store } from "@/lib/api/platform-api";
import {
  createNewStore,
  switchToStore,
} from "@/app/settings/stores/actions";

interface StoresListProps {
  stores: Store[];
  currentStoreId: string;
  canManage: boolean;
}

/**
 * Phase Q.2 — client component for /settings/stores.
 *
 * Two sections:
 *   - List of stores with a "Switch" button on each non-current row
 *   - Inline "Add store" form for owner/admin roles
 *
 * The add form is intentionally minimal: slug + name + country +
 * currency + timezone text inputs, no dropdowns. A searchable
 * picker for ~400 timezones and the currency list is a future
 * slice; for Phase Q.2 the merchant types the ISO codes directly
 * (US / USD / America/New_York). Onboarding uses richer pickers
 * because it's the single most important entry point, but adding
 * a second store is a rarer operation and a thin form is fine.
 */
export function StoresList({
  stores,
  currentStoreId,
  canManage,
}: StoresListProps) {
  const router = useRouter();
  const [pending, startTransition] = useTransition();
  const [error, setError] = useState<string | null>(null);
  const [showAdd, setShowAdd] = useState(false);

  const [slug, setSlug] = useState("");
  const [name, setName] = useState("");
  const [countryCode, setCountryCode] = useState("US");
  const [currencyCode, setCurrencyCode] = useState("USD");
  const [timezone, setTimezone] = useState("America/New_York");

  function handleSwitch(storeId: string) {
    setError(null);
    startTransition(async () => {
      const res = await switchToStore(storeId);
      if (!res.ok) {
        setError(res.message);
        return;
      }
      // Full reload so middleware re-reads x-session-store-id from
      // the new cookie and every server component re-fetches.
      window.location.reload();
    });
  }

  function handleCreate(e: React.FormEvent) {
    e.preventDefault();
    setError(null);
    startTransition(async () => {
      const res = await createNewStore({
        slug: slug.trim(),
        name: name.trim(),
        country_code: countryCode.trim().toUpperCase(),
        currency_code: currencyCode.trim().toUpperCase(),
        timezone: timezone.trim(),
      });
      if (!res.ok) {
        setError(res.message);
        return;
      }
      // Successfully created — immediately switch to the new store.
      const switchRes = await switchToStore(res.store.id);
      if (!switchRes.ok) {
        setError(switchRes.message);
        return;
      }
      window.location.href = "/settings/general";
    });
  }

  return (
    <div className="space-y-6">
      <section className="admin-panel space-y-4 rounded-[1.6rem] p-6">
        <div className="flex items-center justify-between">
          <h2 className="text-sm font-semibold uppercase tracking-[0.16em] text-muted-foreground">
            Your stores
          </h2>
          {canManage && !showAdd && (
            <button
              type="button"
              onClick={() => setShowAdd(true)}
              className="inline-flex items-center justify-center rounded-xl border border-border bg-white/70 px-3 py-1.5 text-xs font-medium text-foreground hover:bg-white"
            >
              + Add store
            </button>
          )}
        </div>

        {stores.length === 0 ? (
          <p className="text-sm text-muted-foreground">
            You don&apos;t have any stores yet.
          </p>
        ) : (
          <ul className="divide-y divide-border/60">
            {stores.map((s) => {
              const isCurrent = s.id === currentStoreId;
              return (
                <li
                  key={s.id}
                  data-testid="store-row"
                  className="flex items-center justify-between gap-4 py-4"
                >
                  <div className="min-w-0">
                    <p className="truncate text-sm font-medium text-foreground">
                      {s.name}
                    </p>
                    <p className="truncate text-xs text-muted-foreground">
                      {s.slug}.mark8ly.com · {s.currency_code}
                    </p>
                  </div>
                  {isCurrent ? (
                    <span className="rounded-full border border-emerald-200 bg-emerald-50 px-2.5 py-1 text-xs font-medium text-emerald-800">
                      Current
                    </span>
                  ) : (
                    <button
                      type="button"
                      onClick={() => handleSwitch(s.id)}
                      disabled={pending}
                      className="rounded-xl border border-border px-3 py-1.5 text-xs font-medium text-muted-foreground transition-colors hover:border-foreground hover:text-foreground disabled:cursor-not-allowed disabled:opacity-50"
                    >
                      Switch
                    </button>
                  )}
                </li>
              );
            })}
          </ul>
        )}
      </section>

      {canManage && showAdd && (
        <section className="admin-panel space-y-4 rounded-[1.6rem] p-6">
          <div className="flex items-center justify-between">
            <h2 className="text-sm font-semibold uppercase tracking-[0.16em] text-muted-foreground">
              Add a store
            </h2>
            <button
              type="button"
              onClick={() => setShowAdd(false)}
              disabled={pending}
              className="text-xs text-muted-foreground hover:text-foreground disabled:cursor-not-allowed disabled:opacity-50"
            >
              Cancel
            </button>
          </div>
          <form onSubmit={handleCreate} className="space-y-4">
            <div className="grid gap-4 sm:grid-cols-2">
              <div className="space-y-2">
                <Label htmlFor="new-store-name">Store name</Label>
                <Input
                  id="new-store-name"
                  value={name}
                  onChange={(e) => setName(e.target.value)}
                  placeholder="Acme Downtown"
                  disabled={pending}
                  required
                />
              </div>
              <div className="space-y-2">
                <Label htmlFor="new-store-slug">URL slug</Label>
                <Input
                  id="new-store-slug"
                  value={slug}
                  onChange={(e) => setSlug(e.target.value)}
                  placeholder="acme-downtown"
                  disabled={pending}
                  required
                />
              </div>
              <div className="space-y-2">
                <Label htmlFor="new-store-country">Country (ISO-2)</Label>
                <Input
                  id="new-store-country"
                  value={countryCode}
                  onChange={(e) => setCountryCode(e.target.value)}
                  maxLength={2}
                  disabled={pending}
                  required
                />
              </div>
              <div className="space-y-2">
                <Label htmlFor="new-store-currency">Currency (ISO-3)</Label>
                <Input
                  id="new-store-currency"
                  value={currencyCode}
                  onChange={(e) => setCurrencyCode(e.target.value)}
                  maxLength={3}
                  disabled={pending}
                  required
                />
              </div>
              <div className="space-y-2 sm:col-span-2">
                <Label htmlFor="new-store-timezone">Timezone</Label>
                <Input
                  id="new-store-timezone"
                  value={timezone}
                  onChange={(e) => setTimezone(e.target.value)}
                  placeholder="America/New_York"
                  disabled={pending}
                  required
                />
              </div>
            </div>

            {error && (
              <div
                role="alert"
                className="rounded-lg border border-destructive/20 bg-destructive/5 px-4 py-3 text-sm text-destructive"
              >
                {error}
              </div>
            )}

            <div className="flex items-center justify-end gap-3">
              <button
                type="submit"
                disabled={pending}
                className="inline-flex items-center justify-center rounded-xl bg-primary px-5 py-2.5 text-sm font-medium text-primary-foreground shadow-[0_14px_30px_rgba(31,30,28,0.18)] transition hover:opacity-95 disabled:cursor-not-allowed disabled:opacity-50"
              >
                {pending ? "Creating..." : "Create store"}
              </button>
            </div>
          </form>
        </section>
      )}

      {error && !showAdd && (
        <div
          role="alert"
          className="rounded-lg border border-destructive/20 bg-destructive/5 px-4 py-3 text-sm text-destructive"
        >
          {error}
        </div>
      )}
    </div>
  );
}
