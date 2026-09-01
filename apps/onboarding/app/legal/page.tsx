import Link from "next/link";

import { MailLink } from "@repo/ui/mail-link";
import {
  MarketingPage,
  PageHero,
} from "@/components/marketing/primitives";

export const metadata = {
  title: "Legal & compliance",
  description:
    "Mark8ly&rsquo;s policies, in one place. Privacy, terms, security, sub-processors, DPA, acceptable use, cookies, and refunds.",
  alternates: { canonical: "/legal" },
};

interface LegalDoc {
  href: string;
  title: string;
  blurb: string;
}

const LEGAL_DOCS: ReadonlyArray<LegalDoc> = [
  {
    href: "/privacy",
    title: "Privacy policy",
    blurb: "What we collect, why, and your rights — under the APPs, GDPR, UK GDPR, CCPA/CPRA, and DPDP.",
  },
  {
    href: "/terms",
    title: "Terms of service",
    blurb: "The agreement that governs your use of Mark8ly. Governed by New South Wales, Australia.",
  },
  {
    href: "/acceptable-use",
    title: "Acceptable use",
    blurb: "What you can&rsquo;t sell or do on Mark8ly, and how we handle violations.",
  },
  {
    href: "/cookies",
    title: "Cookie policy",
    blurb: "The small set of cookies we use, why, and how to opt out.",
  },
  {
    href: "/refunds",
    title: "Refunds",
    blurb: "Ninety-day trial, fourteen-day refund window, Australian Consumer Law always applies.",
  },
  {
    href: "/security",
    title: "Security",
    blurb: "Encryption, access controls, incident response, and responsible disclosure.",
  },
  {
    href: "/sub-processors",
    title: "Sub-processors",
    blurb: "The vendors we use to run the service and where they process data.",
  },
  {
    href: "/dpa",
    title: "Data Processing Addendum",
    blurb: "Our controller-processor contract, auto-accepted when you sign up. GDPR Article 28 aligned.",
  },
];

export default function LegalHubPage() {
  return (
    <MarketingPage>
      <PageHero
        eyebrow="Legal &amp; compliance"
        title={<>Policies.</>}
        lede="Everything we publish about how Mark8ly works legally, in plain language, in one place."
      />

      <section className="border-t border-border-subtle pb-24 pt-16">
        <div className="mx-auto max-w-6xl px-6">
          <ul className="grid gap-x-10 gap-y-10 sm:grid-cols-2 lg:grid-cols-3">
            {LEGAL_DOCS.map((doc) => (
              <li key={doc.href}>
                <Link
                  href={doc.href}
                  className="group block border-t border-border-subtle pt-6 transition-colors"
                >
                  <h2 className="font-serif text-xl font-medium text-foreground group-hover:text-moss-700">
                    {doc.title}
                  </h2>
                  <p className="mt-3 text-[0.9375rem] leading-relaxed text-foreground-secondary">
                    {doc.blurb}
                  </p>
                  <span className="mt-4 inline-flex items-center text-sm text-moss-700 underline decoration-moss-700 decoration-2 underline-offset-4">
                    Read
                  </span>
                </Link>
              </li>
            ))}
          </ul>

          <div className="mt-20 border-t border-border-subtle pt-10 text-sm text-foreground-secondary">
            <p>
              Mark8ly is a product of <strong>Tesserix Pty Ltd</strong> (ACN
              694 070 865, ABN 59 694 070 865), registered in New South Wales,
              Australia. Primary processing region: asia-south1 (Mumbai).
            </p>
            <p className="mt-3">
              Questions?{" "}
              <MailLink
                email="legal@mark8ly.com"
                className="text-foreground underline decoration-moss-700 decoration-2 underline-offset-4 hover:text-moss-700"
              />
            </p>
          </div>
        </div>
      </section>
    </MarketingPage>
  );
}
