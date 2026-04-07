import { headers } from "next/headers";

import { AdminShell } from "@/components/shell/AdminShell";
import { fetchTenant } from "@/lib/api/platform-api";

/**
 * Dashboard — reads the resolved session from headers set by
 * middleware.ts, looks up the tenant row on platform-api, and renders
 * the merchant-facing home.
 *
 * No auth check here — if middleware let the request through, the
 * session headers are guaranteed to be present. The fetchTenant call
 * is the only network hop on the render path.
 */
export default async function DashboardPage() {
  const h = await headers();
  const tenantId = h.get("x-session-tenant-id") ?? "";
  const email = h.get("x-session-email") ?? "";
  const tenant = await fetchTenant(tenantId);

  const tenantName = tenant?.name ?? "your store";
  const headline = tenant
    ? `${tenant.name} is live.`
    : "Your store is live.";

  return (
    <AdminShell tenantName={tenantName}>
      <div className="mx-auto max-w-4xl">
        <p className="text-xs font-medium uppercase tracking-[0.16em] text-foreground-tertiary">
          Welcome back{email ? `, ${email}` : ""}
        </p>
        <h1 className="mt-3 font-serif text-4xl font-medium tracking-tight text-foreground">
          {headline}
        </h1>
        <p className="mt-4 max-w-2xl text-lg leading-7 text-foreground-secondary">
          Add your first product, customize your storefront, or peek at the
          settings. The rest of the admin is coming — this is the scaffold we
          build everything else on.
        </p>

        <div className="mt-10 grid gap-4 sm:grid-cols-3">
          {metrics.map((m) => (
            <div
              key={m.label}
              className="rounded-2xl border border-warm-200/90 bg-white/88 p-5 shadow-sm"
            >
              <p className="text-xs uppercase tracking-[0.14em] text-foreground-tertiary">
                {m.label}
              </p>
              <p className="mt-2 font-serif text-2xl font-medium text-foreground">
                {m.value}
              </p>
              <p className="mt-1 text-xs text-foreground-tertiary">{m.hint}</p>
            </div>
          ))}
        </div>
      </div>
    </AdminShell>
  );
}

const metrics = [
  {
    label: "Products",
    value: "0",
    hint: "Add your first product to start selling.",
  },
  {
    label: "Orders",
    value: "0",
    hint: "They'll show up here when customers buy.",
  },
  {
    label: "Visitors",
    value: "—",
    hint: "Analytics lands in a later slice.",
  },
];
