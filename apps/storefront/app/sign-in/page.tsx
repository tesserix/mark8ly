import type { Metadata } from "next";
import { headers } from "next/headers";
import { slugFromHost } from "@/lib/slug";
import { fetchStoreBySlug } from "@/lib/api/platform-api";
import { CustomerSignInForm } from "@/components/auth/CustomerSignInForm";
import { StorefrontNav } from "@/components/StorefrontNav";

export const metadata: Metadata = {
  title: "Sign in",
  robots: { index: false, follow: false },
};

/**
 * /sign-in — customer sign-in page.
 *
 * Server component reads the store context + GIP config from env, then
 * hands everything to the client-side form. The form calls GIP
 * Identity Toolkit directly (browser → Google), gets an id_token, and
 * hands it to a server action that mints the session via auth-bff.
 */
export default async function SignInPage() {
  const h = await headers();
  const host = h.get("host");
  const storeSlug =
    slugFromHost(host) || process.env.DEFAULT_STORE_SLUG || "default";

  const store = await fetchStoreBySlug(storeSlug).catch(() => null);

  const gipConfig = {
    apiKey:
      process.env.GIP_WEB_API_KEY ??
      process.env.NEXT_PUBLIC_GIP_API_KEY ??
      "",
    tenantId: process.env.GIP_CUSTOMER_TENANT_ID ?? "",
    projectId: process.env.GIP_PROJECT_ID ?? "",
  };

  const protocol = h.get("x-forwarded-proto") ?? "https";
  const origin = host ? `${protocol}://${host}` : "";

  return (
    <div className="min-h-screen bg-[color:var(--storefront-background,var(--paper-200))]">
      <StorefrontNav storeName={store?.name} />
      <main id="main" className="mx-auto max-w-md px-6 py-16 sm:px-8 sm:py-24">
        <header className="space-y-2">
          <p className="text-[11px] font-semibold uppercase tracking-[0.24em] text-[color:var(--storefront-text,var(--ink-900))] opacity-55">
            {store?.name ?? "Store"}
          </p>
          <h1 className="font-[family-name:var(--font-source-serif),'Source_Serif_4',serif] text-3xl font-medium text-[color:var(--storefront-text,var(--ink-900))]">
            Sign in
          </h1>
          <p className="text-sm leading-6 text-[color:var(--storefront-text,var(--ink-900))] opacity-70">
            Sign in to track orders, manage addresses, and earn loyalty rewards.
          </p>
        </header>
        <CustomerSignInForm
          gipConfig={gipConfig}
          storeSlug={storeSlug}
          returnUrl={`${origin}/account`}
        />
      </main>
    </div>
  );
}
