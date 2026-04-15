"use client";

// Editable customer profile form — hydrates from /api/account/profile
// and PATCHes back changes. Fields map 1:1 to customer_profiles columns
// the backend already accepts (first_name, last_name, phone,
// marketing_opt_in).

import { useEffect, useState } from "react";
import { toast } from "@/lib/toast";

interface Profile {
  id: string;
  email: string;
  first_name?: string;
  last_name?: string;
  phone?: string;
  avatar_url?: string;
  marketing_opt_in?: boolean;
  created_at?: string;
}

export function ProfileForm({ email }: { email: string }) {
  const [loaded, setLoaded] = useState(false);
  const [saving, setSaving] = useState(false);
  const [first, setFirst] = useState("");
  const [last, setLast] = useState("");
  const [phone, setPhone] = useState("");
  const [marketing, setMarketing] = useState(false);

  useEffect(() => {
    fetch("/api/account/profile", { cache: "no-store" })
      .then((r) => (r.ok ? r.json() : null))
      .then((body: { data: Profile } | null) => {
        if (body?.data) {
          setFirst(body.data.first_name ?? "");
          setLast(body.data.last_name ?? "");
          setPhone(body.data.phone ?? "");
          setMarketing(body.data.marketing_opt_in ?? false);
        }
        setLoaded(true);
      })
      .catch(() => setLoaded(true));
  }, []);

  async function handleSubmit(e: React.FormEvent) {
    e.preventDefault();
    setSaving(true);
    const res = await fetch("/api/account/profile", {
      method: "PATCH",
      headers: { "Content-Type": "application/json" },
      body: JSON.stringify({
        first_name: first || null,
        last_name: last || null,
        phone: phone || null,
        marketing_opt_in: marketing,
      }),
    });
    setSaving(false);
    if (res.ok) {
      toast({ title: "Profile saved", tone: "success" });
    } else {
      toast({ title: "Couldn't save profile", tone: "error" });
    }
  }

  if (!loaded) {
    return (
      <p className="text-sm text-[color:var(--storefront-text,var(--ink-900))] opacity-50">Loading profile…</p>
    );
  }

  return (
    <form onSubmit={handleSubmit} className="space-y-5">
      <Row label="Email">
        <span className="text-sm text-[color:var(--storefront-text,var(--ink-900))]">{email}</span>
      </Row>
      <Row label="First name">
        <input
          type="text"
          value={first}
          onChange={(e) => setFirst(e.target.value)}
          className={inputClass}
          placeholder="Jane"
        />
      </Row>
      <Row label="Last name">
        <input
          type="text"
          value={last}
          onChange={(e) => setLast(e.target.value)}
          className={inputClass}
          placeholder="Doe"
        />
      </Row>
      <Row label="Phone">
        <input
          type="tel"
          value={phone}
          onChange={(e) => setPhone(e.target.value)}
          className={inputClass}
          placeholder="+91 98765 43210"
        />
      </Row>
      <Row label="Marketing emails">
        <label className="inline-flex items-center gap-2 text-sm text-[color:var(--storefront-text,var(--ink-900))]">
          <input
            type="checkbox"
            checked={marketing}
            onChange={(e) => setMarketing(e.target.checked)}
            className="h-4 w-4 accent-[color:var(--storefront-accent,var(--moss-700))]"
          />
          Send me updates about promotions &amp; new arrivals
        </label>
      </Row>
      <div>
        <button
          type="submit"
          disabled={saving}
          className="inline-flex items-center gap-2 rounded-md bg-[color:var(--storefront-accent,var(--moss-700))] px-4 py-2 text-sm font-medium text-white transition-opacity hover:opacity-90 disabled:cursor-not-allowed disabled:opacity-50"
        >
          {saving ? "Saving…" : "Save changes"}
        </button>
      </div>
    </form>
  );
}

const inputClass =
  "w-full rounded-md border border-[color:var(--storefront-text,var(--ink-900))]/15 bg-white px-3 py-2 text-sm text-[color:var(--storefront-text,var(--ink-900))] placeholder:opacity-40 focus-visible:outline-2 focus-visible:outline-offset-2 focus-visible:outline-[color:var(--storefront-accent,var(--moss-700))]";

function Row({ label, children }: { label: string; children: React.ReactNode }) {
  return (
    <div className="grid grid-cols-1 gap-2 sm:grid-cols-[160px_1fr] sm:items-center">
      <div className="text-xs font-semibold uppercase tracking-[0.12em] text-[color:var(--storefront-text,var(--ink-900))] opacity-60">
        {label}
      </div>
      <div>{children}</div>
    </div>
  );
}
