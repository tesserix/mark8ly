// Resolves a `{slug}-admin.mark8ly.com` request host to the tenant id
// platform-api has on record for that slug.
//
// Extracted from app/login/actions.ts (where it originated as a private
// helper for the GIP path's `resolveWorkspaceTenant`) so the Zitadel
// merchant Google sign-in flow can use the exact same host-matching rule
// without duplicating it — see app/auth/idp/finish/route.ts, which needs a
// `workspace_tenant` to send auth-bff's /auth/zitadel/idp/finish BEFORE it
// knows which Zitadel user the Google identity resolves to (that identity
// is only known inside auth-bff, after the intent is retrieved). The host
// slug is the only signal available at that point — exactly the same
// signal `resolveWorkspaceTenant`'s hostMatched branch already trusts for
// the GIP/password Zitadel paths, so this keeps all three paths agreeing
// on which tenant a given admin subdomain means.
import { platformInternalHeaders } from "@/lib/api/server/platformInternal";

const PLATFORM_API_URL =
  process.env.PLATFORM_API_URL ?? "http://localhost:8086";

/**
 * tenantIdForHostSlug resolves `{slug}-admin.mark8ly.com` to the tenant id
 * platform-api has on record for that slug. Returns null when the host is
 * not a per-tenant subdomain, when platform-api is unreachable, or when
 * the slug isn't a known store. Best-effort — never throws.
 */
export async function tenantIdForHostSlug(
  hostHeader: string | null | undefined,
): Promise<string | null> {
  if (!hostHeader) return null;
  // Strip the port if one was appended (e.g. during local dev or tests).
  const host = hostHeader.split(":")[0] ?? "";
  const match = host.match(/^([^.]+)-admin\.mark8ly\.com$/);
  if (!match || !match[1]) return null;
  const slug = match[1];
  try {
    const res = await fetch(
      `${PLATFORM_API_URL}/internal/stores/by-slug/${encodeURIComponent(slug)}`,
      { cache: "no-store", headers: platformInternalHeaders() },
    );
    if (!res.ok) return null;
    const body = (await res.json()) as { data?: { tenant_id?: string } };
    return body.data?.tenant_id ?? null;
  } catch {
    return null;
  }
}
