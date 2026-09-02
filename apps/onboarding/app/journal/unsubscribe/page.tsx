import { Suspense } from "react";
import { MailLink } from "@repo/ui/mail-link";

import {
  MarketingPage,
  PageHero,
  Prose,
} from "@/components/marketing/primitives";
import { JournalUnsubscribeConfirm } from "@/components/marketing/JournalUnsubscribeConfirm";
import { JournalUnsubscribeTokenReader } from "@/components/marketing/JournalUnsubscribeTokenReader";

// Never indexed — this is a private action page reached only via a
// per-subscriber emailed link, not a destination anyone should land on
// from search.
export const metadata = {
  title: "Unsubscribe",
  robots: { index: false, follow: false },
  alternates: { canonical: "/journal/unsubscribe" },
};

// Rendered per request so the CSP nonce reaches the client component's
// script tags — same reasoning as app/onboarding/verify/page.tsx.
export const dynamic = "force-dynamic";

export default function JournalUnsubscribePage() {
  return (
    <MarketingPage>
      <PageHero
        eyebrow="Journal"
        title={<>Unsubscribe.</>}
        lede="Stop receiving Journal emails and remove your address from our marketing list."
      />

      <Prose>
        {/* JournalUnsubscribeTokenReader reads ?token= via useSearchParams
            (client-only), so it's wrapped in Suspense — same pattern as
            VerifyMagicLink on /onboarding/verify. See
            JournalUnsubscribeConfirm.tsx for why confirmation requires an
            explicit click and never fires from a mount/effect. */}
        <Suspense
          fallback={<p className="text-foreground-secondary">Loading…</p>}
        >
          <JournalUnsubscribeTokenReader>
            {(token) => <JournalUnsubscribeConfirm token={token} />}
          </JournalUnsubscribeTokenReader>
        </Suspense>

        <h2>Deleted the email already?</h2>
        <p>
          If you no longer have the unsubscribe link, we can&rsquo;t look up
          your address by token — email <MailLink email="privacy@mark8ly.com" />{" "}
          from the address you subscribed with and we&rsquo;ll remove it by
          hand.
        </p>
      </Prose>
    </MarketingPage>
  );
}
