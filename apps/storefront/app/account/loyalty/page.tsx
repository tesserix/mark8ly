import { cookies, headers } from "next/headers";
import { slugFromHost } from "@/lib/slug";
import { decodeSession } from "@/lib/auth";
import { LoyaltyDashboard } from "@/components/loyalty/LoyaltyDashboard";
import { getProgram, getMe, enrollCustomer } from "@/lib/api/loyalty";

export const metadata = {
  title: "Loyalty",
};

export default async function LoyaltyAccountPage() {
  const cookieStore = await cookies();
  const sessionCookie = cookieStore.get("mp_customer_session")?.value ?? "";
  const session = decodeSession(sessionCookie);

  const h = await headers();
  const host = h.get("host");
  const storeSlug =
    slugFromHost(host) || process.env.DEFAULT_STORE_SLUG || "default";
  const storefrontKey = process.env.MARKETPLACE_STOREFRONT_KEY ?? "";

  const program = await getProgram(storeSlug, storefrontKey);

  if (!program || !program.is_active) {
    return (
      <div className="space-y-2">
        <h1 className="font-[family-name:var(--font-source-serif),'Source_Serif_4',serif] text-2xl font-medium text-[color:var(--ink-900)]">
          Loyalty
        </h1>
        <p className="text-sm text-[color:var(--ink-900)] opacity-50">
          Loyalty program is not available for this store.
        </p>
      </div>
    );
  }

  if (!session) {
    return (
      <div className="space-y-2">
        <h1 className="font-[family-name:var(--font-source-serif),'Source_Serif_4',serif] text-2xl font-medium text-[color:var(--ink-900)]">
          Loyalty
        </h1>
        <p className="text-sm text-[color:var(--ink-900)] opacity-50">
          Please sign in to view your loyalty balance.
        </p>
      </div>
    );
  }

  // Lazy-enroll: customers who signed up before the loyalty program was
  // active miss the sign-in-time auto-enroll. Catch them here so visiting
  // the page is enough to receive the signup bonus + get a referral code.
  let customer = await getMe(storeSlug, storefrontKey, session.email);
  if (!customer) {
    const referralCode = cookieStore.get("mp_referral")?.value;
    customer = await enrollCustomer(
      storeSlug,
      storefrontKey,
      session.email,
      undefined,
      referralCode,
    );
    // Note: cookie deletion from a server component isn't reliable; the
    // signup action handles the burn-after-use path. If this lazy-enroll
    // ran for a legitimate referral, it was honored before the dashboard
    // renders.
  }

  return (
    <div className="space-y-6">
      <h1 className="font-[family-name:var(--font-source-serif),'Source_Serif_4',serif] text-2xl font-medium text-[color:var(--ink-900)]">
        Loyalty
      </h1>
      <LoyaltyDashboard
        program={program}
        customer={customer}
        storeHost={host ?? ""}
      />
    </div>
  );
}
