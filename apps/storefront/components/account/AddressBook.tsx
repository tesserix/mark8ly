"use client";

// Address book — lists saved addresses, adds up to MAX_ADDRESSES with
// a label (Home / Office / Other), supports edit + delete + set-default.
// Hits /api/account/addresses (proxy to marketplace-api).

import { useEffect, useState } from "react";
import { toast } from "@/lib/toast";

interface Address {
  id: string;
  label?: string;
  is_default?: boolean;
  name: string;
  line1: string;
  line2?: string;
  city: string;
  region?: string;
  postal_code?: string;
  country_code: string;
  phone?: string;
}

const MAX_ADDRESSES = 5;
const LABELS = ["Home", "Office", "Other"] as const;

// Tiny inline-SVG icons so we don't pull in a whole icon pack for
// three labels. Stroke-only, 1em, inherits text color.
function LabelIcon({ label, className = "h-4 w-4" }: { label: string; className?: string }) {
  const common = {
    className,
    viewBox: "0 0 24 24",
    fill: "none",
    stroke: "currentColor",
    strokeWidth: 1.75,
    strokeLinecap: "round" as const,
    strokeLinejoin: "round" as const,
    "aria-hidden": true,
  };
  const normalized = (label || "Home").toLowerCase();
  if (normalized === "home") {
    // House with door
    return (
      <svg {...common}>
        <path d="M3 11 12 4l9 7" />
        <path d="M5 10v10h14V10" />
        <path d="M10 20v-5h4v5" />
      </svg>
    );
  }
  if (normalized === "office") {
    // Briefcase
    return (
      <svg {...common}>
        <rect x="3" y="7" width="18" height="13" rx="2" />
        <path d="M9 7V5a2 2 0 0 1 2-2h2a2 2 0 0 1 2 2v2" />
        <path d="M3 13h18" />
      </svg>
    );
  }
  // Other — pin / location marker
  return (
    <svg {...common}>
      <path d="M12 22s7-7.13 7-12a7 7 0 1 0-14 0c0 4.87 7 12 7 12z" />
      <circle cx="12" cy="10" r="2.5" />
    </svg>
  );
}

type Form = {
  id?: string;
  label: string;
  name: string;
  line1: string;
  line2: string;
  city: string;
  region: string;
  postal_code: string;
  country_code: string;
  phone: string;
  is_default: boolean;
};

const EMPTY: Form = {
  label: "Home",
  name: "",
  line1: "",
  line2: "",
  city: "",
  region: "",
  postal_code: "",
  country_code: "IN",
  phone: "",
  is_default: false,
};

export function AddressBook() {
  const [addresses, setAddresses] = useState<Address[]>([]);
  const [loaded, setLoaded] = useState(false);
  const [showForm, setShowForm] = useState(false);
  const [form, setForm] = useState<Form>(EMPTY);
  const [saving, setSaving] = useState(false);

  async function refresh() {
    const res = await fetch("/api/account/addresses", { cache: "no-store" });
    if (res.ok) {
      const body = (await res.json()) as { data: Address[] };
      setAddresses(body.data ?? []);
    }
    setLoaded(true);
  }

  useEffect(() => {
    void refresh();
  }, []);

  function startAdd() {
    if (addresses.length >= MAX_ADDRESSES) {
      toast({
        title: `Max ${MAX_ADDRESSES} addresses`,
        description: "Remove one to add another.",
        tone: "warn",
      });
      return;
    }
    setForm({ ...EMPTY, is_default: addresses.length === 0 });
    setShowForm(true);
  }

  function startEdit(a: Address) {
    setForm({
      id: a.id,
      label: a.label || "Home",
      name: a.name,
      line1: a.line1,
      line2: a.line2 ?? "",
      city: a.city,
      region: a.region ?? "",
      postal_code: a.postal_code ?? "",
      country_code: a.country_code,
      phone: a.phone ?? "",
      is_default: a.is_default ?? false,
    });
    setShowForm(true);
  }

  async function save(e: React.FormEvent) {
    e.preventDefault();
    setSaving(true);
    const method = form.id ? "PATCH" : "POST";
    const url = form.id
      ? `/api/account/addresses/${form.id}`
      : "/api/account/addresses";
    const res = await fetch(url, {
      method,
      headers: { "Content-Type": "application/json" },
      body: JSON.stringify({
        label: form.label,
        name: form.name,
        line1: form.line1,
        line2: form.line2 || undefined,
        city: form.city,
        region: form.region || undefined,
        postal_code: form.postal_code || undefined,
        country_code: form.country_code,
        phone: form.phone || undefined,
        is_default: form.is_default,
      }),
    });
    setSaving(false);
    if (!res.ok) {
      const body = await res.json().catch(() => ({}));
      toast({
        title: "Couldn't save address",
        description: (body as { message?: string }).message ?? `HTTP ${res.status}`,
        tone: "error",
      });
      return;
    }
    toast({
      title: form.id ? "Address updated" : "Address saved",
      tone: "success",
    });
    setShowForm(false);
    setForm(EMPTY);
    await refresh();
  }

  async function remove(id: string) {
    const res = await fetch(`/api/account/addresses/${id}`, { method: "DELETE" });
    if (res.ok) {
      toast({ title: "Address removed", tone: "info" });
      await refresh();
    } else {
      toast({ title: "Couldn't remove", tone: "error" });
    }
  }

  async function setDefault(a: Address) {
    const res = await fetch(`/api/account/addresses/${a.id}`, {
      method: "PATCH",
      headers: { "Content-Type": "application/json" },
      body: JSON.stringify({ is_default: true }),
    });
    if (res.ok) {
      toast({ title: "Default address updated", tone: "success" });
      await refresh();
    }
  }

  if (!loaded) {
    return <p className="text-sm text-[color:var(--storefront-text,var(--ink-900))] opacity-50">Loading…</p>;
  }

  return (
    <div className="space-y-6">
      <div className="flex items-baseline justify-between gap-3">
        <p className="text-sm text-[color:var(--storefront-text,var(--ink-900))] opacity-70">
          {addresses.length}/{MAX_ADDRESSES} saved
        </p>
        <button
          type="button"
          onClick={startAdd}
          disabled={addresses.length >= MAX_ADDRESSES}
          className="rounded-md border border-[color:var(--storefront-text,var(--ink-900))]/15 bg-white px-4 py-2 text-sm text-[color:var(--storefront-text,var(--ink-900))] transition-colors hover:border-[color:var(--storefront-accent,var(--moss-700))] hover:text-[color:var(--storefront-accent,var(--moss-700))] disabled:cursor-not-allowed disabled:opacity-40"
        >
          + Add address
        </button>
      </div>

      {addresses.length === 0 && !showForm && (
        <p className="text-sm text-[color:var(--storefront-text,var(--ink-900))] opacity-60">
          Your saved addresses will appear here.
        </p>
      )}

      <ul className="grid grid-cols-1 gap-4 sm:grid-cols-2">
        {addresses.map((a) => (
          <li
            key={a.id}
            className="rounded-md border border-[color:var(--storefront-text,var(--ink-900))]/10 bg-white p-4"
          >
            <div className="flex items-center justify-between gap-3">
              <p className="flex items-center gap-2 text-sm font-semibold text-[color:var(--storefront-text,var(--ink-900))]">
                <span className="grid h-7 w-7 place-items-center rounded-full bg-[color:var(--storefront-accent,var(--moss-700))]/10 text-[color:var(--storefront-accent,var(--moss-700))]">
                  <LabelIcon label={a.label || "Home"} />
                </span>
                {a.label || "Home"}
                {a.is_default && (
                  <span className="ml-1 rounded-full bg-[color:var(--storefront-accent,var(--moss-700))]/10 px-2 py-0.5 text-[11px] font-medium text-[color:var(--storefront-accent,var(--moss-700))]">
                    Default
                  </span>
                )}
              </p>
            </div>
            <address className="mt-2 text-sm not-italic leading-relaxed text-[color:var(--storefront-text,var(--ink-900))] opacity-80">
              {a.name}
              <br />
              {a.line1}
              {a.line2 && <><br />{a.line2}</>}
              <br />
              {a.city}
              {a.region ? `, ${a.region}` : ""}
              {a.postal_code ? ` ${a.postal_code}` : ""}
              <br />
              {a.country_code}
              {a.phone && <><br />{a.phone}</>}
            </address>
            <div className="mt-3 flex flex-wrap gap-3 text-xs">
              <button
                type="button"
                onClick={() => startEdit(a)}
                className="text-[color:var(--storefront-accent,var(--moss-700))] underline-offset-2 hover:underline"
              >
                Edit
              </button>
              {!a.is_default && (
                <button
                  type="button"
                  onClick={() => void setDefault(a)}
                  className="text-[color:var(--storefront-text,var(--ink-900))] opacity-70 underline-offset-2 hover:underline hover:opacity-100"
                >
                  Set as default
                </button>
              )}
              <button
                type="button"
                onClick={() => void remove(a.id)}
                className="text-[color:var(--storefront-danger)] underline-offset-2 hover:underline"
              >
                Remove
              </button>
            </div>
          </li>
        ))}
      </ul>

      {showForm && (
        <form
          onSubmit={save}
          className="space-y-4 rounded-md border border-[color:var(--storefront-text,var(--ink-900))]/15 bg-white p-5"
        >
          <h3 className="text-sm font-semibold uppercase tracking-wider text-[color:var(--storefront-text,var(--ink-900))] opacity-70">
            {form.id ? "Edit address" : "Add address"}
          </h3>
          <div>
            <label className="mb-1 block text-xs text-[color:var(--storefront-text,var(--ink-900))] opacity-70">
              Label
            </label>
            <div className="flex flex-wrap gap-2">
              {LABELS.map((l) => (
                <button
                  key={l}
                  type="button"
                  onClick={() => setForm({ ...form, label: l })}
                  className={`inline-flex items-center gap-1.5 rounded-full border px-3 py-1 text-xs transition-colors ${
                    form.label === l
                      ? "border-[color:var(--storefront-accent,var(--moss-700))] bg-[color:var(--storefront-accent,var(--moss-700))]/10 text-[color:var(--storefront-accent,var(--moss-700))]"
                      : "border-[color:var(--storefront-text,var(--ink-900))]/15 text-[color:var(--storefront-text,var(--ink-900))] opacity-70 hover:opacity-100"
                  }`}
                >
                  <LabelIcon label={l} className="h-3.5 w-3.5" />
                  {l}
                </button>
              ))}
            </div>
          </div>
          <Field label="Full name">
            <input
              required
              className={inputClass}
              value={form.name}
              onChange={(e) => setForm({ ...form, name: e.target.value })}
            />
          </Field>
          <Field label="Address line 1">
            <input
              required
              className={inputClass}
              value={form.line1}
              onChange={(e) => setForm({ ...form, line1: e.target.value })}
            />
          </Field>
          <Field label="Address line 2">
            <input
              className={inputClass}
              value={form.line2}
              onChange={(e) => setForm({ ...form, line2: e.target.value })}
            />
          </Field>
          <div className="grid grid-cols-1 gap-4 sm:grid-cols-2">
            <Field label="City">
              <input
                required
                className={inputClass}
                value={form.city}
                onChange={(e) => setForm({ ...form, city: e.target.value })}
              />
            </Field>
            <Field label="State / Region">
              <input
                className={inputClass}
                value={form.region}
                onChange={(e) => setForm({ ...form, region: e.target.value })}
              />
            </Field>
            <Field label="Postal code">
              <input
                className={inputClass}
                value={form.postal_code}
                onChange={(e) => setForm({ ...form, postal_code: e.target.value })}
              />
            </Field>
            <Field label="Country (ISO)">
              <input
                required
                maxLength={2}
                className={inputClass}
                value={form.country_code}
                onChange={(e) =>
                  setForm({ ...form, country_code: e.target.value.toUpperCase() })
                }
              />
            </Field>
            <Field label="Phone">
              <input
                className={inputClass}
                value={form.phone}
                onChange={(e) => setForm({ ...form, phone: e.target.value })}
              />
            </Field>
          </div>
          <label className="inline-flex items-center gap-2 text-sm">
            <input
              type="checkbox"
              checked={form.is_default}
              onChange={(e) => setForm({ ...form, is_default: e.target.checked })}
              className="h-4 w-4 accent-[color:var(--storefront-accent,var(--moss-700))]"
            />
            Set as default shipping address
          </label>
          <div className="flex items-center gap-3">
            <button
              type="submit"
              disabled={saving}
              className="rounded-md bg-[color:var(--storefront-accent,var(--moss-700))] px-4 py-2 text-sm font-medium text-white hover:opacity-90 disabled:cursor-not-allowed disabled:opacity-50"
            >
              {saving ? "Saving…" : form.id ? "Update address" : "Save address"}
            </button>
            <button
              type="button"
              onClick={() => setShowForm(false)}
              className="text-sm text-[color:var(--storefront-text,var(--ink-900))] opacity-70 hover:opacity-100"
            >
              Cancel
            </button>
          </div>
        </form>
      )}
    </div>
  );
}

const inputClass =
  "w-full rounded-md border border-[color:var(--storefront-text,var(--ink-900))]/15 bg-white px-3 py-2 text-sm text-[color:var(--storefront-text,var(--ink-900))] placeholder:opacity-40 focus-visible:outline-2 focus-visible:outline-offset-2 focus-visible:outline-[color:var(--storefront-accent,var(--moss-700))]";

function Field({ label, children }: { label: string; children: React.ReactNode }) {
  return (
    <label className="block">
      <span className="mb-1 block text-xs text-[color:var(--storefront-text,var(--ink-900))] opacity-70">
        {label}
      </span>
      {children}
    </label>
  );
}
