"use client";

import Link from "next/link";

import { useOnboardingStore } from "@/lib/store/onboarding-store";

// NEXT_PUBLIC_ADMIN_URL_TEMPLATE is baked into the client bundle at build
// time (see apps/onboarding/Dockerfile). It should contain a `{slug}`
// placeholder so each tenant lands on their own admin subdomain. Prod
// value: https://{slug}-admin.mark8ly.com/dashboard. Dev fallback is the
// flat localhost host used by the docker-compose stack.
const ADMIN_URL_TEMPLATE =
  process.env.NEXT_PUBLIC_ADMIN_URL_TEMPLATE ??
  "http://localhost:4202/dashboard";

/**
 * WelcomeCta — renders the "Open admin dashboard" + "Back to home" row.
 *
 * Lives in a client component so it can read the freshly-onboarded
 * tenant slug from the onboarding zustand store and substitute it into
 * the per-tenant admin URL template. The zustand value is set by
 * OnboardingForm.onValid after a successful submit, so by the time the
 * welcome page renders it's always populated.
 *
 * If for some reason the slug is empty (direct navigation to /welcome
 * without finishing onboarding), we still render a working button by
 * leaving the `{slug}` placeholder in place — the subdomain 404 will
 * make the problem obvious rather than silently leaking to localhost.
 */
interface WelcomeCtaProps {
  /** False on the Zitadel path, where no session was minted for the admin
   *  origin and the merchant signs in there once. Only the button LABEL
   *  changes — the destination is the same either way, and admin's own
   *  middleware sends an unauthenticated visitor to its login page. */
  signedIn?: boolean;
}

export function WelcomeCta({ signedIn = true }: WelcomeCtaProps) {
  const slug = useOnboardingStore((s) => s.slug);
  const adminUrl = resolveAdminUrl(ADMIN_URL_TEMPLATE, slug);

  return (
    <div className="flex flex-wrap items-center gap-x-8 gap-y-4">
      <a
        href={adminUrl}
        className="inline-flex h-12 items-center rounded-md bg-primary px-6 text-base font-medium text-primary-foreground hover:bg-primary-hover"
      >
        {signedIn ? "Open admin dashboard" : "Sign in to your admin"}
      </a>
      <Link href="/" className="btn-ghost">
        Back to home
      </Link>
    </div>
  );
}

function resolveAdminUrl(template: string, slug: string): string {
  if (!slug) return template;
  return template.replace("{slug}", slug);
}
