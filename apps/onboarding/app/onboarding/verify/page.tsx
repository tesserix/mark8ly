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
      description="This should feel like a polished handoff, not a bare loading state. The branded shell now carries the same quality as the rest of the journey."
    >
      <Suspense fallback={<div className="text-foreground-secondary">Loading…</div>}>
        <VerifyMagicLink />
      </Suspense>
    </PostSubmitShell>
  );
}
