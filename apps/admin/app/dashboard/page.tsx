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
  const headline = tenant ? `${tenant.name} is live.` : "Your store is live.";

  return (
    <AdminShell
      tenantName={tenantName}
      userEmail={email}
      role={role}
      memberships={memberships}
      currentTenantId={tenantId}
    >
      <div className="mx-auto max-w-7xl space-y-16">
        <section className="grid gap-10 lg:grid-cols-[minmax(0,1.35fr)_minmax(22rem,0.9fr)] lg:items-end">
          <div className="space-y-5">
            <p className="eyebrow">
              Welcome back{email ? `, ${email}` : ""}
            </p>
            <h1 className="font-serif text-5xl font-medium leading-[1.05] tracking-tight text-foreground sm:text-6xl">
              {headline}
            </h1>
            <p className="max-w-2xl text-lg leading-8 text-foreground-secondary">
              This is your working surface for products, orders, and storefront
              setup. The migrated shell is ready, and the first feature slices
              can layer in cleanly from here.
            </p>

            <div className="flex flex-wrap gap-3 pt-2">
              <a
                href="/products"
                className="inline-flex h-12 items-center justify-center rounded-md bg-primary px-6 text-base font-medium text-primary-foreground hover:bg-primary-hover"
              >
                Add products
              </a>
              <a
                href="/settings/general"
                className="inline-flex h-12 items-center justify-center rounded-md border border-border bg-background-elevated px-6 text-base font-medium text-foreground hover:border-border-strong"
              >
                Review settings
              </a>
            </div>
          </div>

          <aside className="border-t border-border-subtle pt-8 lg:border-t-0 lg:border-l lg:pt-0 lg:pl-10">
            <p className="eyebrow">Workspace notes</p>
            <ul className="mt-4 divide-y divide-border-subtle">
              {workspaceNotes.map((note) => (
                <li key={note.title} className="py-4 first:pt-0 last:pb-0">
                  <p className="text-sm font-semibold text-foreground">
                    {note.title}
                  </p>
                  <p className="mt-1 text-sm leading-6 text-foreground-secondary">
                    {note.body}
                  </p>
                </li>
              ))}
            </ul>
          </aside>
        </section>

        <section className="border-t border-border-subtle pt-10">
          <p className="eyebrow">By the numbers</p>
          <div className="mt-6 grid gap-10 sm:grid-cols-3">
            {metrics.map((m) => (
              <div key={m.label}>
                <p className="text-[11px] font-semibold uppercase tracking-[0.16em] text-foreground-tertiary">
                  {m.label}
                </p>
                <p className="mt-3 font-serif text-5xl font-medium text-foreground">
                  {m.value}
                </p>
                <p className="mt-2 text-sm leading-6 text-foreground-secondary">
                  {m.hint}
                </p>
              </div>
            ))}
          </div>
        </section>

        <section className="border-t border-border-subtle pt-10">
          <p className="eyebrow">Next up</p>
          <ol className="mt-6 grid gap-8 sm:grid-cols-3">
            {launchSteps.map((step) => (
              <li key={step.title}>
                <p className="font-serif text-3xl font-medium text-moss-700">
                  {step.id}
                </p>
                <p className="mt-3 text-sm font-semibold text-foreground">
                  {step.title}
                </p>
                <p className="mt-1 text-sm leading-6 text-foreground-secondary">
                  {step.body}
                </p>
              </li>
            ))}
          </ol>
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
    id: "01",
    title: "Stock the catalog",
    body: "Add your first product and define the shape of your inventory model.",
  },
  {
    id: "02",
    title: "Lock in store details",
    body: "Review your business name, region, and currency before more customer-facing features land.",
  },
  {
    id: "03",
    title: "Expand into orders and customers",
    body: "Those sections are scaffolded and ready for the next migration slices.",
  },
];
