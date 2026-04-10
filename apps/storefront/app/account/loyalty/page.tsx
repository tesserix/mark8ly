import { LoyaltyDashboard } from "@/components/loyalty/LoyaltyDashboard";
import { getProgram, getMe } from "@/lib/api/loyalty";

// This page is accessed by logged-in customers. The store slug and
// customer email come from the storefront session/context.

export default async function LoyaltyAccountPage() {
  // These values should come from your storefront session middleware.
  // Adjust the imports to match your actual session utility.
  const storeSlug = process.env.STORE_SLUG ?? "";
  const storefrontKey = process.env.STOREFRONT_KEY ?? "";
  const customerEmail = ""; // TODO: get from session

  const program = await getProgram(storeSlug, storefrontKey);

  if (!program || !program.is_active) {
    return (
      <div className="mx-auto max-w-2xl px-4 py-12">
        <p className="text-sm text-[color:var(--ink-900)]/50">
          Loyalty program is not available for this store.
        </p>
      </div>
    );
  }

  const customer = customerEmail
    ? await getMe(storeSlug, storefrontKey, customerEmail)
    : null;

  return (
    <div className="mx-auto max-w-2xl space-y-8 px-4 py-12">
      <header>
        <h1 className="font-[family-name:var(--font-source-serif),'Source_Serif_4',serif] text-3xl font-medium text-[color:var(--ink-900)]">
          Loyalty
        </h1>
      </header>
      <LoyaltyDashboard program={program} customer={customer} />
    </div>
  );
}
