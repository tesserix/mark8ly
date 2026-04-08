import { Suspense } from "react";

import { PostSubmitShell } from "@/components/onboarding/PostSubmitShell";
import { VerifyMagicLink } from "@/components/onboarding/VerifyMagicLink";

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
          <div className="w-full max-w-xl mx-auto rounded-[2rem] border border-warm-200/90 bg-white/90 px-8 py-10 text-center text-foreground-secondary shadow-[0_24px_80px_rgba(43,38,34,0.12)] backdrop-blur-sm">
            Loading…
          </div>
        }
      >
        <VerifyMagicLink />
      </Suspense>
    </PostSubmitShell>
  );
}
