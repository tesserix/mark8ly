import { cookies, headers } from "next/headers";
import { resolveStoreSlug } from "@/lib/slug";
import { decodeSessionForScope } from "@/lib/session";
import { ProfileForm } from "@/components/account/ProfileForm";

export const metadata = {
  title: "My Account",
};

export const dynamic = "force-dynamic";
export const revalidate = 0;

export default async function AccountDashboardPage() {
  const h = await headers();
  const host = h.get("host");
  const storeSlug =
    await resolveStoreSlug(host);

  const cookieStore = await cookies();
  const sessionCookie = cookieStore.get("mp_customer_session")?.value ?? "";
  const session = decodeSessionForScope(sessionCookie, { storeSlug });

  if (!session) {
    return (
      <div className="space-y-2">
        <h1 className="font-[family-name:var(--storefront-heading-font,var(--font-source-serif))] text-2xl font-medium text-[color:var(--storefront-text,var(--ink-900))]">
          My Account
        </h1>
        <p className="text-sm text-[color:var(--storefront-text,var(--ink-900))] opacity-50">
          Please sign in to view your account.
        </p>
      </div>
    );
  }

  return (
    <div className="space-y-6">
      <h1 className="font-[family-name:var(--storefront-heading-font,var(--font-source-serif))] text-2xl font-medium text-[color:var(--storefront-text,var(--ink-900))]">
        My Account
      </h1>

      <div className="border-t border-[color:var(--storefront-text,var(--ink-900))]/10 pt-6">
        <ProfileForm email={session.email} />
      </div>
    </div>
  );
}
