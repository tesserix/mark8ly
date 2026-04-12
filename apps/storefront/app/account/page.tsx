import { cookies, headers } from "next/headers";
import { slugFromHost } from "@/lib/slug";
import { decodeSession } from "@/lib/auth";

export const metadata = {
  title: "My Account",
};

export default async function AccountDashboardPage() {
  const h = await headers();
  const host = h.get("host");
  const storeSlug =
    slugFromHost(host) || process.env.DEFAULT_STORE_SLUG || "default";

  const cookieStore = await cookies();
  const sessionCookie = cookieStore.get("mp_customer_session")?.value ?? "";
  const session = decodeSession(sessionCookie);

  if (!session) {
    return (
      <div className="space-y-2">
        <h1 className="font-[family-name:var(--font-source-serif),'Source_Serif_4',serif] text-2xl font-medium text-[color:var(--ink-900)]">
          My Account
        </h1>
        <p className="text-sm text-[color:var(--ink-900)] opacity-50">
          Please sign in to view your account.
        </p>
      </div>
    );
  }

  return (
    <div className="space-y-6">
      <h1 className="font-[family-name:var(--font-source-serif),'Source_Serif_4',serif] text-2xl font-medium text-[color:var(--ink-900)]">
        My Account
      </h1>

      <div className="space-y-4 border-t border-[color:var(--ink-900)]/10 pt-6">
        <dl className="grid grid-cols-[auto_1fr] gap-x-6 gap-y-3 text-sm">
          <dt className="text-[color:var(--ink-900)] opacity-50">Email</dt>
          <dd className="text-[color:var(--ink-900)]">{session.email}</dd>
        </dl>
      </div>
    </div>
  );
}
