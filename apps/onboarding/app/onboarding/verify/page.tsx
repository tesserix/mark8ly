import { Suspense } from "react";

import { PostSubmitShell } from "@/components/onboarding/PostSubmitShell";
import { VerifyMagicLink } from "@/components/onboarding/VerifyMagicLink";

// Funnel intermediate — never index.
export const metadata = {
  title: "Verifying",
  robots: { index: false, follow: false },
};

// The verify page is the magic link target. We wrap the client component
// in Suspense because it reads searchParams via useSearchParams (client-side
// only) — the wrapper avoids the static prerendering bailout warning.
export default function VerifyPage() {
  return (
    <PostSubmitShell
      eyebrow="Verification in progress"
      title="We’re preparing your workspace."
      description="We&apos;re confirming your email and moving your session forward. This usually takes a moment."
    >
      <Suspense
        fallback={
          <div className="border-t border-border-subtle pt-10 text-foreground-tertiary">
            Loading…
          </div>
        }
      >
        <VerifyMagicLink />
      </Suspense>
    </PostSubmitShell>
  );
}
