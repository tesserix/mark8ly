// Pure-function helpers used by /pick-tenant to decide what to render.
//
// The whole point of breaking this out of the page is so it can be unit
// tested. The bug we're guarding against is: a single-tenant user lands
// on a `{slug}-admin.mark8ly.com` host whose slug they don't have
// access to, the middleware redirects them to /pick-tenant on the same
// host, the page redirects to /dashboard (because they have only one
// tenant), and middleware redirects right back to /pick-tenant. The
// loop only goes away when the page refuses to redirect to /dashboard
// on a host whose slug isn't in the user's tenant list.
//
// Decision matrix (host slug = the slug parsed off
// `{slug}-admin.mark8ly.com`, null on the canonical host):
//
//  | tenants | host slug | action                                    |
//  |---------|-----------|-------------------------------------------|
//  | 0       | any       | redirect("/login")                        |
//  | 1       | null      | redirect("/dashboard") — fast path        |
//  | 1       | matches   | redirect("/dashboard") — same host        |
//  | 1       | mismatch  | render picker w/ wrong-store header       |
//  | 2+      | matches   | render picker (multi-tenant primary path) |
//  | 2+      | null      | render picker                             |
//  | 2+      | mismatch  | render picker w/ wrong-store header       |

export interface PickTenantTenant {
  tenant_id: string;
  name: string;
  role: string;
}

export type PickTenantDecision =
  | { kind: "redirect_login" }
  | { kind: "redirect_dashboard" }
  | { kind: "render_picker"; wrongStore: boolean };

interface DecideArgs {
  tenants: PickTenantTenant[];
  /** Tenant id resolved from the request host slug, or null when the
   *  request is on the canonical admin host / a custom domain / a host
   *  whose slug doesn't match a known store. */
  hostTenantId: string | null;
}

export function decidePickTenantOutcome({
  tenants,
  hostTenantId,
}: DecideArgs): PickTenantDecision {
  if (tenants.length === 0) {
    return { kind: "redirect_login" };
  }

  const hostSlugMatchesAccessibleTenant =
    hostTenantId !== null &&
    tenants.some((t) => t.tenant_id === hostTenantId);

  // Auto-skip to /dashboard ONLY when staying on the current host won't
  // re-trigger a wrong-slug redirect from middleware. Two cases qualify:
  //   • canonical host (no slug to match against), or
  //   • slug subdomain whose tenant the user actually owns.
  if (
    tenants.length === 1 &&
    (hostTenantId === null || hostSlugMatchesAccessibleTenant)
  ) {
    return { kind: "redirect_dashboard" };
  }

  const wrongStore = hostTenantId !== null && !hostSlugMatchesAccessibleTenant;
  return { kind: "render_picker", wrongStore };
}
