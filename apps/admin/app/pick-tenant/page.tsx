import { headers } from "next/headers";
import { redirect } from "next/navigation";

import { listMemberTenants } from "@/lib/api/platform-api";
import { PickTenantForm } from "@/components/auth/PickTenantForm";

/**
 * Phase P — /pick-tenant.
 *
 * Landing page for users with more than one tenant. Signed-in via
 * admin /login → multi-tenant membership detected → redirected here.
 * Lists all tenants the user belongs to as cards; clicking one calls
 * the switchToTenant server action then redirects to /dashboard.
 *
 * Solo-founder users should never see this page — the sign-in flow
 * skips straight to /dashboard when exactly one tenant is returned.
 * If someone navigates here manually with zero or one tenant, we
 * redirect them away.
 */
export default async function PickTenantPage() {
  const h = await headers();
  const uid = h.get("x-session-user-id") ?? "";
  if (!uid) {
    // Middleware should have caught this, but fail safe.
    redirect("/login");
  }

  const tenants = await listMemberTenants(uid).catch(() => []);

  if (tenants.length === 0) {
    redirect("/login");
  }
  if (tenants.length === 1) {
    // Nothing to pick — go to dashboard.
    redirect("/dashboard");
  }

  return (
    <main className="min-h-screen px-4 py-10 sm:px-6 sm:py-14">
      <div className="mx-auto grid min-h-[calc(100vh-5rem)] max-w-6xl items-center gap-8 lg:grid-cols-[minmax(0,1.05fr)_minmax(24rem,0.95fr)]">
        <section className="hidden lg:block">
          <p className="admin-eyebrow">Store access</p>
          <h1 className="mt-4 max-w-xl font-serif text-6xl font-medium leading-none tracking-tight text-foreground">
            Choose the workspace you want to open.
          </h1>
          <p className="mt-6 max-w-xl text-lg leading-8 text-muted-foreground">
            Your account belongs to more than one store. Pick the one you want
            to work in right now and we&apos;ll switch the admin session for you.
          </p>

          <div className="mt-10 grid max-w-2xl gap-4 sm:grid-cols-2">
            {pickTenantNotes.map((item) => (
              <div key={item.title} className="admin-panel rounded-[1.5rem] p-5">
                <p className="text-sm font-semibold text-foreground">{item.title}</p>
                <p className="mt-2 text-sm leading-6 text-muted-foreground">{item.body}</p>
              </div>
            ))}
          </div>
        </section>

        <div className="mx-auto w-full max-w-xl">
          <header className="mb-6 text-center lg:text-left">
            <p className="admin-eyebrow lg:hidden">Store access</p>
            <h1 className="mt-2 font-serif text-3xl font-medium tracking-tight text-foreground">
              Pick a store
            </h1>
            <p className="mt-2 text-sm leading-6 text-muted-foreground">
              You have access to multiple stores. Choose one to continue.
            </p>
          </header>
          <PickTenantForm tenants={tenants} />
        </div>
      </div>
    </main>
  );
}

const pickTenantNotes = [
  {
    title: "Switch without signing out",
    body: "We remint your session for the selected store and take you straight into its admin.",
  },
  {
    title: "Role-aware access",
    body: "Each store can give you a different level of access depending on how you help the team.",
  },
];
