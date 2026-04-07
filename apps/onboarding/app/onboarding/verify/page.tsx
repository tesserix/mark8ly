import { Suspense } from "react";

import { VerifyMagicLink } from "@/components/onboarding/VerifyMagicLink";

// The verify page is the magic link target. We wrap the client component
// in Suspense because it reads searchParams via useSearchParams (client-side
// only) — the wrapper avoids the static prerendering bailout warning.
export default function VerifyPage() {
  return (
    <main className="min-h-screen flex items-center justify-center px-4 py-12">
      <Suspense fallback={<div className="text-zinc-500">Loading…</div>}>
        <VerifyMagicLink />
      </Suspense>
    </main>
  );
}
