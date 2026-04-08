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
      </div>
    </PostSubmitShell>
  );
}
