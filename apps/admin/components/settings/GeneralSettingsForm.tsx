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
import { Input, Label } from "@tesserix/web";

import { updateGeneralSettings } from "@/app/settings/general/actions";
import type { Tenant } from "@/lib/api/platform-api";

interface GeneralSettingsFormProps {
  tenant: Tenant;
}

export function GeneralSettingsForm({ tenant }: GeneralSettingsFormProps) {
  const router = useRouter();

  const [name, setName] = useState(tenant.name);
  const [error, setError] = useState<string | null>(null);
  const [success, setSuccess] = useState(false);
  const [pending, startTransition] = useTransition();

  const dirty = name.trim() !== tenant.name && name.trim().length > 0;

  function handleSubmit(e: React.FormEvent) {
    e.preventDefault();
    setError(null);
    setSuccess(false);

    if (!dirty) return;

    startTransition(async () => {
      const result = await updateGeneralSettings({ name });
      if (!result.ok) {
        setError(result.message);
        return;
      }
      setSuccess(true);
      // Refresh so the server component (AdminShell) re-reads the
      // updated tenant name on the next render.
      router.refresh();
    });
  }

  return (
    <form onSubmit={handleSubmit} className="space-y-8">
      <Section title="Store">
        <Field label="Store name" htmlFor="name">
          <Input
            id="name"
            value={name}
            onChange={(e) => {
              setName(e.target.value);
              setSuccess(false);
            }}
            maxLength={200}
            disabled={pending}
            required
          />
        </Field>

        <ReadOnlyField
          label="Store URL slug"
          value={`${tenant.slug}.mark8ly.com`}
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
          <ReadOnlyField label="Country" value={tenant.country_code} />
          <ReadOnlyField label="Currency" value={tenant.currency_code} />
        </div>
        <ReadOnlyField
          label="Timezone"
          value={tenant.timezone}
          hint="Timezone editing is coming in a follow-up. Contact support if you need it changed now."
        />
      </Section>

      {error && (
        <div
          role="alert"
          className="rounded-lg border border-destructive/20 bg-destructive/5 px-4 py-3 text-sm text-destructive"
        >
          {error}
        </div>
      )}
      {success && (
        <div
          role="status"
          className="rounded-lg border border-emerald-200 bg-emerald-50 px-4 py-3 text-sm text-emerald-800"
        >
          Saved.
        </div>
      )}

      <div className="flex items-center justify-end gap-3 border-t border-border pt-6">
        <button
          type="button"
          onClick={() => {
            setName(tenant.name);
            setError(null);
            setSuccess(false);
          }}
          disabled={!dirty || pending}
          className="rounded-xl px-4 py-2.5 text-sm font-medium text-muted-foreground hover:text-foreground disabled:cursor-not-allowed disabled:opacity-50"
        >
          Reset
        </button>
        <button
          type="submit"
          disabled={!dirty || pending}
          className="inline-flex items-center justify-center rounded-xl bg-primary px-5 py-2.5 text-sm font-medium text-primary-foreground shadow-[0_14px_30px_rgba(31,30,28,0.18)] transition hover:bg-primary-hover disabled:cursor-not-allowed disabled:opacity-50"
        >
          {pending ? "Saving..." : "Save changes"}
        </button>
      </div>
    </form>
  );
}

interface SectionProps {
  title: string;
  children: React.ReactNode;
}

function Section({ title, children }: SectionProps) {
  return (
    <section className="space-y-4 rounded-xl border border-border bg-card p-6">
      <h2 className="text-sm font-semibold uppercase tracking-wide text-muted-foreground">
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
