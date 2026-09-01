import Link from "next/link";

import { MailLink } from "@repo/ui/mail-link";

/**
 * Thin footer used by the onboarding flow and post-submit
 * pages. Pure type, a single row, hairline top border. No card
 * chrome, no icons.
 */
export function SlimFooter() {
  return (
    <footer className="border-t border-border-subtle bg-background">
      <div className="mx-auto flex max-w-6xl flex-col gap-3 px-6 py-5 text-sm text-foreground-secondary sm:flex-row sm:items-center sm:justify-between">
        <p>
          Need help?{" "}
          <MailLink
            email="hello@mark8ly.com"
            className="font-medium text-foreground underline decoration-moss-700 decoration-2 underline-offset-4 hover:text-moss-700"
          />
        </p>

        <div className="flex flex-wrap items-center gap-x-6 gap-y-2">
          <Link href="/terms" className="hover:text-foreground">
            Terms
          </Link>
          <Link href="/privacy" className="hover:text-foreground">
            Privacy
          </Link>
          <Link href="/legal" className="hover:text-foreground">
            Security
          </Link>
        </div>

        {/* Shown through onboarding because this is where a merchant commits
            to the platform and to its payment terms. */}
        <p className="w-full text-xs leading-relaxed opacity-80">
          Tesserix Pty Ltd (ACN 694 070 865 · ABN 59 694 070 865)
        </p>
      </div>
    </footer>
  );
}
