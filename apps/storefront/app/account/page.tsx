import { cookies, headers } from "next/headers";
import { slugFromHost } from "@/lib/slug";
import { fetchProfile } from "@/lib/api/customer-api";
import { hasSessionCookie } from "@/lib/auth";

export const metadata = {
  title: "My Account",
};

export default async function AccountDashboardPage() {
  const h = await headers();
  const host = h.get("host");
  const storeSlug =
    slugFromHost(host) || process.env.DEFAULT_STORE_SLUG || "default";

  const cookieStore = await cookies();
  const cookieHeader = cookieStore.toString();
  const authenticated = hasSessionCookie(cookieHeader);

  const profile = authenticated
    ? await fetchProfile(storeSlug, cookieHeader)
    : null;

  if (!authenticated || !profile) {
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

  const fullName = [profile.first_name, profile.last_name]
    .filter(Boolean)
    .join(" ");

  return (
    <div className="space-y-6">
      <h1 className="font-[family-name:var(--font-source-serif),'Source_Serif_4',serif] text-2xl font-medium text-[color:var(--ink-900)]">
        My Account
      </h1>

      <div className="space-y-4 border-t border-[color:var(--ink-900)]/10 pt-6">
        <dl className="grid grid-cols-[auto_1fr] gap-x-6 gap-y-3 text-sm">
          {fullName && (
            <>
              <dt className="text-[color:var(--ink-900)] opacity-50">Name</dt>
              <dd className="text-[color:var(--ink-900)]">{fullName}</dd>
            </>
          )}
          <dt className="text-[color:var(--ink-900)] opacity-50">Email</dt>
          <dd className="text-[color:var(--ink-900)]">{profile.email}</dd>
          {profile.phone && (
            <>
              <dt className="text-[color:var(--ink-900)] opacity-50">Phone</dt>
              <dd className="text-[color:var(--ink-900)]">{profile.phone}</dd>
            </>
          )}
          <dt className="text-[color:var(--ink-900)] opacity-50">
            Member since
          </dt>
          <dd className="text-[color:var(--ink-900)]">
            {new Date(profile.created_at).toLocaleDateString(undefined, {
              year: "numeric",
              month: "long",
              day: "numeric",
            })}
          </dd>
        </dl>
      </div>
    </div>
  );
}
