"use client";

// General settings form — Phase N.
//
// Surfaces every field the merchant filled in during onboarding:
//   - name           → editable
//   - slug           → read-only (URL stability)
//   - owner_email    → read-only (auth-coupled)
//   - country_code   → read-only (tax implications)
//   - currency_code  → read-only (invoicing implications)
//   - timezone       → read-only (searchable picker deferred)
//
// Read-only fields render as disabled inputs with a small "contact
// support" hint line. No Zod/RHF here — the form has one editable
// field and Next's useTransition keeps it simple. We upgrade to RHF
// when the second editable field lands.

import { useState, useTransition } from "react";
import { useRouter } from "next/navigation";
import {
  Input,
  Label,
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from "@tesserix/web";

import { updateGeneralSettings } from "@/app/(admin)/settings/general/actions";
import type { Store, Tenant, Timezone } from "@/lib/api/platform-api";
import { useToast } from "@/components/feedback/Toaster";

// Humanise ISO codes. Falls through to the raw code when Intl doesn't
// know the region or currency — better to show "XX" than the empty
// string.
function formatCountryLabel(code: string): string {
  if (!code) return "";
  try {
    const display = new Intl.DisplayNames(["en"], { type: "region" }).of(
      code.toUpperCase(),
    );
    return display ?? code.toUpperCase();
  } catch {
    return code.toUpperCase();
  }
}

function formatCurrencyLabel(code: string): string {
  if (!code) return "";
  try {
    const display = new Intl.DisplayNames(["en"], { type: "currency" }).of(
      code.toUpperCase(),
    );
    if (display && display.toUpperCase() !== code.toUpperCase()) {
      return `${display} (${code.toUpperCase()})`;
    }
  } catch {
    // fall through
  }
  return code.toUpperCase();
}

interface GeneralSettingsFormProps {
  tenant: Tenant;
  // Phase Q: the general settings form now edits the CURRENT store,
  // not the tenant. Tenant is passed through for owner_email (auth-
  // coupled, lives on tenant) while name/slug/country/currency/
  // timezone all come from store.
  store: Store;
  editable?: boolean;
  timezones: Timezone[];
}

export function GeneralSettingsForm({
  tenant,
  store,
  editable = true,
  timezones,
}: GeneralSettingsFormProps) {
  const router = useRouter();
  const { toast } = useToast();

  const [name, setName] = useState(store.name);
  const [timezone, setTimezone] = useState(store.timezone);
  const [error, setError] = useState<string | null>(null);
  const [pending, startTransition] = useTransition();

  const nameDirty = name.trim() !== store.name && name.trim().length > 0;
  const tzDirty = timezone !== store.timezone && timezone.length > 0;
  const dirty = nameDirty || tzDirty;

  function handleSubmit(e: React.FormEvent) {
    e.preventDefault();
    setError(null);

    if (!dirty) return;

    startTransition(async () => {
      const result = await updateGeneralSettings({
        name: name.trim(),
        timezone: tzDirty ? timezone : undefined,
      });
      if (!result.ok) {
        setError(result.message);
        toast.error("Couldn't save changes", result.message);
        return;
      }
      toast.success("Saved", "Store settings updated.");
      // Refresh so the server component (AdminShell) re-reads the
      // updated tenant name on the next render.
      router.refresh();
    });
  }

  return (
    <form onSubmit={handleSubmit} className="space-y-6">
      <Section title="Store">
        <Field label="Store name" htmlFor="name">
          <Input
            id="name"
            value={name}
            onChange={(e) => setName(e.target.value)}
            maxLength={200}
            disabled={pending || !editable}
            required
          />
        </Field>

        <ReadOnlyField
          label="Store URL slug"
          value={`${store.slug}.mark8ly.com`}
          hint="Changing your slug would break existing links. Contact support if you need a new one."
        />
      </Section>

      <Section title="Owner">
        <ReadOnlyField
          label="Owner email"
          value={tenant.owner_email}
          hint="Your sign-in email. Contact support to transfer ownership."
        />
      </Section>

      <Section title="Region &amp; currency">
        <div className="grid gap-6 sm:grid-cols-2">
          <ReadOnlyField
            label="Country"
            value={formatCountryLabel(store.country_code)}
          />
          <ReadOnlyField
            label="Currency"
            value={formatCurrencyLabel(store.currency_code)}
          />
        </div>
        <div className="space-y-2">
          <Label htmlFor="timezone">Timezone</Label>
          <Select
            value={timezone}
            onValueChange={setTimezone}
            disabled={pending || !editable}
          >
            <SelectTrigger id="timezone">
              <SelectValue placeholder="Select timezone…" />
            </SelectTrigger>
            <SelectContent>
              {timezones.map((tz) => (
                <SelectItem key={tz.id} value={tz.id}>
                  {tz.id}
                  {tz.utc_offset ? ` (UTC${tz.utc_offset})` : ""}
                </SelectItem>
              ))}
            </SelectContent>
          </Select>
          <p className="text-xs text-muted-foreground">
            Used for daily rollups, scheduled campaigns, and order timestamps.
          </p>
        </div>
      </Section>

      {error && (
        <div
          role="alert"
          className="rounded-lg border border-destructive/20 bg-destructive/5 px-4 py-3 text-sm text-destructive"
        >
          {error}
        </div>
      )}

      {editable && (
        <div className="flex items-center justify-end gap-3 border-t admin-soft-rule pt-6">
          <button
            type="button"
            onClick={() => {
              setName(store.name);
              setTimezone(store.timezone);
              setError(null);
            }}
            disabled={!dirty || pending}
            className="rounded-md px-4 py-2 text-sm font-medium text-foreground-secondary transition-colors hover:text-foreground focus-visible:outline-2 focus-visible:outline-offset-2 focus-visible:outline-[color:var(--moss-700)] disabled:cursor-not-allowed disabled:opacity-50"
          >
            Reset
          </button>
          <button
            type="submit"
            disabled={!dirty || pending}
            className="inline-flex items-center justify-center rounded-md bg-[color:var(--ink-900)] px-5 py-2 text-sm font-medium text-[color:var(--primary-foreground)] transition-colors hover:bg-[color:var(--moss-700)] focus-visible:outline-2 focus-visible:outline-offset-2 focus-visible:outline-[color:var(--moss-700)] disabled:cursor-not-allowed disabled:opacity-50"
          >
            {pending ? "Saving..." : "Save changes"}
          </button>
        </div>
      )}
    </form>
  );
}

interface SectionProps {
  title: string;
  children: React.ReactNode;
}

function Section({ title, children }: SectionProps) {
  return (
    <section className="admin-panel space-y-5 rounded-md p-6">
      <h2 className="text-sm font-semibold uppercase tracking-[0.16em] text-muted-foreground">
        {title}
      </h2>
      <div className="space-y-4">{children}</div>
    </section>
  );
}

interface FieldProps {
  label: string;
  htmlFor: string;
  children: React.ReactNode;
}

function Field({ label, htmlFor, children }: FieldProps) {
  return (
    <div className="space-y-2">
      <Label htmlFor={htmlFor}>{label}</Label>
      {children}
    </div>
  );
}

interface ReadOnlyFieldProps {
  label: string;
  value: string;
  hint?: string;
}

function ReadOnlyField({ label, value, hint }: ReadOnlyFieldProps) {
  // Derive a stable id from the label so getByLabel() in e2e tests can
  // find the associated input. Lowercase, spaces → dashes.
  const id = `ro-${label.toLowerCase().replace(/[^a-z0-9]+/g, "-")}`;
  return (
    <div className="space-y-2">
      <Label htmlFor={id}>{label}</Label>
      <Input
        id={id}
        value={value}
        readOnly
        disabled
        className="bg-muted/50"
      />
      {hint && <p className="text-xs text-muted-foreground">{hint}</p>}
    </div>
  );
}
