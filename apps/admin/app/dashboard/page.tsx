import { headers } from "next/headers";

import { AdminShell } from "@/components/shell/AdminShell";
import {
  fetchTenant,
  listMemberTenants,
  type TenantRole,
} from "@/lib/api/platform-api";

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
  const userId = h.get("x-session-user-id") ?? "";
  const email = h.get("x-session-email") ?? "";
  const role = (h.get("x-session-role") ?? "viewer") as TenantRole;
  const [tenant, memberships] = await Promise.all([
    fetchTenant(tenantId),
    userId ? listMemberTenants(userId).catch(() => []) : Promise.resolve([]),
  ]);

  const tenantName = tenant?.name ?? "your store";
  const headline = tenant
    ? `${tenant.name} is live.`
    : "Your store is live.";

  return (
    <AdminShell
      tenantName={tenantName}
      userEmail={email}
      role={role}
      memberships={memberships}
      currentTenantId={tenantId}
    >
      <div className="mx-auto max-w-7xl space-y-6">
        <section className="admin-surface overflow-hidden rounded-[2rem]">
          <div className="grid gap-8 px-6 py-7 sm:px-8 lg:grid-cols-[minmax(0,1.35fr)_minmax(22rem,0.9fr)] lg:px-10 lg:py-9">
            <div>
              <p className="admin-eyebrow">
                Welcome back{email ? `, ${email}` : ""}
              </p>
              <h1 className="mt-3 font-serif text-4xl font-medium tracking-tight text-foreground sm:text-5xl">
                {headline}
              </h1>
              <p className="mt-4 max-w-2xl text-base leading-7 text-muted-foreground sm:text-lg">
                This is your working surface for products, orders, and storefront setup.
                The migrated shell is ready, and the first feature slices can layer in
                cleanly from here.
              </p>

              <div className="mt-7 flex flex-wrap gap-3">
                <a
                  href="/products"
                  className="inline-flex items-center justify-center rounded-full bg-primary px-5 py-3 text-sm font-medium text-primary-foreground shadow-[0_16px_36px_rgba(34,28,23,0.18)] transition-[transform,box-shadow,opacity] hover:-translate-y-0.5 hover:opacity-95"
                >
                  Add products
                </a>
                <a
                  href="/settings/general"
                  className="inline-flex items-center justify-center rounded-full border border-border/90 bg-white/70 px-5 py-3 text-sm font-medium text-foreground transition-[background-color,border-color] hover:bg-white"
                >
                  Review settings
                </a>
              </div>
            </div>

            <div className="admin-panel rounded-[1.75rem] p-5 sm:p-6">
              <p className="admin-eyebrow">Workspace Notes</p>
              <div className="mt-5 space-y-4">
                {workspaceNotes.map((note) => (
                  <div
                    key={note.title}
                    className="border-b admin-soft-rule pb-4 last:border-b-0 last:pb-0"
                  >
                    <p className="text-sm font-semibold text-foreground">{note.title}</p>
                    <p className="mt-1 text-sm leading-6 text-muted-foreground">{note.body}</p>
                  </div>
                ))}
              </div>
            </div>
          </div>
        </section>

        <section className="grid gap-4 lg:grid-cols-[minmax(0,1.45fr)_minmax(18rem,0.75fr)]">
          <div className="grid gap-4 sm:grid-cols-3">
            {metrics.map((m) => (
              <div key={m.label} className="admin-panel rounded-[1.6rem] p-5">
                <p className="text-[11px] font-semibold uppercase tracking-[0.16em] text-muted-foreground">
                  {m.label}
                </p>
                <p className="mt-3 font-serif text-3xl font-medium text-card-foreground">
                  {m.value}
                </p>
                <p className="mt-2 text-sm leading-6 text-muted-foreground">{m.hint}</p>
              </div>
            ))}
          </div>

          <div className="admin-panel rounded-[1.6rem] p-5">
            <p className="admin-eyebrow">Next Up</p>
            <div className="mt-5 space-y-4">
              {launchSteps.map((step) => (
                <div key={step.title} className="flex gap-3">
                  <div className="mt-0.5 flex h-7 w-7 shrink-0 items-center justify-center rounded-full bg-[rgba(138,100,64,0.12)] text-xs font-semibold text-[var(--app-brown)]">
                    {step.id}
                  </div>
                  <div>
                    <p className="text-sm font-semibold text-foreground">{step.title}</p>
                    <p className="mt-1 text-sm leading-6 text-muted-foreground">{step.body}</p>
                  </div>
                </div>
              ))}
            </div>
          </div>
        </section>
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

const workspaceNotes = [
  {
    title: "Products and settings are the first solid surfaces",
    body: "Use the current shell to shape how inventory, pricing, and store configuration will feel day to day.",
  },
  {
    title: "The chrome is ready for feature growth",
    body: "Navigation, account access, and route structure are already in place, so new slices can stay visually consistent.",
  },
];

const launchSteps = [
  {
    id: "1",
    title: "Stock the catalog",
    body: "Add your first product and define the shape of your inventory model.",
  },
  {
    id: "2",
    title: "Lock in store details",
    body: "Review your business name, region, and currency before more customer-facing features land.",
  },
  {
    id: "3",
    title: "Expand into orders and customers",
    body: "Those sections are scaffolded and ready for the next migration slices.",
  },
];
