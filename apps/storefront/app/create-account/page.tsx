import type { Metadata } from "next";
import { headers } from "next/headers";
import { resolveStoreSlug } from "@/lib/slug";
import { fetchStoreBySlug } from "@/lib/api/platform-api";
import { CreateAccountForm } from "@/components/auth/CreateAccountForm";
import { StorefrontNav } from "@/components/StorefrontNav";

export const metadata: Metadata = {
  title: "Create account",
  robots: { index: false, follow: false },
};

export default async function CreateAccountPage() {
  const h = await headers();
  const host = h.get("host");
  const storeSlug =
    await resolveStoreSlug(host);

  const store = await fetchStoreBySlug(storeSlug).catch(() => null);

  const gipConfig = {
    apiKey:
      process.env.GIP_WEB_API_KEY ??
      process.env.NEXT_PUBLIC_GIP_API_KEY ??
      "",
    tenantId: process.env.GIP_CUSTOMER_TENANT_ID ?? "",
    projectId: process.env.GIP_PROJECT_ID ?? "",
    googleClientId: process.env.NEXT_PUBLIC_GOOGLE_CLIENT_ID ?? "",
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
          <h1 className="font-[family-name:var(--storefront-heading-font,var(--font-source-serif))] text-3xl font-medium text-[color:var(--storefront-text,var(--ink-900))]">
            Create account
          </h1>
          <p className="text-sm leading-6 text-[color:var(--storefront-text,var(--ink-900))] opacity-70">
            Create an account to track orders, save addresses, and earn loyalty
            rewards.
          </p>
        </header>
        <CreateAccountForm
          gipConfig={gipConfig}
          storeSlug={storeSlug}
          returnUrl={`${origin}/account`}
        />
      </main>
    </div>
  );
}
