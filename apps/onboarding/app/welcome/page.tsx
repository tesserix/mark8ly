import { AppStoreBadges } from "@repo/ui/app-store-badges";

import { PostSubmitShell } from "@/components/onboarding/PostSubmitShell";
import { WelcomeCta } from "@/components/onboarding/WelcomeCta";

// Welcome page shown after successful onboarding + auto-login.
// The session cookie has already been minted by the time the
// user lands here, so the primary CTA into admin works immediately.
// The CTA itself is a client component so it can read the freshly
// onboarded tenant slug from the zustand store and build a per-tenant
// admin URL — see components/onboarding/WelcomeCta.tsx.

// Funnel destination — never index.
export const metadata = {
  title: "Welcome",
  robots: { index: false, follow: false },
};

export default function WelcomePage() {
  return (
    <PostSubmitShell
      eyebrow="Store ready"
      title="Your store is open."
      description="You&rsquo;re signed in. Step into the admin dashboard to add your first product, shape your storefront, and confirm your settings."
    >
      <div className="border-t border-border-subtle pt-10">
        {/* Head start — same 20% the admin checklist opens with, so the
            momentum promise carries across apps without a reset to zero. */}
        <div className="mb-10 max-w-md">
          <div className="flex items-baseline justify-between">
            <p className="eyebrow">Store setup</p>
            <p className="font-serif text-xl font-medium text-moss-700 tabular-nums">
              20%
            </p>
          </div>
          <div className="mt-3 h-1.5 w-full overflow-hidden rounded-full bg-ink-900/10">
            <div className="h-full w-1/5 rounded-full bg-moss-700 motion-safe:animate-[fadeInUp_0.6s_ease-out_both]" />
          </div>
          <p className="mt-3 text-sm text-foreground-secondary">
            Creating your store already completed the first steps. Finish the
            checklist in your admin to go live.
          </p>
        </div>

        <WelcomeCta />

        <dl className="mt-16 grid gap-10 border-t border-border-subtle pt-10 sm:grid-cols-2">
          <div>
            <dt className="eyebrow mb-3">First thing to try</dt>
            <dd className="text-foreground-secondary leading-relaxed">
              Add your first product and confirm how your storefront should
              look. Everything else can wait.
            </dd>
          </div>
          <div>
            <dt className="eyebrow mb-3">Your admin lives at</dt>
            <dd className="font-serif text-lg text-foreground">
              your-store-admin.mark8ly.com
            </dd>
          </div>
        </dl>

        {/* Highest-intent moment in the funnel: the store exists and they
            are already signed in. Badges render per configured platform —
            see MOBILE_ADMIN_APP_LINKS. */}
        <section className="mt-16 border-t border-border-subtle pt-10">
          <p className="eyebrow mb-3">On your phone</p>
          <h2 className="font-serif text-2xl font-medium text-foreground">
            Run your store from anywhere.
          </h2>
          <p className="mt-3 max-w-prose leading-relaxed text-foreground-secondary">
            Confirm orders, check stock, and answer customers from the Mark8ly
            Admin app — the same store, in your pocket.
          </p>
          <AppStoreBadges className="mt-6" />
        </section>
      </div>
    </PostSubmitShell>
  );
}
