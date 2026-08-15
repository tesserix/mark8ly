import { NextResponse, type NextRequest } from "next/server";

import { classifyStorefrontHost } from "./lib/host-policy";

const MARKETPLACE_API_URL =
  process.env.MARKETPLACE_API_URL || "http://localhost:8080";

// Paths that must remain reachable on any host (probes, asset CDN warm
// hits, sitemap crawlers). Everything else routes through the
// host-classification gate.
const HOST_GATE_BYPASS = [
  "/_next",
  "/favicon",
  "/icon-",
  "/api/health",
  "/api/analytics-config",
  "/robots.txt",
  "/sitemap",
];

export async function middleware(req: NextRequest) {
  const { pathname } = req.nextUrl;

  // Skip the gate for static assets + probes — CDN edges and kubelet
  // hit these on whatever host they were warmed against.
  if (HOST_GATE_BYPASS.some((p) => pathname === p || pathname.startsWith(p))) {
    return passthrough(req);
  }

  const hostHeader = req.headers.get("host") ?? "";
  const hostKind = classifyStorefrontHost(hostHeader);

  // Hard 404 on hosts the storefront should never serve. Wildcard
  // subdomains like `random.mark8ly.com` for an unonboarded slug will
  // be rejected by the slug-existence check below; this is the
  // structurally-malformed case (admin hosts hit storefront pod, etc.).
  if (hostKind.kind === "unknown") {
    return new NextResponse(null, { status: 404 });
  }

  // mark8ly.com / www.mark8ly.com / api.mark8ly.com / admin.mark8ly.com:
  // not storefront surfaces. Marketing site is a separate app — Istio
  // routes those hosts elsewhere in prod, so a hit here means config
  // drift. 404 cleanly instead of rendering DEFAULT_STORE under the
  // wrong brand.
  if (hostKind.kind === "marketing") {
    return passthrough(req);
  }

  // Tenanted slug or custom domain — verify the slug actually
  // resolves to an onboarded store before letting the page render.
  // This is the wildcard-subdomain block: `random.mark8ly.com` for
  // an unonboarded `random` no longer renders anything. Plus the
  // custom-domain takeover: once the merchant has a verified
  // `custom_domains.status='active'` row, the platform `*.mark8ly.com`
  // URL 301's to the custom domain so old links still work but the
  // canonical URL is the merchant's own.
  if (hostKind.kind === "slug") {
    const lookup = await fetchSlugStatus(hostKind.slug);
    if (lookup === "not_found") {
      return new NextResponse(null, { status: 404 });
    }
    if (lookup && lookup.customDomain) {
      const target = new URL(req.nextUrl);
      target.host = lookup.customDomain;
      target.protocol = "https:";
      target.port = "";
      return NextResponse.redirect(target, 301);
    }
    // lookup === null → marketplace-api unreachable. Fail open and
    // render the platform subdomain rather than 404 a real merchant.
  } else if (hostKind.kind === "custom") {
    const resolved = await resolveCustomDomain(hostKind.domain);
    if (resolved === false) {
      return new NextResponse(null, { status: 404 });
    }
    // resolved === null → API unreachable; same fail-open behaviour.
  }

  return passthrough(req);
}

// passthrough preserves the legacy referral-cookie capture while letting
// the request continue. Captures `?ref=CODE` from any page load and
// persists it in the `mp_referral` cookie so referral attribution
// survives the gap between an invite click and signup + enrolment.
function passthrough(req: NextRequest): NextResponse {
  const ref = req.nextUrl.searchParams.get("ref");
  if (!ref || !/^[A-Z0-9]{4,20}$/.test(ref)) {
    return NextResponse.next();
  }
  const res = NextResponse.next();
  res.cookies.set({
    name: "mp_referral",
    value: ref,
    path: "/",
    httpOnly: true,
    sameSite: "lax",
    maxAge: 60 * 60 * 24 * 30,
  });
  return res;
}

// SlugStatus is the result of fetchSlugStatus: either the slug is
// unknown (`"not_found"`), or the slug exists and we know whether the
// merchant has a verified custom-domain takeover via `customDomain`.
// `null` is a transient failure — caller fails open and renders the
// platform subdomain rather than 404 a real merchant.
type SlugStatus =
  | "not_found"
  | { kind: "ok"; customDomain: string };

// fetchSlugStatus calls marketplace-api's
// `/internal/store-active-domain/:slug`. One round trip resolves both
// "is this slug onboarded?" (404 → not_found) and "does the merchant
// have a verified custom-domain takeover?" (200 + non-empty
// `custom_domain`).
async function fetchSlugStatus(slug: string): Promise<SlugStatus | null> {
  if (!slug) return "not_found";
  try {
    const res = await fetch(
      `${MARKETPLACE_API_URL}/internal/store-active-domain/${encodeURIComponent(slug)}`,
      { cache: "no-store" },
    );
    if (res.status === 404) return "not_found";
    if (!res.ok) return null;
    const body = (await res.json().catch(() => ({}))) as {
      slug?: string;
      custom_domain?: string;
    };
    return {
      kind: "ok",
      customDomain: (body.custom_domain ?? "").trim().toLowerCase(),
    };
  } catch {
    return null;
  }
}

// resolveCustomDomain returns true if marketplace-api recognises the
// domain as a registered storefront, false on 404, and null on transient
// errors.
async function resolveCustomDomain(domain: string): Promise<boolean | null> {
  try {
    const res = await fetch(
      `${MARKETPLACE_API_URL}/api/v1/storefront/resolve-domain?domain=${encodeURIComponent(domain)}`,
      { cache: "no-store" },
    );
    if (res.ok) {
      const body = (await res.json().catch(() => ({}))) as { slug?: string };
      return Boolean(body.slug);
    }
    if (res.status === 404) return false;
    return null;
  } catch {
    return null;
  }
}

export const config = {
  // Match every route except Next internals and static files. The
  // bypass list inside middleware() handles probes, sitemap, etc.
  matcher: ["/((?!_next/static|_next/image|favicon.ico).*)"],
};
