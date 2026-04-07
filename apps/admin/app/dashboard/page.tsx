import { AdminShell } from "@/components/shell/AdminShell";

/**
 * Dashboard — the only "real" page in the Phase I slice. It exists to
 * prove the auth gate works end-to-end: anonymous visitors get bounced
 * by middleware, authenticated visitors land here.
 *
 * Phase J will swap the hard-coded tenant name for the resolved tenant
 * from `GET /api/v1/tenants/me`. For now it's a placeholder that
 * proves the chrome renders.
 */
export default function DashboardPage() {
  // Phase I: hard-coded. Phase J: resolved from the auth-bff session
  // cookie via the server action wired into lib/auth/session.ts.
  const tenantName = "your store";

  return (
    <AdminShell tenantName={tenantName}>
      <div className="mx-auto max-w-4xl">
        <p className="text-xs font-medium uppercase tracking-[0.16em] text-foreground-tertiary">
          Welcome back
        </p>
        <h1 className="mt-3 font-serif text-4xl font-medium tracking-tight text-foreground">
          Your store is live.
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
